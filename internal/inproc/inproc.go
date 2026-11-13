// Package inproc lets one part of the program call another through its HTTP API without a
// network.
//
// The three services OurTaikoNet is made from talked to each other over HTTP: Profile read
// the song sites' scores, the song sites asked Profile for awards. They now run in one
// program and still make those calls; this transport delivers them to the handler directly.
// It is a stage, not the destination: each such call is to become an ordinary function call
// or a query, and when the last one has, this package goes.
package inproc

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
)

// Transport answers every request with handler, whatever host it names.
type Transport struct{ Handler http.Handler }

func (t Transport) RoundTrip(r *http.Request) (*http.Response, error) {
	inner := r.Clone(r.Context())
	inner.RequestURI = r.URL.RequestURI()
	inner.RemoteAddr = "127.0.0.1:0"
	if r.Body == nil {
		inner.Body = http.NoBody
	}
	rec := httptest.NewRecorder()
	t.Handler.ServeHTTP(rec, inner)
	res := rec.Result()
	res.Request = r
	// The body is complete already; hand it over as a plain reader.
	body, _ := io.ReadAll(res.Body)
	res.Body = io.NopCloser(bytes.NewReader(body))
	return res, nil
}

// Hosts answers a request with the handler registered for its host name.
type Hosts map[string]http.Handler

func (h Hosts) RoundTrip(r *http.Request) (*http.Response, error) {
	handler, ok := h[r.URL.Hostname()]
	if !ok {
		handler = http.NotFoundHandler()
	}
	return Transport{handler}.RoundTrip(r)
}
