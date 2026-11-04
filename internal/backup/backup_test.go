package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ad/clauds/internal/account"
	"github.com/ad/clauds/internal/claudeconfig"
	"github.com/ad/clauds/internal/paths"
)

func home(t *testing.T, dir string) *paths.Paths {
	t.Helper()
	p := &paths.Paths{Home: dir, ClaudsRoot: filepath.Join(dir, ".config", "clauds")}
	os.MkdirAll(p.ClaudeHome(), 0700)
	return p
}

func write(t *testing.T, path, data string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0755)
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRoundTrip(t *testing.T) {
	root := t.TempDir()
	src := home(t, filepath.Join(root, "Users", "old"))
	dst := home(t, filepath.Join(root, "Users", "new"))
	ch := src.ClaudeHome()

	write(t, filepath.Join(ch, "settings.json"), `{"hook":"`+src.Home+`/hook.sh"}`)
	write(t, filepath.Join(ch, "CLAUDE.md"), "theirs")
	write(t, filepath.Join(ch, "skills", "a", "SKILL.md"), "a")
	write(t, filepath.Join(ch, "skills", "a", ".venv", "bin", "python"), "x")
	write(t, filepath.Join(root, "elsewhere", "SKILL.md"), "linked")
	os.Symlink(filepath.Join(root, "elsewhere"), filepath.Join(ch, "skills", "linked"))
	os.Symlink(filepath.Join(ch, "skills"), filepath.Join(ch, "skills", "loop"))
	write(t, filepath.Join(ch, "projects", encode(src.Home)+"-proj", "t.jsonl"), `{"cwd":"`+src.Home+`/proj"}`)
	write(t, src.ClaudeJSON(), `{"userID":"secret","mcpServers":{"x":{"command":"x"},"y":{"command":"theirs"}}}`)
	for _, n := range []string{"work", "home"} {
		a, err := account.Create(src, n, nil, "now")
		if err != nil {
			t.Fatal(err)
		}
		if err := claudeconfig.Ensure(a, ch, src.ClaudeJSON()); err != nil {
			t.Fatal(err)
		}
		write(t, filepath.Join(claudeconfig.Dir(a), "projects", encode(src.Home)+"-p", "memory", "m.md"), n)
	}

	write(t, filepath.Join(dst.ClaudeHome(), "CLAUDE.md"), "mine")
	write(t, dst.ClaudeJSON(), `{"userID":"mine","mcpServers":{"y":{"command":"mine"}}}`)
	work, err := account.Create(dst, "work", nil, "now")
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := Create(src, &buf, true); err != nil {
		t.Fatal(err)
	}
	gz, _ := gzip.NewReader(bytes.NewReader(buf.Bytes()))
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}
		if hdr.Typeflag != tar.TypeReg || strings.Contains(hdr.Name, ".venv") || strings.HasSuffix(hdr.Name, ".claude.json") {
			t.Errorf("unexpected entry %s (type %c)", hdr.Name, hdr.Typeflag)
		}
	}

	rep, err := Restore(dst, bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Accounts) != 1 || rep.Accounts[0] != "home" || len(rep.MergedAccounts) != 1 {
		t.Fatalf("report = %+v", rep)
	}

	dc := dst.ClaudeHome()
	if got := read(t, filepath.Join(dc, "settings.json")); got != `{"hook":"`+dst.Home+`/hook.sh"}` {
		t.Errorf("settings.json = %s", got)
	}
	if got := read(t, filepath.Join(dc, "CLAUDE.md")); got != "mine" {
		t.Errorf("existing CLAUDE.md overwritten: %s", got)
	}
	if got := read(t, filepath.Join(dc, "skills", "linked", "SKILL.md")); got != "linked" {
		t.Errorf("linked skill = %s", got)
	}
	if fi, err := os.Lstat(filepath.Join(dc, "skills", "linked")); err != nil || !fi.IsDir() {
		t.Errorf("linked skill not restored as a dir: %v", err)
	}
	if read(t, filepath.Join(dc, "projects", encode(dst.Home)+"-proj", "t.jsonl")) != `{"cwd":"`+dst.Home+`/proj"}` {
		t.Error("project transcript not rehomed")
	}

	var doc struct {
		UserID     string                     `json:"userID"`
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	json.Unmarshal([]byte(read(t, dst.ClaudeJSON())), &doc)
	if doc.UserID != "mine" || len(doc.MCPServers) != 2 || !strings.Contains(string(doc.MCPServers["y"]), "mine") {
		t.Errorf(".claude.json = %+v", doc)
	}

	h, err := account.Resolve(dst, "home")
	if err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(claudeconfig.Dir(h), "projects", encode(dst.Home)+"-p", "memory", "m.md")) != "home" {
		t.Error("account memory not restored")
	}
	if read(t, filepath.Join(claudeconfig.Dir(work), "projects", encode(dst.Home)+"-p", "memory", "m.md")) != "work" {
		t.Error("existing account did not get the backup's memory")
	}
	if target, _ := os.Readlink(filepath.Join(claudeconfig.Dir(h), "settings.json")); target != filepath.Join(dc, "settings.json") {
		t.Errorf("settings.json link = %q", target)
	}
}

func TestRestoreRejectsUntrusted(t *testing.T) {
	p := home(t, t.TempDir())
	for _, c := range []struct{ home, name, want string }{
		{p.Home, "claude/../../x", "unsafe"},
		{"", "claude/CLAUDE.md", "unsupported"},
		{p.Home, "accounts/1-a/claude-config/.claude.json", "unexpected"},
		{p.Home, "claude/.credentials.json", "unexpected"},
	} {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)
		writeBytes(tw, manifestName, []byte(`{"version":1,"home":"`+c.home+`"}`))
		writeBytes(tw, c.name, []byte("x"))
		tw.Close()
		gz.Close()

		if _, err := Restore(p, &buf); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want %q", c.name, err, c.want)
		}
	}
}
