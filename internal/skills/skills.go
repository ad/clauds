package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ad/clauds/internal/account"
	"github.com/ad/clauds/internal/paths"
)

type Result struct {
	Name   string
	Copied bool
	Err    error
}

type manifest struct {
	Skills []struct {
		Name        string `json:"name"`
		CreatorType string `json:"creatorType"`
	} `json:"skills"`
}

func Sync(acct *account.Account, dst string) ([]Result, error) {
	manifests, err := filepath.Glob(filepath.Join(acct.DataDir(), "local-agent-mode-sessions", "skills-plugin", "*", "*", "manifest.json"))
	if err != nil {
		return nil, err
	}

	var out []Result
	for _, mp := range manifests {
		raw, err := os.ReadFile(mp)
		if err != nil {
			return out, err
		}
		var m manifest
		if err := json.Unmarshal(raw, &m); err != nil {
			return out, fmt.Errorf("parsing %s: %w", mp, err)
		}

		for _, s := range m.Skills {
			// names come from the server; refuse anything that could escape dst
			if s.CreatorType != "user" || s.Name == "" || s.Name != filepath.Base(s.Name) || s.Name == ".." {
				continue
			}
			src := filepath.Join(filepath.Dir(mp), "skills", s.Name)
			if !paths.PathExists(src) {
				continue
			}
			to := filepath.Join(dst, s.Name)
			if paths.PathExists(to) {
				out = append(out, Result{Name: s.Name})
				continue
			}
			if err := os.CopyFS(to, os.DirFS(src)); err != nil {
				os.RemoveAll(to)
				out = append(out, Result{Name: s.Name, Err: err})
				continue
			}
			out = append(out, Result{Name: s.Name, Copied: true})
		}
	}
	return out, nil
}
