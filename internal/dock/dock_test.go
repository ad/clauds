package dock

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func fakeClaudeApp(t *testing.T) string {
	app := filepath.Join(t.TempDir(), "Claude.app")
	res := filepath.Join(app, "Contents", "Resources")
	if err := os.MkdirAll(res, 0755); err != nil {
		t.Fatal(err)
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict><key>CFBundleIconFile</key><string>electron</string></dict></plist>`
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(plist), 0644); err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 16, 16)))
	if err := os.WriteFile(filepath.Join(res, "electron.icns"), b.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	return app
}

func TestCreate(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	dir := t.TempDir()
	name := "o'neil & co"
	app, err := Create(dir, name, 1, "/opt/my bin/clauds", fakeClaudeApp(t))
	if err != nil {
		t.Fatal(err)
	}
	if app != filepath.Join(dir, "Claude o'neil & co.app") {
		t.Fatalf("app = %s", app)
	}

	if out, err := exec.Command("plutil", "-lint", filepath.Join(app, "Contents", "Info.plist")).CombinedOutput(); err != nil {
		t.Fatalf("plutil: %s", out)
	}
	plist, _ := os.ReadFile(filepath.Join(app, "Contents", "Info.plist"))
	for _, want := range []string{
		"<string>Claude o&#39;neil &amp; co</string>",
		"<string>com.github.ad.clauds.dock.1</string>",
		"<string>clauds-open</string>",
		"<string>clauds.icns</string>",
	} {
		if !strings.Contains(string(plist), want) {
			t.Errorf("Info.plist lacks %s", want)
		}
	}

	exe := filepath.Join(app, "Contents", "MacOS", "clauds-open")
	script, _ := os.ReadFile(exe)
	if want := "#!/bin/sh\nexec '/opt/my bin/clauds' open 'o'\\''neil & co'\n"; string(script) != want {
		t.Errorf("script = %q, want %q", script, want)
	}
	if st, _ := os.Stat(exe); st.Mode().Perm() != 0755 {
		t.Errorf("script mode = %v", st.Mode())
	}

	icon, _ := os.ReadFile(filepath.Join(app, "Contents", "Resources", "clauds.icns"))
	if !bytes.HasPrefix(icon, []byte("icns")) || !bytes.HasPrefix(icon[8:], []byte("ic10")) || !bytes.HasPrefix(icon[16:], []byte("\x89PNG")) {
		t.Errorf("icon is not an ic10 icns: % x", icon[:min(len(icon), 24)])
	}
}

func TestKeepsForeignApp(t *testing.T) {
	dir := t.TempDir()
	foreign := filepath.Join(BundlePath(dir, "work"), "Contents")
	if err := os.MkdirAll(foreign, 0755); err != nil {
		t.Fatal(err)
	}
	if err := Remove(dir, "work"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatal("removed an app clauds did not make")
	}
	if _, err := Create(dir, "work", 1, "/bin/clauds", fakeClaudeApp(t)); err == nil {
		t.Fatal("overwrote an app clauds did not make")
	}

	app, err := Create(dir, "home", 2, "/bin/clauds", fakeClaudeApp(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := Remove(dir, "home"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(app); !os.IsNotExist(err) {
		t.Fatal("shortcut still there")
	}
}
