package files

import "testing"

func TestPaths(t *testing.T) {
	for address, want := range map[string]string{
		Root + "fanmade/" + "a.webp":         "fanmade/a.webp",
		Root + "sso/avatars/a/b.webp":        "sso/avatars/a/b.webp",
		Root + "3dchara/cos/0.glb":           "3dchara/cos/0.glb",
		Root + "fanmade/a.webp?v=1":          "",
		Root + "fanmade/../secret":           "",
		Root:                                 "",
		"https://elsewhere.example/a.webp":   "",
		"https://distrib.ourtaiko.org.evil/": "",
		"http://distrib.ourtaiko.org/a.webp": "",
	} {
		if got := Path(address); got != want {
			t.Errorf("Path(%q) = %q, want %q", address, got, want)
		}
	}
	if URL("", "fanmade/a.webp") != Root+"fanmade/a.webp" || URL("https://m.example/ourtaiko/", "x") != "https://m.example/ourtaiko/x" || URL("", "") != "" {
		t.Fatal("URL")
	}
}
