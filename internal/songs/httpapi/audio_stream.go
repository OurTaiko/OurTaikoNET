package httpapi

import (
	"net/http"

	"ourtaiko.dev/net/internal/kit/audio"
)

func (s *Server) streamAudio(w http.ResponseWriter, r *http.Request) {
	if key, name, digest, ok := s.publishedFile(w, r, "audio_file"); ok {
		w.Header().Set("X-Accel-Buffering", "no")
		s.serveObject(w, r, key, name, digest, audio.MediaType(name), "inline")
	}
}
