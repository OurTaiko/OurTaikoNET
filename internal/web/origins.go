package web

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Origins are the addresses the one website is served at: its own (APP_ORIGIN) and others
// (APP_OTHER_ORIGINS), such as a host name in mainland China whose proxy passes everything
// to this server. Every address is the same site, the same database and the same sign-in;
// a browser keeps a session of each apart, by the cookies of its host.
//
// A request is answered in the origin of the host it came to (Handler, Origin): a write must
// come from that origin's pages, sign-in returns there, and the full addresses an answer gives
// are on it. A request that came to no listed host, such as one to an old site's host name,
// is answered in the site's own origin.
type Origins struct {
	primary string
	// byHost is each origin by its host (hostKey).
	byHost map[string]string
}

type originKey struct{}

// NewOrigins is the site at primary (APP_ORIGIN) and others (APP_OTHER_ORIGINS): origins, each
// a scheme and host, with a port if not the scheme's, and nothing else.
func NewOrigins(primary string, others []string) (Origins, error) {
	o := Origins{primary: strings.TrimRight(primary, "/"), byHost: map[string]string{}}
	if u, err := url.Parse(o.primary); err == nil && u.Host != "" {
		o.byHost[hostKey(u.Host)] = o.primary
	}
	for _, raw := range others {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil ||
			strings.TrimRight(u.Path, "/") != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
			return Origins{}, fmt.Errorf("APP_OTHER_ORIGINS must hold origins such as https://example.org, not %q", raw)
		}
		key := hostKey(u.Host)
		if _, seen := o.byHost[key]; seen {
			return Origins{}, fmt.Errorf("APP_OTHER_ORIGINS repeats the host %s", u.Host)
		}
		o.byHost[key] = u.Scheme + "://" + strings.ToLower(u.Host)
	}
	return o, nil
}

// Primary is the site's own origin, APP_ORIGIN.
func (o Origins) Primary() string { return o.primary }

// Others are the origins besides the site's own, in no particular order.
func (o Origins) Others() []string {
	var others []string
	for _, origin := range o.byHost {
		if origin != o.primary {
			others = append(others, origin)
		}
	}
	return others
}

// Handler serves next with each request's origin (Origin): the listed one whose host the
// request came to. The proxy in front must pass the Host it was asked for.
func (o Origins) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin, ok := o.byHost[hostKey(r.Host)]; ok {
			r = r.WithContext(context.WithValue(r.Context(), originKey{}, origin))
		}
		next.ServeHTTP(w, r)
	})
}

// Origin is the origin the request of ctx came to (Handler), or the site's own, fallback,
// when it came to none of them or nothing told it.
func Origin(ctx context.Context, fallback string) string {
	if origin, ok := ctx.Value(originKey{}).(string); ok {
		return origin
	}
	return strings.TrimRight(fallback, "/")
}

// hostKey is a host as origins are looked up by: lower case, without a default port (which
// a client may or may not write, and a proxy that ends TLS cannot tell apart).
func hostKey(host string) string {
	host = strings.ToLower(host)
	for _, port := range []string{":443", ":80"} {
		host = strings.TrimSuffix(host, port)
	}
	return host
}
