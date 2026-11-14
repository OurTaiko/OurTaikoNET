package objectstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// ErrReadOnly is the answer of a store that only reads to a write.
var ErrReadOnly = errors.New("the file store is read-only")

// NewPublic is a store that only reads, over the public address of a prefix. It is what a
// second copy of the site, running beside the one that owns the files, is given: it holds no
// credentials, so nothing it does can change or remove a file. Put and Delete answer
// ErrReadOnly, and links are always the public ones.
//
// It is an *S3 so that everything that tells a bucket from a folder (links to the public
// address, the prefix in front of preview clips) treats it as the bucket it reads.
func NewPublic(prefix, base string) (*S3, error) {
	prefix = strings.TrimSuffix(prefix, "/")
	if !ValidKey(prefix) || base == "" {
		return nil, errors.New("object prefix and its public address required")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 30 * time.Second
	s := &S3{Prefix: prefix + "/", http: &http.Client{Transport: transport}}
	return s, s.SetPublicBaseURL(base)
}

// ReadOnly reports a store made by NewPublic.
func (s *S3) ReadOnly() bool { return s.Client == nil }

func (s *S3) openPublic(ctx context.Context, key string) (Object, error) {
	address, e := s.PublicURL(key)
	if e != nil {
		return nil, e
	}
	if address == "" || s.http == nil {
		return nil, errors.New("the read-only file store has no public address")
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodHead, address, nil)
	if e != nil {
		return nil, e
	}
	res, e := s.http.Do(req)
	if e != nil {
		return nil, e
	}
	res.Body.Close()
	switch {
	// A bucket that may not be listed answers a missing key with 403, as in Open.
	case res.StatusCode == http.StatusNotFound || res.StatusCode == http.StatusForbidden:
		return nil, os.ErrNotExist
	case res.StatusCode != http.StatusOK || res.ContentLength < 0:
		return nil, fmt.Errorf("public file address answered %d", res.StatusCode)
	}
	modified, _ := http.ParseTime(res.Header.Get("Last-Modified"))
	etag := res.Header.Get("ETag")
	f := &remoteObject{ctx: ctx, size: res.ContentLength, modified: modified}
	f.fetch = func(pos int64) (io.ReadCloser, error) {
		req, e := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
		if e != nil {
			return nil, e
		}
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", pos))
		// The same pin as a bucket read: bytes of two versions are never mixed.
		if etag != "" {
			req.Header.Set("If-Match", etag)
		}
		res, e := s.http.Do(req)
		if e != nil {
			return nil, e
		}
		if res.StatusCode != http.StatusPartialContent && !(res.StatusCode == http.StatusOK && pos == 0) {
			res.Body.Close()
			return nil, fmt.Errorf("public file address answered %d to a ranged read", res.StatusCode)
		}
		return res.Body, nil
	}
	return f, nil
}
