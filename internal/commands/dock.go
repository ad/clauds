package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ad/clauds/internal/account"
	"github.com/ad/clauds/internal/dock"
	"github.com/ad/clauds/internal/paths"
)

func Dock(p *paths.Paths, acct *account.Account) error {
	if err := paths.RequireDir(p.App, "Claude.app"); err != nil {
		return err
	}
	clauds, err := os.Executable()
	if err != nil {
		return err
	}
	app, err := dock.Create(shortcutsDir(p), acct.Meta.Name, acct.Meta.Slot, clauds, p.App)
	if err != nil {
		return err
	}
	fmt.Printf("✓ Created %s.\n  Drag it to the Dock. Rerun `clauds dock %s` if clauds moves.\n", app, acct.Meta.Name)
	return nil
}

func shortcutsDir(p *paths.Paths) string {
	return filepath.Join(p.Home, "Applications")
}
