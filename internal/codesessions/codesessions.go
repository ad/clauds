package codesessions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ad/clauds/internal/account"
	"github.com/ad/clauds/internal/paths"
)

const FolderName = "claude-code-sessions"

func MeldSessionsRoot(meldConfig string) *string {
	raw, err := os.ReadFile(meldConfig)
	if err != nil {
		return nil
	}

	for _, line := range strings.Split(string(raw), "\n") {
		l := strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(l, "sessions_root"); ok {
			if idx := strings.Index(rest, "="); idx >= 0 {
				v := strings.TrimSpace(rest[idx+1:])
				v = strings.Trim(v, "\"")
				if v != "" {
					return &v
				}
			}
		}
	}
	return nil
}

func IsLinked(acct *account.Account, target string) bool {
	link := filepath.Join(acct.DataDir(), FolderName)
	linkTarget, err := os.Readlink(link)
	if err != nil {
		return false
	}
	return linkTarget == target
}

func AccountUUID(acct *account.Account) *string {
	raw, err := os.ReadFile(filepath.Join(acct.DataDir(), "config.json"))
	if err != nil {
		return nil
	}

	var v map[string]interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}

	if uuid, ok := v["lastKnownAccountUuid"].(string); ok {
		return &uuid
	}
	return nil
}

func EnsureMeldAccountFolder(acct *account.Account, target string) error {
	if uuid := AccountUUID(acct); uuid != nil {
		dir := filepath.Join(target, *uuid)
		if !paths.PathExists(dir) {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("creating %s: %w", dir, err)
			}
		}
	}
	return nil
}

func Link(acct *account.Account, target string) error {
	if err := os.MkdirAll(target, 0755); err != nil {
		return fmt.Errorf("creating %s: %w", target, err)
	}
	if err := os.MkdirAll(acct.DataDir(), 0755); err != nil {
		return err
	}

	link := filepath.Join(acct.DataDir(), FolderName)

	md, err := os.Lstat(link)
	if err != nil {
		return os.Symlink(target, link)
	}

	if md.Mode()&os.ModeSymlink != 0 {
		linkTarget, err := os.Readlink(link)
		if err != nil {
			return os.Symlink(target, link)
		}
		if linkTarget != target {
			os.Remove(link)
			return os.Symlink(target, link)
		}
		return nil
	}

	if md.IsDir() {
		if err := MergeMove(link, target); err != nil {
			return err
		}
		os.RemoveAll(link)
		return os.Symlink(target, link)
	}

	os.Remove(link)
	return os.Symlink(target, link)
}

func MergeMove(src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		from := filepath.Join(src, entry.Name())
		to := filepath.Join(dst, entry.Name())
		if !paths.PathExists(to) {
			if err := os.Rename(from, to); err != nil {
				return err
			}
		} else if entry.IsDir() {
			toInfo, err := os.Stat(to)
			if err == nil && toInfo.IsDir() {
				if err := MergeMove(from, to); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
