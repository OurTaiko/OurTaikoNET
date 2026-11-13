package web

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProblems(t *testing.T) {
	answer := func(send func(w *httptest.ResponseRecorder)) (int, map[string]any) {
		w := httptest.NewRecorder()
		w.Header().Set("X-Request-ID", "request-1")
		send(w)
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err, w.Body.String())
		}
		return w.Code, body
	}
	plain, versioned := Problems{}, Problems{Extra: map[string]any{"validationVersion": "tja-v1"}}
	status, body := answer(func(w *httptest.ResponseRecorder) { plain.Problem(w, 404, "NOT_FOUND", "nothing here") })
	if status != 404 || len(body) != 3 || body["code"] != "NOT_FOUND" || body["message"] != "nothing here" || body["requestId"] != "request-1" {
		t.Fatal(status, body)
	}
	// A part's own fields are on every answer, the unavailable one included, which does not
	// say why.
	status, body = answer(func(w *httptest.ResponseRecorder) { versioned.Internal(w, errors.New("the database is away")) })
	if status != 503 || body["code"] != "SERVICE_UNAVAILABLE" || body["validationVersion"] != "tja-v1" || strings.Contains(body["message"].(string), "database") {
		t.Fatal(status, body)
	}

	var into struct{ Name string }
	decode := func(contentType, text string, limit int64) (bool, int, map[string]any) {
		var ok bool
		status, body := answer(func(w *httptest.ResponseRecorder) {
			r := httptest.NewRequest("POST", "/", strings.NewReader(text))
			r.Header.Set("Content-Type", contentType)
			if ok = plain.DecodeLimit(w, r, &into, limit); ok {
				Respond(w, 200, map[string]any{})
			}
		})
		return ok, status, body
	}
	if ok, _, _ := decode("application/json; charset=utf-8", `{"name":"a"}`, 64); !ok || into.Name != "a" {
		t.Fatal("a JSON body was not read", into)
	}
	for name, c := range map[string]struct {
		contentType, text string
		status            int
		code              string
	}{
		"not sent as JSON": {"text/plain", `{"name":"a"}`, 415, "CONTENT_TYPE_INVALID"},
		"not well formed":  {"application/json", `{"name":`, 400, "REQUEST_INVALID"},
		"an unknown field": {"application/json", `{"name":"a","other":1}`, 400, "REQUEST_INVALID"},
		"too large":        {"application/json", `{"name":"` + strings.Repeat("a", 100) + `"}`, 400, "REQUEST_INVALID"},
	} {
		if ok, status, body := decode(c.contentType, c.text, 64); ok || status != c.status || body["code"] != c.code {
			t.Fatal(name, ok, status, body)
		}
	}
}
