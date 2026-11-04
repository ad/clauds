package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/ad/clauds/internal/account"
	"github.com/ad/clauds/internal/backup"
	"github.com/ad/clauds/internal/claudeconfig"
	"github.com/ad/clauds/internal/codesessions"
	"github.com/ad/clauds/internal/config"
	"github.com/ad/clauds/internal/doctor"
	"github.com/ad/clauds/internal/paths"
	"github.com/ad/clauds/internal/process"
	"github.com/ad/clauds/internal/skills"
)

const quitTimeout = 15 * time.Second

func OpenAccount(p *paths.Paths, pm process.Manager, acct *account.Account) error {
	if err := paths.RequireDir(p.App, "Claude.app"); err != nil {
		return err
	}

	data := acct.DataDir()

	running, err := pm.IsRunning(data)
	if err != nil {
		return err
	}

	if running {
		pm.Activate(data)
		fmt.Printf("✓ %s is already open — bringing it to the front.\n", acct.Meta.Name)
		return nil
	}

	cfg, _ := config.Load(p)
	if cfg == nil {
		cfg = config.Default()
	}
	rc := config.Resolve(p, cfg)
	ensureSharedSessions(rc, acct)
	ensureClaudeConfig(p, acct)

	if err := pm.Launch(p.App, data, claudeconfig.Dir(acct)); err != nil {
		return err
	}

	updated := *acct
	now := Now()
	updated.Meta.LastOpened = &now
	_ = updated.SaveMeta()

	fmt.Printf("✓ Opened %s.\n", acct.Meta.Name)
	return nil
}

func OpenAll(p *paths.Paths, pm process.Manager) error {
	if err := paths.RequireDir(p.App, "Claude.app"); err != nil {
		return err
	}

	accounts, err := account.List(p)
	if err != nil {
		return err
	}

	if len(accounts) == 0 {
		fmt.Println("No saved accounts yet. Run `clauds add <name>`.")
		return nil
	}

	cfg, _ := config.Load(p)
	if cfg == nil {
		cfg = config.Default()
	}
	rc := config.Resolve(p, cfg)

	opened := 0

	for _, a := range accounts {
		running, _ := pm.IsRunning(a.DataDir())
		if running {
			continue
		}
		ensureSharedSessions(rc, a)
		ensureClaudeConfig(p, a)
		if err := pm.Launch(p.App, a.DataDir(), claudeconfig.Dir(a)); err != nil {
			return err
		}
		opened++
		time.Sleep(700 * time.Millisecond)
	}

	alreadyOpen := len(accounts) - opened
	suffix := ""
	if len(accounts) != 1 {
		suffix = "s"
	}
	fmt.Printf("✓ Opened %d of %d account window%s (%d already open).\n",
		opened, len(accounts), suffix, alreadyOpen)
	return nil
}

func AddAccount(p *paths.Paths, pm process.Manager, name string, email *string) error {
	if err := paths.RequireDir(p.App, "Claude.app"); err != nil {
		return err
	}

	acct, err := account.Create(p, name, email, Now())
	if err != nil {
		return err
	}

	cfg, _ := config.Load(p)
	if cfg == nil {
		cfg = config.Default()
	}
	rc := config.Resolve(p, cfg)
	ensureSharedSessions(rc, acct)
	ensureClaudeConfig(p, acct)

	if err := pm.Launch(p.App, acct.DataDir(), claudeconfig.Dir(acct)); err != nil {
		return err
	}

	fmt.Printf("✓ Created %s. A fresh Claude window is opening — log into this account there.\n  From now on: `clauds %s` reopens it, already logged in.\n",
		acct.Meta.Name, acct.Meta.Name)
	return nil
}

func RemoveAccount(p *paths.Paths, pm process.Manager, acct *account.Account) error {
	running, err := pm.IsRunning(acct.DataDir())
	if err != nil {
		return err
	}

	if running {
		if err := pm.Quit(acct.DataDir(), quitTimeout); err != nil {
			return err
		}
	}

	if err := account.Delete(acct); err != nil {
		return err
	}

	fmt.Printf("✓ Removed %s.\n", acct.Meta.Name)
	return nil
}

