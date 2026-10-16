package claudeconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ad/clauds/internal/account"
)

func TestEnsureLinksSharedAndKeepsStatePrivate(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, ".claude")
	srcJSON := filepath.Join(root, ".claude.json")
	for _, d := range []string{"skills/foo", "projects/p", "agents"} {
		os.MkdirAll(filepath.Join(src, d), 0755)
	}
	os.WriteFile(filepath.Join(src, "settings.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(src, "CLAUDE.md"), []byte("hi"), 0644)
	os.WriteFile(filepath.Join(src, "history.jsonl"), []byte(""), 0644)
	os.WriteFile(srcJSON, []byte(`{"userID":"secret","mcpServers":{"x":{"command":"y"}}}`), 0600)

	acct := &account.Account{Dir: filepath.Join(root, "acct")}
	for range 2 {
		if err := Ensure(acct, src, srcJSON); err != nil {
			t.Fatal(err)
		}
	}

	dir := Dir(acct)
	for _, n := range []string{"skills", "agents", "settings.json", "CLAUDE.md"} {
		if target, err := os.Readlink(filepath.Join(dir, n)); err != nil || target != filepath.Join(src, n) {
			t.Errorf("%s: want link to %s, got %q %v", n, filepath.Join(src, n), target, err)
		}
	}
	for _, n := range []string{"projects", "history.jsonl"} {
		if _, err := os.Lstat(filepath.Join(dir, n)); err == nil {
			t.Errorf("%s must not be shared", n)
		}
	}

	raw, _ := os.ReadFile(filepath.Join(dir, ".claude.json"))
	var seeded map[string]any
	json.Unmarshal(raw, &seeded)
	if _, ok := seeded["mcpServers"]; !ok || len(seeded) != 1 {
		t.Errorf("seeded .claude.json = %s", raw)
	}

	if d := Drifted(acct, src); len(d) != 0 {
		t.Errorf("unexpected drift %v", d)
	}
	os.Remove(filepath.Join(dir, "settings.json"))
	os.WriteFile(filepath.Join(dir, "settings.json"), []byte("{}"), 0644)
	if d := Drifted(acct, src); !slices.Equal(d, []string{"settings.json"}) {
		t.Errorf("drift = %v", d)
	}

	if err := account.Delete(acct); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(src, "skills", "foo")); err != nil {
		t.Fatalf("removing the account deleted shared skills: %v", err)
	}
}
