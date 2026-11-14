package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ourtaiko.dev/net/internal/kit/objectstore"
)

// A server whose files are only read turns an upload away at the door; one that owns its
// files lets it through.
func TestAReadOnlyStoreTurnsUploadsAway(t *testing.T) {
	public, err := objectstore.NewPublic("fanmade", "https://cdn.example/fanmade")
	if err != nil {
		t.Fatal(err)
	}
	reached := false
	next := func(w http.ResponseWriter, r *http.Request) { reached = true }

	s := &Server{}
	s.Config.Objects = public
	w := httptest.NewRecorder()
	s.storing(next)(w, httptest.NewRequest("POST", "/api/v1/charts", nil))
	if reached || w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "FILES_READ_ONLY") {
		t.Fatal(reached, w.Code, w.Body.String())
	}
	if !s.ReadOnlyFiles() {
		t.Fatal("a public store is read-only")
	}

	s.Config.Objects = objectstore.Local{Root: t.TempDir()}
	s.storing(next)(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/v1/charts", nil))
	if !reached || s.ReadOnlyFiles() {
		t.Fatal(reached, s.ReadOnlyFiles())
	}
}
