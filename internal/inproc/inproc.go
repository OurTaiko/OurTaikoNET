// Package inproc lets one part of the program call another through its HTTP API without a
// network.
//
// The services OurTaikoNet is made from talked to each other over HTTP. They now run in one
// program, and the calls that are still made that way (the song sources asking the players'
// part about titles and awards) are delivered to the handler directly by this transport.
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
