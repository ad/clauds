package skills

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ad/clauds/internal/account"
)

func TestSyncCopiesOnlyUserSkills(t *testing.T) {
	root := t.TempDir()
	acct := &account.Account{Dir: filepath.Join(root, "acct"), Meta: account.Meta{Name: "a"}}
	plugin := filepath.Join(acct.DataDir(), "local-agent-mode-sessions", "skills-plugin", "org", "acc")
	for _, n := range []string{"mine", "pdf", "kept"} {
		os.MkdirAll(filepath.Join(plugin, "skills", n), 0755)
		os.WriteFile(filepath.Join(plugin, "skills", n, "SKILL.md"), []byte(n), 0644)
	}
	os.WriteFile(filepath.Join(plugin, "manifest.json"), []byte(`{"skills":[
		{"name":"mine","creatorType":"user"},
		{"name":"pdf","creatorType":"anthropic"},
		{"name":"kept","creatorType":"user"},
		{"name":"../evil","creatorType":"user"}]}`), 0644)

	dst := filepath.Join(root, "skills")
	os.MkdirAll(filepath.Join(dst, "kept"), 0755)
	os.WriteFile(filepath.Join(dst, "kept", "SKILL.md"), []byte("local"), 0644)

	res, err := Sync(acct, dst)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 || !res[0].Copied || res[1].Copied {
		t.Fatalf("results = %+v", res)
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "mine", "SKILL.md")); string(b) != "mine" {
		t.Errorf("mine not copied: %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "kept", "SKILL.md")); string(b) != "local" {
		t.Errorf("existing skill overwritten: %q", b)
	}
	if _, err := os.Stat(filepath.Join(dst, "pdf")); err == nil {
		t.Error("built-in skill copied")
	}
}