func List(p *paths.Paths, pm process.Manager, jsonOutput bool) error {
	accounts, err := account.List(p)
	if err != nil {
		return err
	}

	if jsonOutput {
		var arr []map[string]interface{}
		for _, a := range accounts {
			running, _ := pm.IsRunning(a.DataDir())
			item := map[string]interface{}{
				"slot":        a.Meta.Slot,
				"name":        a.Meta.Name,
				"email":       a.Meta.Email,
				"created":     a.Meta.Created,
				"last_opened": a.Meta.LastOpened,
				"running":     running,
			}
			arr = append(arr, item)
		}
		data, _ := json.MarshalIndent(arr, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	if len(accounts) == 0 {
		fmt.Println("No saved accounts yet. Run `clauds add <name>`.")
		return nil
	}

	for _, a := range accounts {
		running, _ := pm.IsRunning(a.DataDir())
		dot := " "
		if running {
			dot = "●"
		}
		if a.Meta.Email != nil {
			fmt.Printf("%s %d. %-14s ·  %s\n", dot, a.Meta.Slot, a.Meta.Name, *a.Meta.Email)
		} else {
			fmt.Printf("%s %d. %s\n", dot, a.Meta.Slot, a.Meta.Name)
		}
	}
	return nil
}

func DoctorCmd(p *paths.Paths, pm process.Manager, jsonOutput bool) error {
	report, err := doctor.Run(p, pm)
	if err != nil {
		return err
	}

	if jsonOutput {
		data, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(data))
	} else {
		doctor.PrintHuman(report)
	}

	if !report.Healthy {
		os.Exit(1)
	}
	return nil
}

func ConfigCmd(p *paths.Paths, args []string) error {
	cfg, err := config.Load(p)
	if err != nil {
		return err
	}

	rc := config.Resolve(p, cfg)

	if len(args) == 0 {
		fmt.Printf("app-path            = %s\n", rc.AppPath)
		shareStr := "off"
		if rc.ShareSessions {
			shareStr = "on"
		}
		fmt.Printf("share-code-sessions = %s\n", shareStr)
		fmt.Printf("code-sessions-root  = %s\n", rc.CodeSessions)
		return nil
	}

	action := args[0]
	switch action {
	case "get":
		if len(args) < 2 {
			return fmt.Errorf("missing config key")
		}
		key := args[1]
		var val string
		switch key {
		case "app-path":
			val = rc.AppPath
		case "share-code-sessions":
			if rc.ShareSessions {
				val = "on"
			} else {
				val = "off"
			}
		case "code-sessions-root":
			val = rc.CodeSessions
		default:
			return fmt.Errorf("unknown config key: %s", key)
		}
		fmt.Println(val)
		return nil

	case "set":
		if len(args) < 3 {
			return fmt.Errorf("missing config key and value")
		}
		key := args[1]
		value := args[2]
		switch key {
		case "app-path":
			cfg.AppPath = &value
		case "share-code-sessions":
			v, err := config.ParseOnOff(value)
			if err != nil {
				return err
			}
			cfg.ShareCodeSessions = v
		case "code-sessions-root":
			cfg.CodeSessionsRoot = &value
		default:
			return fmt.Errorf("unknown config key: %s", key)
		}
		if err := cfg.Save(p); err != nil {
			return err
		}
		fmt.Printf("✓ set %s = %s\n", key, value)
		return nil

	default:
		return fmt.Errorf("unknown config action: %s", action)
	}
}

func ensureSharedSessions(rc *config.Resolved, acct *account.Account) {
	if !rc.ShareSessions {
		return
	}
	if err := codesessions.Link(acct, rc.CodeSessions); err != nil {
		fmt.Fprintf(os.Stderr, "note: couldn't link %s's chats for meld (%s); continuing.\n", acct.Meta.Name, err)
	}
	_ = codesessions.EnsureMeldAccountFolder(acct, rc.CodeSessions)
}

func ensureClaudeConfig(p *paths.Paths, acct *account.Account) {
	if err := claudeconfig.Ensure(acct, p.ClaudeHome(), p.ClaudeJSON()); err != nil {
		fmt.Fprintf(os.Stderr, "note: couldn't prepare %s's Claude Code config (%s); continuing.\n", acct.Meta.Name, err)
	}
}

func Run(p *paths.Paths, acct *account.Account, argv []string) error {
	ensureClaudeConfig(p, acct)
	if len(argv) == 0 {
		argv = []string{"claude"}
	}
	bin, err := exec.LookPath(argv[0])
	if err != nil {
		return err
	}
	// syscall.Exec keeps duplicates, and getenv returns the first: drop one inherited from a Desktop terminal
	env := slices.DeleteFunc(os.Environ(), func(kv string) bool { return strings.HasPrefix(kv, "CLAUDE_CONFIG_DIR=") })
	env = append(env, "CLAUDE_CONFIG_DIR="+claudeconfig.Dir(acct))
	return syscall.Exec(bin, argv, env)
}

func SyncSkills(p *paths.Paths) error {
	accounts, err := account.List(p)
	if err != nil {
		return err
	}
	dst := filepath.Join(p.ClaudeHome(), "skills")
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}

	copied := 0
	for _, a := range accounts {
		results, err := skills.Sync(a, dst)
		for _, r := range results {
			switch {
			case r.Err != nil:
				fmt.Fprintf(os.Stderr, "✗ %s  ← %s: %s\n", r.Name, a.Meta.Name, r.Err)
			case r.Copied:
				copied++
				fmt.Printf("✓ %s  ← %s\n", r.Name, a.Meta.Name)
			default:
				fmt.Printf("· %s  already in %s, kept\n", r.Name, dst)
			}
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "note: skipped the rest of %s's skills (%s)\n", a.Meta.Name, err)
		}
	}
	fmt.Printf("✓ Synced %d skill(s) into %s.\n", copied, dst)
	return nil
}

func Backup(p *paths.Paths, file string, full bool) error {
	if file == "" {
		file = "clauds-backup-" + Now() + ".tar.gz"
	}
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if err := backup.Create(p, f, full); err != nil {
		f.Close()
		os.Remove(file)
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	fmt.Printf("✓ Saved %s.\n", file)
	if !full {
		fmt.Println("  Settings only: no accounts, chats or memory.")
	}
	fmt.Println("  It holds your CLAUDE.md, hooks and MCP env: share it only with people you trust.")
	return nil
}

func Restore(p *paths.Paths, file string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()

	rep, err := backup.Restore(p, f)
	if rep != nil {
		for _, n := range rep.Accounts {
			fmt.Printf("✓ account %s\n", n)
		}
		for _, n := range rep.MergedAccounts {
			fmt.Printf("· account %s already exists, added its missing chats\n", n)
		}
		for _, n := range rep.MCPAdded {
			fmt.Printf("✓ MCP server %s\n", n)
		}
		fmt.Printf("✓ Restored %d file(s), kept %d existing.\n", rep.Added, rep.Kept)
		if len(rep.Accounts) > 0 {
			fmt.Println("  Sign in again: open each account with `clauds <name>`, and `clauds run <name>` for the terminal.")
		}
	}
	return err
}

func Now() string {
	return time.Now().Format("2006-01-02T15-04-05")
}
