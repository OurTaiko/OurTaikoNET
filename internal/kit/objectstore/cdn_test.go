package objectstore

import "testing"

func TestPublicURL(t *testing.T) {
	s := &S3{}
	for _, raw := range []string{"http://cdn.example/fanmade", "https://u:p@cdn.example/fanmade", "https://cdn.example/f?x=1", "https://cdn.example/f#x", "https://cdn.example/f?", "https://cdn.example/a b"} {
		if s.SetPublicBaseURL(raw) == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
	if err := s.SetPublicBaseURL("https://cdn.example/fanmade/"); err != nil {
		t.Fatal(err)
	}
	got, err := s.PublicURL("objects/abc/song #?.ogg")
	if err != nil || got != "https://cdn.example/fanmade/objects/abc/song%20%23%3F.ogg" {
		t.Fatalf("%q %v", got, err)
	}
	for _, key := range []string{"../private", "/private", "objects/../secret", "objects\\secret"} {
		if _, err := s.PublicURL(key); err == nil {
			t.Fatalf("accepted key %q", key)
		}
	}
	s.SetPublicBaseURL("")
	if got, err := s.PublicURL("objects/abc/audio"); got != "" || err != nil {
		t.Fatal(got, err)
	}
}

// Each store may sign with its own key; half a key is a mistake in the settings.
func TestAStoreSignsWithItsOwnKey(t *testing.T) {
	ctx := t.Context()
	if _, err := NewS3As(ctx, "bucket", "ap-northeast-1", "fanmade", "AKIAEXAMPLE", ""); err == nil {
		t.Fatal("accepted a key without its secret")
	}
	a, err := NewS3As(ctx, "bucket", "ap-northeast-1", "fanmade", "AKIAFANMADE", "secret-one")
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewS3As(ctx, "bucket", "ap-northeast-1", "database", "AKIAOFFICIAL", "secret-two")
	if err != nil {
		t.Fatal(err)
	}
	for store, want := range map[*S3]string{a: "AKIAFANMADE", b: "AKIAOFFICIAL"} {
		got, err := store.Client.Options().Credentials.Retrieve(ctx)
		if err != nil || got.AccessKeyID != want {
			t.Fatal(got.AccessKeyID, err)
		}
		if store.ReadOnly() {
			t.Fatal("a store with a key is not read-only")
		}
	}
}
