package distribution

import "net/http"

// SourceAnswer tells a client, once, which root this visitor's public files come from, so it
// can join every path the API gives (package files) to it.
type SourceAnswer struct {
	Mirror           bool   `json:"mirror"`
	BaseURL          string `json:"baseUrl"`
	CanonicalBaseURL string `json:"canonicalBaseUrl"`
}

// Source uses the same country and trusted-proxy policy as every other mirror choice. It does
// not probe: a client falls back to the canonical root when a mirror object fails to load.
func (r *Router) Source(request *http.Request) SourceAnswer {
	if base := r.MirrorBase(request); base != "" {
		return SourceAnswer{Mirror: true, BaseURL: base, CanonicalBaseURL: CanonicalBase}
	}
	return SourceAnswer{BaseURL: CanonicalBase, CanonicalBaseURL: CanonicalBase}
}
