package distribution

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

var playerPath = regexp.MustCompile(`^/player/[a-f0-9]{16}/index\.html$`)
var playerFile = regexp.MustCompile(`^[a-zA-Z0-9_-][a-zA-Z0-9_.-]*(/[a-zA-Z0-9_-][a-zA-Z0-9_.-]*)*$`)
var sha256Value = regexp.MustCompile(`^[a-f0-9]{64}$`)

type playerBuild struct {
	Path  string            `json:"path"`
	Files map[string]string `json:"files"`
}

type playerSnapshot struct {
	canonical *playerBuild
	mirror    *playerBuild
}

type playerCache struct {
	mu    sync.Mutex
	value playerSnapshot
	until time.Time
	done  chan struct{}
}

func (r *Router) readPlayer(address string) (*playerBuild, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	client := r.Client
	if client == nil {
		client = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("player manifest unavailable")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return nil, errors.New("player manifest exceeds limit")
	}
	var build playerBuild
	if json.Unmarshal(data, &build) != nil || !playerPath.MatchString(build.Path) || len(build.Files) == 0 || len(build.Files) > 4096 || !sha256Value.MatchString(build.Files["index.html"]) {
		return nil, errors.New("invalid player manifest")
	}
	for path, digest := range build.Files {
		if !playerFile.MatchString(path) || strings.Contains(path, "..") || !sha256Value.MatchString(digest) {
			return nil, errors.New("invalid player file")
		}
	}
	return &build, nil
}

func (r *Router) playerNow(ctx context.Context) playerSnapshot {
	r.player.mu.Lock()
	value := r.player.value
	if time.Now().Before(r.player.until) {
		r.player.mu.Unlock()
		return value
	}
	done := r.player.done
	if done == nil {
		done = make(chan struct{})
		r.player.done = done
		go func() {
			canonical, err := r.readPlayer(CanonicalBase + "player-build.json")
			var mirror *playerBuild
			if err == nil && r.Base != "" {
				mirror, _ = r.readPlayer(strings.TrimRight(r.Base, "/") + "/player-build.json")
			}
			r.player.mu.Lock()
			if err == nil {
				r.player.value = playerSnapshot{canonical, mirror}
			}
			ttl := time.Minute
			if err != nil {
				ttl = 15 * time.Second
			}
			r.player.until = time.Now().Add(ttl)
			r.player.done = nil
			close(done)
			r.player.mu.Unlock()
		}()
	}
	r.player.mu.Unlock()
	if value.canonical != nil {
		return value
	}
	select {
	case <-done:
	case <-ctx.Done():
		return playerSnapshot{}
	}
	r.player.mu.Lock()
	defer r.player.mu.Unlock()
	return r.player.value
}

func (r *Router) playerAnswer(w http.ResponseWriter, request *http.Request) {
	ctx, cancel := context.WithTimeout(request.Context(), 4*time.Second)
	defer cancel()
	value := r.playerNow(ctx)
	if value.canonical == nil {
		http.Error(w, `{"error":"PLAYER_MANIFEST_UNAVAILABLE"}`, http.StatusServiceUnavailable)
		return
	}
	base := CanonicalBase
	if r.Base != "" && r.IsMainland(request) && value.mirror != nil && value.mirror.Path == value.canonical.Path && maps.Equal(value.mirror.Files, value.canonical.Files) {
		base = strings.TrimRight(r.Base, "/") + "/"
	}
	json.NewEncoder(w).Encode(struct {
		*playerBuild
		BaseURL         string `json:"baseUrl"`
		FallbackBaseURL string `json:"fallbackBaseUrl"`
	}{value.canonical, base, CanonicalBase})
}
