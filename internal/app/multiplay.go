package app

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand/v2"
	apiv1 "ourtaiko.dev/net/internal/api/v1"

	"github.com/jackc/pgx/v5"
	"ourtaiko.dev/net/internal/multiplay"
	songs "ourtaiko.dev/net/internal/songs/httpapi"
)

// Multi-Enso (internal/multiplay), the game's online rooms, runs either in this program on a
// port of its own (development) or as the program cmd/rooms (production), which then asks this
// one over package roomsapi. This file is what it is given of the rest: who a game session
// belongs to, and the songs of both sources.

// Multiplay configures Multi-Enso. With both addresses empty the service is off.
type Multiplay struct {
	// ListenAddr is the host:port the gRPC service listens on in this program, in clear text;
	// the reverse proxy in front of it ends TLS.
	ListenAddr string
	// InternalAddr is the host:port cmd/rooms asks for players and songs, when the service
	// runs there. Never give it to the reverse proxy. Not both this and ListenAddr.
	InternalAddr string
	// PublicURL is what the game is told to dial (GET /api/v1/game/multiplay); empty is the
	// site's Origin.
	PublicURL string
	// MinPlayers is the fewest players a round needs; zero is the default (2). One lets a
	// developer try a round alone.
	MinPlayers int
}

// gamePlayer answers for a game session as the game routes of a source do: both sources go
// by the one account center, so either can be asked.
func gamePlayer(source *songs.Server) multiplay.Authenticator {
	return func(ctx context.Context, token string) (multiplay.Player, error) {
		u, err := source.GameIdentity(ctx, token)
		if errors.Is(err, pgx.ErrNoRows) {
			return multiplay.Player{}, multiplay.ErrUnauthenticated
		}
		if err != nil {
			return multiplay.Player{}, err
		}
		// The nickname only: a login name is told to nobody but its owner, so a player
		// without a nickname has none here either and the game shows a placeholder.
		return multiplay.Player{UserID: u.ID, Nickname: u.Nickname, AvatarURL: u.AvatarURL}, nil
	}
}

// roomSongs is the songs rooms can play: the playable charts of the sources.
type roomSongs struct {
	sources []roomSource
	// intn is a random number in [0, n).
	intn func(n int) int
}

type roomSource struct {
	id     string
	server *songs.Server
}

func newRoomSongs(fanmade, official *songs.Server) roomSongs {
	return roomSongs{sources: []roomSource{{"fanmade", fanmade}, {"official", official}}, intn: rand.IntN}
}

// roomSong describes a chart to a room: its JSON is what the source's GET /charts/{id}
// answers, and its courses are those of its blocks, easiest first, each with the level of
// its single-play block when it has one, else of its first block. Nil for a chart without
// blocks, which nobody can play.
func roomSong(source string, c songs.Chart) (*multiplay.Song, error) {
	body, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	// Released games read the chart as version 1 of the API gives it.
	body = apiv1.ConvertJSON(body)
	song := &multiplay.Song{Source: source, ChartID: c.ID, Title: c.Title, Subtitle: c.Subtitle, ChartJSON: body}
	for _, name := range multiplay.CourseNames() {
		level, found := 0, false
		for _, d := range c.Difficulties {
			if d.Course == name && (!found || d.Side == "single") {
				level, found = d.Level, true
				if d.Side == "single" {
					break
				}
			}
		}
		if found {
			song.Courses = append(song.Courses, multiplay.Course{Name: name, Level: level})
		}
	}
	if len(song.Courses) == 0 {
		return nil, nil
	}
	return song, nil
}

// Get is the chart of a source when the game can play it.
func (s roomSongs) Get(ctx context.Context, source, chartID string) (*multiplay.Song, error) {
	for _, src := range s.sources {
		if src.id != source {
			continue
		}
		c, err := src.server.PlayableChart(ctx, chartID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, multiplay.ErrChartNotFound
		}
		if err != nil {
			return nil, err
		}
		song, err := roomSong(source, c)
		if err == nil && song == nil {
			err = multiplay.ErrChartNotFound
		}
		return song, err
	}
	return nil, multiplay.ErrChartNotFound
}

// Random is any chart the game's catalog lists, each with the same chance whichever source
// it is in. Nil when there is none.
func (s roomSongs) Random(ctx context.Context) (*multiplay.Song, error) {
	// The charts are counted, then one is read by its place: a chart withdrawn in between
	// can leave that place empty, and the draw is made again.
	for try := 0; try < 3; try++ {
		counts, total := make([]int, len(s.sources)), 0
		for i, src := range s.sources {
			n, err := src.server.GameChartCount(ctx)
			if err != nil {
				return nil, err
			}
			counts[i] = n
			total += n
		}
		if total == 0 {
			return nil, nil
		}
		n := s.intn(total)
		for i, src := range s.sources {
			if n >= counts[i] {
				n -= counts[i]
				continue
			}
			c, err := src.server.GameChartAt(ctx, n)
			if errors.Is(err, pgx.ErrNoRows) {
				break
			}
			if err != nil {
				return nil, err
			}
			if song, err := roomSong(src.id, c); err != nil || song != nil {
				return song, err
			}
			break
		}
	}
	return nil, errors.New("no chart could be drawn")
}
