package httpapi

import (
	"context"
	"testing"

	"ourtaiko.dev/net/internal/netdb/netdbtest"
	"ourtaiko.dev/net/internal/web"
)

// interactionWorld is a source with the four test users signed in (testUsers) and two charts
// of owner's: "Song", two days old, and "Other". Tests of what both sources do alike are
// written against it and run once for each (bothSources).
type interactionWorld struct {
	c           testClient
	s           *Server
	song, other string
}

func (w interactionWorld) chart(id string) string {
	return "/api/v1/charts/" + id
}

// interactionWorldOf makes the world on a source. On the official source the two charts are
// registered without files; a Fanmade chart always has its files.
func interactionWorldOf(t *testing.T, src *Source) interactionWorld {
	t.Helper()
	schema := "fanmade"
	if src.Official {
		schema = "official"
	}
	pool := netdbtest.New(t, schema)
	s := New(pool, Config{Source: src, Storage: t.TempDir(), Origin: "http://localhost", SSO: fakeSSO(t)})
	w := interactionWorld{c: newTestClient(t, s), s: s}
	ctx := context.Background()
	owner := testUsers["owner"].ID
	var tja, audio *int64
	duration := 0
	if !src.Official {
		duration = 10
		err := pool.QueryRow(ctx, `INSERT INTO chart_resources(storage_key,original_filename,sha256,byte_size,extension) VALUES('song/tja','a.tja',repeat('a',64),14,'tja') RETURNING id`).Scan(&tja)
		if err == nil {
			err = pool.QueryRow(ctx, `INSERT INTO chart_resources(storage_key,original_filename,sha256,byte_size,extension) VALUES('song/audio','a.ogg',repeat('a',64),14,'ogg') RETURNING id`).Scan(&audio)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	err := pool.QueryRow(ctx, `INSERT INTO charts(id,owner_ids,title,bpm,duration,demo_end,tja_file,audio_file,created_at) VALUES($2,ARRAY[$1::text],'Song',120,$3,$3,$4,$5,now()-interval '2 days') RETURNING id`, owner, web.NewID(), duration, tja, audio).Scan(&w.song)
	if err == nil {
		err = pool.QueryRow(ctx, `INSERT INTO charts(id,owner_ids,title,bpm,duration,demo_end,tja_file,audio_file) VALUES($2,ARRAY[$1::text],'Other',120,$3,$3,$4,$5) RETURNING id`, owner, web.NewID(), duration, tja, audio).Scan(&w.other)
	}
	if err != nil {
		t.Fatal(err)
	}
	return w
}

// officialInteractionFixture is the world on the official source.
func officialInteractionFixture(t *testing.T) interactionWorld {
	t.Helper()
	return interactionWorldOf(t, Official)
}

// bothSources runs a test of what the sources do alike once on each.
func bothSources(t *testing.T, test func(t *testing.T, w interactionWorld)) {
	for name, src := range map[string]*Source{"fanmade": Fanmade, "official": Official} {
		t.Run(name, func(t *testing.T) { test(t, interactionWorldOf(t, src)) })
	}
}
