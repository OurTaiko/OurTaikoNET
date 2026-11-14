package httpapi

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"ourtaiko.dev/net/internal/account"
	"ourtaiko.dev/net/internal/web"
)

// Activity is one thing a player did that the home page tells the others: a play they
// cleared ("score") or a chart they published ("upload"), on either source.
type Activity struct {
	Kind   string         `json:"kind"`
	At     time.Time      `json:"at"`
	Source string         `json:"source"`
	User   ActivityPlayer `json:"user"`
	Chart  ActivityChart  `json:"chart"`
	// Play is the play of a "score".
	Play *ActivityPlay `json:"play,omitempty"`
}

// ActivityPlayer is who did it. Nickname and avatar are empty while the site has no copy of
// them (account, profiles.go).
type ActivityPlayer struct {
	ID        string `json:"id"`
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatarUrl"`
}

type ActivityChart struct {
	ID                string          `json:"id"`
	Title             string          `json:"title"`
	TitleTranslations json.RawMessage `json:"titleTranslations"`
}

type ActivityPlay struct {
	ID          string `json:"id"`
	Course      string `json:"course"`
	Side        string `json:"side"`
	Level       int    `json:"level"`
	Score       int64  `json:"score"`
	ClearStatus int    `json:"clearStatus"`
	// The chart block played: several plays of one by one player are one row, the latest.
	block int64
}

const (
	activityPlays   = 20
	activityUploads = 10
	// One player's evening of plays is not all the others get to see.
	activityPlaysEach = 4
	// How many of a source's latest plays are read to find activityPlays different ones.
	activityPlaysRead = 100
	// The answer is the same for everyone, and every visit to the home page asks.
	activityKept = 20 * time.Second
)

// sourceActivity reads one source's latest cleared plays and latest published charts, each
// newest first. Songs the official source lists without files are not uploads.
func (s *Server) sourceActivity(ctx context.Context, source Source) ([]Activity, []Activity, error) {
	schema := pgx.Identifier{source.ID}.Sanitize()
	rows, err := s.DB.Query(ctx, `SELECT sc.id,sc.user_id,sc.submitted_at,sc.score,sc.clear_status,sc.difficulty_id,d.course,d.side,d.level,c.id,c.title,c.title_translations
 FROM net.scores sc JOIN `+schema+`.chart_difficulties d ON d.id=sc.difficulty_id JOIN `+schema+`.charts c ON c.id=d.chart_id
 WHERE sc.source=$1 AND sc.clear_status>0 AND c.status='published'
 ORDER BY sc.submitted_at DESC,sc.id DESC LIMIT $2`, source.ID, activityPlaysRead)
	if err != nil {
		return nil, nil, err
	}
	plays := []Activity{}
	seen := map[string]bool{}
	var a Activity
	var p ActivityPlay
	var translations []byte
	if _, err = pgx.ForEachRow(rows, []any{&p.ID, &a.User.ID, &a.At, &p.Score, &p.ClearStatus, &p.block, &p.Course, &p.Side, &p.Level, &a.Chart.ID, &a.Chart.Title, &translations}, func() error {
		key := a.User.ID + "/" + strconv.FormatInt(p.block, 10)
		if seen[key] {
			return nil
		}
		seen[key] = true
		row, play := a, p
		row.Kind, row.Source, row.Play = "score", source.ID, &play
		row.Chart.TitleTranslations = json.RawMessage(translations)
		plays = append(plays, row)
		return nil
	}); err != nil {
		return nil, nil, err
	}
	rows, err = s.DB.Query(ctx, `SELECT c.id,c.owner_ids[1],c.created_at,c.title,c.title_translations FROM `+schema+`.charts c
 WHERE c.status='published' AND c.tja_file IS NOT NULL ORDER BY c.created_at DESC,c.id DESC LIMIT $1`, activityUploads)
	if err != nil {
		return nil, nil, err
	}
	uploads := []Activity{}
	a = Activity{}
	if _, err = pgx.ForEachRow(rows, []any{&a.Chart.ID, &a.User.ID, &a.At, &a.Chart.Title, &translations}, func() error {
		row := a
		row.Kind, row.Source = "upload", source.ID
		row.Chart.TitleTranslations = json.RawMessage(translations)
		uploads = append(uploads, row)
		return nil
	}); err != nil {
		return nil, nil, err
	}
	return plays, uploads, nil
}

// newestFirst sorts the rows by when they happened and keeps the first n.
func newestFirst(rows []Activity, n int) []Activity {
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].At.After(rows[j].At) })
	return rows[:min(n, len(rows))]
}

// recentActivity is the site's latest activity, newest first: the 20 latest cleared plays
// (one per player and chart block, four of a player at most) and the 10 latest uploads of
// all sources together. A source that cannot be read is left out.
func (s *Server) recentActivity(ctx context.Context) []Activity {
	plays, uploads := []Activity{}, []Activity{}
	for _, source := range s.Config.Sources {
		p, u, err := s.sourceActivity(ctx, source)
		if err != nil {
			log.Printf("activity on %s: %v", source.ID, err)
			continue
		}
		plays, uploads = append(plays, p...), append(uploads, u...)
	}
	shown, each := []Activity{}, map[string]int{}
	for _, play := range newestFirst(plays, len(plays)) {
		if each[play.User.ID] < activityPlaysEach && len(shown) < activityPlays {
			each[play.User.ID]++
			shown = append(shown, play)
		}
	}
	rows := newestFirst(append(shown, newestFirst(uploads, activityUploads)...), activityPlays+activityUploads)
	ids, seen := []string{}, map[string]bool{}
	for _, row := range rows {
		if !seen[row.User.ID] {
			seen[row.User.ID] = true
			ids = append(ids, row.User.ID)
		}
	}
	names := s.profiles(ctx, ids)
	for i := range rows {
		p := names[rows[i].User.ID]
		rows[i].User.Nickname, rows[i].User.AvatarURL = p.Nickname, p.AvatarURL
	}
	return rows
}

// profiles is what the site shows of each player: the copy it keeps of their nickname and
// avatar. A player it has nothing of is left out; so is everyone while it cannot be read.
// Without an account center only the development user has a name.
func (s *Server) profiles(ctx context.Context, ids []string) map[string]account.PublicProfile {
	if len(ids) == 0 {
		return nil
	}
	if s.Config.SSO == nil {
		names := map[string]account.PublicProfile{}
		for _, id := range ids {
			if id == s.Config.DevUserID {
				names[id] = account.PublicProfile{Nickname: devUser(id).Nickname}
			}
		}
		return names
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	names, err := s.Config.SSO.Profiles(ctx, ids)
	if err != nil {
		log.Printf("activity: players' names: %v", err)
		return nil
	}
	return names
}

// activity is GET /api/v1/activity, public: what the home page's community list shows. The
// answer is kept for a few seconds and given to everyone who asks meanwhile.
func (s *Server) activity(w http.ResponseWriter, r *http.Request) {
	s.feedMu.Lock()
	defer s.feedMu.Unlock()
	if s.feed == nil || time.Since(s.feedAt) > activityKept {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		rows := s.recentActivity(ctx)
		// A reader who left mid-way read nothing worth keeping.
		if ctx.Err() == nil {
			s.feed, s.feedAt = rows, time.Now()
		} else if s.feed == nil {
			web.Respond(w, 200, map[string]any{"items": rows})
			return
		}
	}
	web.Respond(w, 200, map[string]any{"items": s.feed})
}
