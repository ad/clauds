package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

const DefaultAppPath = "/Applications/Claude.app"

type Paths struct {
	Home       string
	App        string
	ClaudsRoot string
}

func Resolve() (*Paths, error) {
	home, err := HomeDir()
	if err != nil {
		return nil, err
	}

	claudsRoot, err := resolveClaudsRoot(home)
	if err != nil {
		return nil, err
	}

	return &Paths{
		Home:       home,
		App:        DefaultAppPath,
		ClaudsRoot: claudsRoot,
	}, nil
}

func (p *Paths) AppBinary() string {
	return filepath.Join(p.App, "Contents", "MacOS", "Claude")
}

func (p *Paths) AccountsDir() string {
	return filepath.Join(p.ClaudsRoot, "accounts")
}

func (p *Paths) ToolConfigJSON() string {
	return filepath.Join(p.ClaudsRoot, "config.json")
}

func (p *Paths) DefaultCodeSessionsRoot() string {
	return filepath.Join(p.Home, "Library", "Application Support", "Claude", "claude-code-sessions")
}

func (p *Paths) ClaudeHome() string {
	return filepath.Join(p.Home, ".claude")
}

func (p *Paths) ClaudeJSON() string {
	return filepath.Join(p.Home, ".claude.json")
}

func (p *Paths) MeldConfig() string {
	return filepath.Join(p.Home, ".meld", "config.toml")
}

func HomeDir() (string, error) {
	home := os.Getenv("HOME")
	if home == "" {
		return "", fmt.Errorf("$HOME is not set; cannot locate the home directory")
	}
	return home, nil
}

func PathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func RequireDir(path string, what string) error {
	md, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s not found at %s", what, path)
	}
	if !md.IsDir() {
		return fmt.Errorf("%s at %s is not a directory", what, path)
	}
	return nil
}

func WriteBytesAtomic(path string, data []byte) error {
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return fmt.Errorf("creating %s: %w", parent, err)
	}

	tmp := fmt.Sprintf("%s.tmp-%d", path, os.Getpid())
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("renaming into %s: %w", path, err)
	}
	return nil
}

func resolveClaudsRoot(home string) (string, error) {
	xdgRoot := filepath.Join(home, ".config", "clauds")
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		xdgRoot = filepath.Join(xdg, "clauds")
	}

	legacyRoot := filepath.Join(home, ".clauds")

	if PathExists(xdgRoot) {
		return xdgRoot, nil
	}

	if PathExists(legacyRoot) {
		if err := migrateLegacyToXdg(legacyRoot, xdgRoot); err != nil {
			return "", err
		}
		return xdgRoot, nil
	}

	return xdgRoot, nil
}

func migrateLegacyToXdg(legacy, xdg string) error {
	parent := filepath.Dir(xdg)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return fmt.Errorf("creating %s: %w", parent, err)
	}

	if err := os.Rename(legacy, xdg); err != nil {
		return fmt.Errorf("migrating %s -> %s: %w", legacy, xdg, err)
	}

	_ = os.Symlink(xdg, legacy)

	fmt.Fprintf(os.Stderr, "clauds: migrated data from %s -> %s\n", legacy, xdg)
	return nil
}
