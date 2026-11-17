// Package requester carries who a request turned out to be from, for code that runs around
// the handlers and cannot tell by itself.
//
// Whoever authenticates a request calls Note once they know the player; code wrapping the
// handler (a middleware) reads Of after the handler has run, or as it writes the answer. The
// two sides know nothing of each other: this package is all they share.
package requester

import (
	"context"
	"sync"
)

type slot struct {
	mu   sync.Mutex
	user string
}

type key struct{}

// With gives a request's context a place for its player. Without it, Note does nothing and Of
// is "".
func With(ctx context.Context) context.Context {
	return context.WithValue(ctx, key{}, &slot{})
}

// Note says that the request comes from the player id. The last call wins.
func Note(ctx context.Context, id string) {
	if s, ok := ctx.Value(key{}).(*slot); ok {
		s.mu.Lock()
		s.user = id
		s.mu.Unlock()
	}
}

// Of is the player the request was noted to come from, or "".
func Of(ctx context.Context) string {
	s, ok := ctx.Value(key{}).(*slot)
	if !ok {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.user
}
