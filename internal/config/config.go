package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/ad/clauds/internal/codesessions"
	"github.com/ad/clauds/internal/paths"
)

type Tool struct {
	AppPath           *string `json:"app_path,omitempty"`
	ShareCodeSessions bool    `json:"share_code_sessions"`
	CodeSessionsRoot  *string `json:"code_sessions_root,omitempty"`
}

type Resolved struct {
	AppPath       string
	CodeSessions  string
	ShareSessions bool
}

func Default() *Tool {
	return &Tool{
		ShareCodeSessions: true,
	}
}

func Load(p *paths.Paths) (*Tool, error) {
	fp := p.ToolConfigJSON()
	if !paths.PathExists(fp) {
		return Default(), nil
	}

	raw, err := os.ReadFile(fp)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", fp, err)
	}

	var cfg Tool
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", fp, err)
	}

	return &cfg, nil
}

func Resolve(p *paths.Paths, cfg *Tool) *Resolved {
	appPath := paths.DefaultAppPath
	if cfg.AppPath != nil {
		appPath = *cfg.AppPath
	}

	sessionsRoot := p.DefaultCodeSessionsRoot()
	if root := codesessions.MeldSessionsRoot(p.MeldConfig()); root != nil {
		sessionsRoot = *root
	}
	if cfg.CodeSessionsRoot != nil {
		sessionsRoot = *cfg.CodeSessionsRoot
	}

	return &Resolved{
		AppPath:       appPath,
		CodeSessions:  sessionsRoot,
		ShareSessions: cfg.ShareCodeSessions,
	}
}

func (c *Tool) Save(p *paths.Paths) error {
	if err := os.MkdirAll(p.ClaudsRoot, 0755); err != nil {
		return fmt.Errorf("creating %s: %w", p.ClaudsRoot, err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return paths.WriteBytesAtomic(p.ToolConfigJSON(), data)
}

func ParseOnOff(v string) (bool, error) {
	switch strings.ToLower(v) {
	case "on", "true", "yes", "1":
		return true, nil
	case "off", "false", "no", "0":
		return false, nil
	default:
		return false, fmt.Errorf("expected on|off, got '%s'", v)
	}
}
