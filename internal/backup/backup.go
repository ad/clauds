package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ad/clauds/internal/account"
	"github.com/ad/clauds/internal/claudeconfig"
	"github.com/ad/clauds/internal/paths"
)

const (
	manifestName = "manifest.json"
	mcpName      = "mcpServers.json"
)

var personal = []string{"projects", "history.jsonl"}

var skipDirs = []string{".venv", "venv", "node_modules", "__pycache__"}

type manifest struct {
	Version int    `json:"version"`
	Home    string `json:"home"`
}

type Report struct {
	Added, Kept    int
	Accounts       []string
	MergedAccounts []string
	MCPAdded       []string
}

func Create(p *paths.Paths, w io.Writer, full bool) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	if err := create(p, tw, full); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

// Restore order matters: shared config and MCP servers land before accounts, so Ensure links and seeds them.
func create(p *paths.Paths, tw *tar.Writer, full bool) error {
	m, _ := json.Marshal(manifest{Version: 1, Home: p.Home})
	if err := writeBytes(tw, manifestName, m); err != nil {
		return err
	}
	if err := addTree(tw, p.ToolConfigJSON(), "clauds/config.json", map[string]bool{}); err != nil {
		return err
	}

	names, err := claudeconfig.Shared(p.ClaudeHome())
	if err != nil {
		return err
	}
	if full {
		names = append(names, personal...)
	}
	for _, n := range names {
		if err := addTree(tw, filepath.Join(p.ClaudeHome(), n), "claude/"+n, map[string]bool{}); err != nil {
			return err
		}
	}

	mcp, err := mcpServers(p.ClaudeJSON())
	if err != nil {
		return err
	}
	if mcp != nil {
		if err := writeBytes(tw, mcpName, mcp); err != nil {
			return err
		}
	}

	if !full {
		return nil
	}
	accounts, err := account.List(p)
	if err != nil {
		return err
	}
	for _, a := range accounts {
		base := "accounts/" + filepath.Base(a.Dir) + "/"
		srcs := [][2]string{
			{account.MetaPath(a.Dir), base + "meta.json"},
			{filepath.Join(a.DataDir(), "claude_desktop_config.json"), base + "data/claude_desktop_config.json"},
		}
		for _, n := range personal {
			srcs = append(srcs, [2]string{filepath.Join(claudeconfig.Dir(a), n), base + "claude-config/" + n})
		}
		for _, s := range srcs {
			if err := addTree(tw, s[0], s[1], map[string]bool{}); err != nil {
				return err
			}
		}
	}
	return nil
}

func mcpServers(claudeJSON string) ([]byte, error) {
	raw, err := os.ReadFile(claudeJSON)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var v map[string]json.RawMessage
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", claudeJSON, err)
	}
	return v["mcpServers"], nil
}

func writeBytes(tw *tar.Writer, name string, data []byte) error {
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

// addTree follows symlinks: shared skills often link outside ~/.claude, and the archive itself holds only files.
func addTree(tw *tar.Writer, src, name string, ancestors map[string]bool) error {
	fi, err := os.Stat(src)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	if fi.IsDir() {
		real, err := filepath.EvalSymlinks(src)
		if err != nil {
			return err
		}
		if ancestors[real] {
			return nil
		}
		ancestors[real] = true
		defer delete(ancestors, real)

		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if slices.Contains(skipDirs, e.Name()) {
				continue
			}
			if err := addTree(tw, filepath.Join(src, e.Name()), name+"/"+e.Name(), ancestors); err != nil {
				return err
			}
		}
		return nil
	}

	if !fi.Mode().IsRegular() {
		return nil
	}
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	hdr := &tar.Header{Name: name, Mode: int64(fi.Mode().Perm()), Size: fi.Size(), ModTime: fi.ModTime(), Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	// CopyN: a live transcript may grow while we read it
	if _, err := io.CopyN(tw, f, fi.Size()); err != nil {
		return fmt.Errorf("reading %s: %w", src, err)
	}
	return nil
}

func Restore(p *paths.Paths, r io.Reader) (*Report, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, errors.New("not a clauds backup")
	}
	tr := tar.NewReader(gz)
	hdr, err := tr.Next()
	if err != nil || hdr.Name != manifestName {
		return nil, errors.New("not a clauds backup")
	}
	var m manifest
	if err := json.NewDecoder(tr).Decode(&m); err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}
	if m.Version != 1 || !filepath.IsAbs(m.Home) || filepath.Clean(m.Home) == "/" {
		return nil, fmt.Errorf("unsupported backup (version %d, home %q)", m.Version, m.Home)
	}

	rep := &Report{}
	targets := map[string]*account.Account{}

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return rep, nil
		}
		if err != nil {
			return rep, err
		}
		if hdr.Typeflag != tar.TypeReg || !filepath.IsLocal(hdr.Name) {
			return rep, fmt.Errorf("unsafe entry in backup: %q", hdr.Name)
		}

		var body io.Reader = tr
		if m.Home != p.Home && (strings.HasSuffix(hdr.Name, ".json") || strings.HasSuffix(hdr.Name, ".jsonl")) {
			b, err := io.ReadAll(tr)
			if err != nil {
				return rep, err
			}
			b = bytes.ReplaceAll(b, []byte(m.Home+"/"), []byte(p.Home+"/"))
			b = bytes.ReplaceAll(b, []byte(m.Home+`"`), []byte(p.Home+`"`))
			body = bytes.NewReader(b)
		}

		dst := ""
		parts := strings.SplitN(hdr.Name, "/", 3)
		switch {
		case hdr.Name == mcpName:
			added, err := mergeMCP(p.ClaudeJSON(), body)
			if err != nil {
				return rep, err
			}
			rep.MCPAdded = append(rep.MCPAdded, added...)
			continue

		case hdr.Name == "clauds/config.json":
			dst = p.ToolConfigJSON()

		case parts[0] == "claude" && len(parts) > 1 && (claudeconfig.IsShared(parts[1]) || slices.Contains(personal, parts[1])):
			dst = filepath.Join(p.ClaudeHome(), rehome(strings.TrimPrefix(hdr.Name, "claude/"), m.Home, p.Home))

		case parts[0] == "accounts" && len(parts) == 3 && parts[2] == "meta.json":
			acct, isNew, err := targetAccount(p, body)
			if err != nil {
				return rep, err
			}
			targets[parts[1]] = acct
			if isNew {
				rep.Accounts = append(rep.Accounts, acct.Meta.Name)
			} else {
				rep.MergedAccounts = append(rep.MergedAccounts, acct.Meta.Name)
			}
			continue

		case parts[0] == "accounts" && len(parts) == 3 && isAccountFile(parts[2]):
			acct := targets[parts[1]]
			if acct == nil {
				continue
			}
			dst = filepath.Join(acct.Dir, rehome(parts[2], m.Home, p.Home))

		default:
			return rep, fmt.Errorf("unexpected entry in backup: %q", hdr.Name)
		}

		added, err := writeNew(dst, os.FileMode(hdr.Mode).Perm(), body)
		if err != nil {
			return rep, err
		}
		if added {
			rep.Added++
		} else {
			rep.Kept++
		}
	}
}

