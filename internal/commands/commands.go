package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/ad/clauds/internal/account"
	"github.com/ad/clauds/internal/codesessions"
	"github.com/ad/clauds/internal/config"
	"github.com/ad/clauds/internal/doctor"
	"github.com/ad/clauds/internal/paths"
	"github.com/ad/clauds/internal/process"
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
		pm.Activate(p.App)
		fmt.Printf("✓ %s is already open — bringing it to the front.\n", acct.Meta.Name)
		return nil
	}

	cfg, _ := config.Load(p)
	if cfg == nil {
		cfg = config.Default()
	}
	rc := config.Resolve(p, cfg)
	ensureSharedSessions(rc, acct)

	if err := pm.Launch(p.App, data); err != nil {
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
		if err := pm.Launch(p.App, a.DataDir()); err != nil {
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

	if err := pm.Launch(p.App, acct.DataDir()); err != nil {
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

func Now() string {
	return time.Now().Format("2006-01-02T15-04-05")
}
