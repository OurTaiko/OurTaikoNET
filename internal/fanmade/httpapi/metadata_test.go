package httpapi

import (
	"encoding/json"
	"strings"
	"testing"
)

// A translation is replaced by the text sent and removed by null or "": the site's
// translations are the chart's own, not a layer over the TJA's.
func TestPatchTranslations(t *testing.T) {
	titles := map[string]string{"en": "Edited", "ja": "旧名", "zh": "中文"}
	if !patchTranslations(json.RawMessage(`{"ja":" 新名 ","zh":null,"en":"","ko":"한국어"}`), titles) || len(titles) != 2 || titles["ja"] != "新名" || titles["ko"] != "한국어" {
		t.Fatalf("patched %+v", titles)
	}
	// Nothing sent, or null for the whole set, leaves it alone.
	for _, raw := range []string{``, `null`, `{}`} {
		if !patchTranslations(json.RawMessage(raw), titles) || len(titles) != 2 {
			t.Fatalf("%q changed %+v", raw, titles)
		}
	}
	for _, raw := range []string{`{"de":"Unsupported"}`, `[]`, `{"ja":12}`, `{"ja":"a\u0000b"}`, `{"ja":"` + strings.Repeat("x", 501) + `"}`} {
		if patchTranslations(json.RawMessage(raw), map[string]string{}) {
			t.Fatalf("accepted %s", raw)
		}
	}
}
