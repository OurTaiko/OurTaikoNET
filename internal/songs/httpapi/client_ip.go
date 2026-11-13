package httpapi

import (
	"net/http"

	"ourtaiko.dev/net/internal/web"
)

// Whose request this is, is found the same way everywhere on the site (package web).
var ParseTrustedProxies = web.ParseTrustedProxies

func (s *Server) clientIP(r *http.Request) string { return web.ClientIP(r, s.Config.TrustedProxies) }