// an account with the same name takes the backup's chats too: never overwritten, and a half-done restore can be rerun
func targetAccount(p *paths.Paths, r io.Reader) (*account.Account, bool, error) {
	var meta account.Meta
	if err := json.NewDecoder(r).Decode(&meta); err != nil {
		return nil, false, fmt.Errorf("reading account meta: %w", err)
	}
	existing, err := account.List(p)
	if err != nil {
		return nil, false, err
	}
	for _, a := range existing {
		if strings.EqualFold(a.Meta.Name, meta.Name) {
			return a, false, nil
		}
	}
	acct, err := account.Create(p, meta.Name, meta.Email, meta.Created)
	if err != nil {
		return nil, false, err
	}
	return acct, true, claudeconfig.Ensure(acct, p.ClaudeHome(), p.ClaudeJSON())
}

func isAccountFile(rel string) bool {
	if rel == "data/claude_desktop_config.json" {
		return true
	}
	parts := strings.SplitN(rel, "/", 3)
	return len(parts) > 1 && parts[0] == claudeconfig.DirName && slices.Contains(personal, parts[1])
}

// rehome renames projects/<encoded cwd> folders, where Claude Code keys transcripts and memory by path.
func rehome(rel, oldHome, newHome string) string {
	oldEnc, newEnc := encode(oldHome), encode(newHome)
	parts := strings.Split(rel, "/")
	for i := 1; i < len(parts); i++ {
		if parts[i-1] != "projects" || !(parts[i] == oldEnc || strings.HasPrefix(parts[i], oldEnc+"-")) {
			continue
		}
		// past NAME_MAX Claude Code hashes the name itself; we can't match that, so keep the old one
		if name := newEnc + parts[i][len(oldEnc):]; len(name) <= 255 {
			parts[i] = name
		}
	}
	return filepath.Join(parts...)
}

func encode(path string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '-'
	}, path)
}

func writeNew(dst string, mode os.FileMode, r io.Reader) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return false, err
	}
	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if os.IsExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_, err = io.Copy(f, r)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(dst)
		return false, err
	}
	return true, nil
}

func mergeMCP(claudeJSON string, r io.Reader) ([]string, error) {
	var add map[string]json.RawMessage
	if err := json.NewDecoder(r).Decode(&add); err != nil {
		return nil, fmt.Errorf("reading MCP servers: %w", err)
	}

	doc := map[string]json.RawMessage{}
	raw, err := os.ReadFile(claudeJSON)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil {
		if err := json.Unmarshal(raw, &doc); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", claudeJSON, err)
		}
	}
	have := map[string]json.RawMessage{}
	if v, ok := doc["mcpServers"]; ok {
		if err := json.Unmarshal(v, &have); err != nil {
			return nil, fmt.Errorf("parsing mcpServers in %s: %w", claudeJSON, err)
		}
	}

	var added []string
	for k, v := range add {
		if _, ok := have[k]; !ok {
			have[k] = v
			added = append(added, k)
		}
	}
	if len(added) == 0 {
		return nil, nil
	}
	slices.Sort(added)

	doc["mcpServers"], _ = json.Marshal(have)
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	tmp := claudeJSON + ".clauds-tmp"
	if err := os.WriteFile(tmp, out, 0600); err != nil {
		return nil, err
	}
	return added, os.Rename(tmp, claudeJSON)
}
