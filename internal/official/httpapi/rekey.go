package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
)

// fetch copies a stored file into path and verifies it against its record (as Fanmade's).
func (s *Server) fetch(ctx context.Context, key, sha string, size int64, path string) error {
	in, err := s.Config.Objects.Open(ctx, key)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, h), io.LimitReader(in, size+1))
	if err = errors.Join(err, out.Close()); err != nil {
		return err
	}
	if n != size || hex.EncodeToString(h.Sum(nil)) != sha {
		return fmt.Errorf("stored file integrity mismatch: %s", key)
	}
	return nil
}
