package web

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

// Problems is how a part of the site answers a request it cannot serve: a code for programs,
// a message for people and the request's ID (the X-Request-ID the part set on the response).
// Extra are fields a part adds to every such answer.
type Problems struct {
	Extra map[string]any
}

// Problem answers with the given status, code and message.
func (p Problems) Problem(w http.ResponseWriter, status int, code, message string) {
	body := map[string]any{"code": code, "message": message, "requestId": w.Header().Get("X-Request-ID")}
	for field, value := range p.Extra {
		body[field] = value
	}
	Respond(w, status, body)
}

// Internal answers that the service is unavailable and logs why, under the request's ID. The
// client is not told the reason.
func (p Problems) Internal(w http.ResponseWriter, err error) {
	log.Printf("request=%s error=%v", w.Header().Get("X-Request-ID"), err)
	p.Problem(w, 503, "SERVICE_UNAVAILABLE", "服务暂时不可用，请稍后重试")
}

// Decode reads a JSON request body of at most 64 KiB into v. It reports false after
// answering: the body must be sent as JSON, be well formed and name no field v does not have.
func (p Problems) Decode(w http.ResponseWriter, r *http.Request, v any) bool {
	return p.DecodeLimit(w, r, v, 64*1024)
}

// DecodeLimit is Decode with another limit on the body's size, in bytes.
func (p Problems) DecodeLimit(w http.ResponseWriter, r *http.Request, v any, limit int64) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		p.Problem(w, 415, "CONTENT_TYPE_INVALID", "请使用 JSON 请求")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		p.Problem(w, 400, "REQUEST_INVALID", "请求格式不正确")
		return false
	}
	return true
}
