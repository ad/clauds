package claudeconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ad/clauds/internal/account"
	"github.com/ad/clauds/internal/paths"
)

const DirName = "claude-config"

var sharedNames = []string{
	"settings.json", "settings.local.json", "keybindings.json",
	"agents", "commands", "skills", "hooks", "plugins", "output-styles", "rules",
}

func Dir(acct *account.Account) string {
	return filepath.Join(acct.Dir, DirName)
}

func shared(src string) ([]string, error) {
	entries, err := os.ReadDir(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if strings.HasSuffix(n, ".md") || slices.Contains(sharedNames, n) {
			names = append(names, n)
		}
	}
	return names, nil
}

func Ensure(acct *account.Account, src, srcJSON string) error {
	dir := Dir(acct)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}

	names, err := shared(src)
	if err != nil {
		return err
	}
	for _, n := range names {
		link := filepath.Join(dir, n)
		// a regular file here may hold the account's own edits; doctor reports it instead
		if paths.PathExists(link) {
			continue
		}
		if err := os.Symlink(filepath.Join(src, n), link); err != nil {
			return err
		}
	}

	return seedClaudeJSON(filepath.Join(dir, ".claude.json"), srcJSON)
}

func seedClaudeJSON(dst, srcJSON string) error {
	if paths.PathExists(dst) {
		return nil
	}
	raw, err := os.ReadFile(srcJSON)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var v map[string]json.RawMessage
	if err := json.Unmarshal(raw, &v); err != nil {
		return fmt.Errorf("parsing %s: %w", srcJSON, err)
	}
	mcp, ok := v["mcpServers"]
	if !ok {
		return nil
	}
	data, err := json.MarshalIndent(map[string]json.RawMessage{"mcpServers": mcp}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0600)
}

func Drifted(acct *account.Account, src string) []string {
	names, _ := shared(src)
	var out []string
	for _, n := range names {
		target, err := os.Readlink(filepath.Join(Dir(acct), n))
		if err == nil && target == filepath.Join(src, n) {
			continue
		}
		if paths.PathExists(filepath.Join(Dir(acct), n)) {
			out = append(out, n)
		}
	}
	return out
}
