package main

import (
	"os/exec"
	"strings"
	"testing"
)

// The deploy workflow restarts the rooms only when cmd/rooms or internal/multiplay changed. A
// package of this module outside those would change the rooms without the workflow knowing.
func TestRoomsDependOnNothingElseOfThisModule(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range strings.Fields(string(out)) {
		if strings.HasPrefix(pkg, "ourtaiko.dev/net/") && !strings.HasPrefix(pkg, "ourtaiko.dev/net/cmd/rooms") && !strings.HasPrefix(pkg, "ourtaiko.dev/net/internal/multiplay") {
			t.Errorf("cmd/rooms depends on %s: add its path to the rooms' part in .github/workflows/deploy.yml, or do without it", pkg)
		}
	}
}
