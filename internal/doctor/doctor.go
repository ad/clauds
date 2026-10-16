package doctor

import (
	"fmt"
	"path/filepath"

	"github.com/ad/clauds/internal/account"
	"github.com/ad/clauds/internal/claudeconfig"
	"github.com/ad/clauds/internal/codesessions"
	"github.com/ad/clauds/internal/config"
	"github.com/ad/clauds/internal/paths"
	"github.com/ad/clauds/internal/process"
)

type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

type AccountHealth struct {
	Slot        uint32   `json:"slot"`
	Name        string   `json:"name"`
	Email       *string  `json:"email,omitempty"`
	Running     bool     `json:"running"`
	Initialized bool     `json:"initialized"`
	ChatsShared bool     `json:"chats_shared"`
	OwnConfig   bool     `json:"own_claude_config"`
	Unshared    []string `json:"unshared,omitempty"`
}

type Report struct {
	Checks   []Check         `json:"checks"`
	Accounts []AccountHealth `json:"accounts"`
	Healthy  bool            `json:"healthy"`
}

func Run(p *paths.Paths, pm process.Manager) (*Report, error) {
	var checks []Check

	appOK := paths.PathExists(p.App)
	checks = append(checks, Check{
		Name:   "Claude.app",
		OK:     appOK,
		Detail: p.App,
	})

	binOK := paths.PathExists(p.AppBinary())
	checks = append(checks, Check{
		Name:   "Claude executable",
		OK:     binOK,
		Detail: p.AppBinary(),
	})

	storeOK := paths.PathExists(p.ClaudsRoot)
	storeDetail := p.ClaudsRoot
	if !storeOK {
		storeDetail = "will be created on first `clauds add`"
	}
	checks = append(checks, Check{
		Name:   "clauds store",
		OK:     true,
		Detail: storeDetail,
	})

	cfg, _ := config.Load(p)
	if cfg == nil {
		cfg = config.Default()
	}
	rc := config.Resolve(p, cfg)

	sharingDetail := "off (clauds config set share-code-sessions on)"
	if rc.ShareSessions {
		sharingDetail = fmt.Sprintf("on → %s", rc.CodeSessions)
	}
	checks = append(checks, Check{
		Name:   "meld chat sharing",
		OK:     true,
		Detail: sharingDetail,
	})

	var accounts []AccountHealth

	acctList, err := account.List(p)
	if err != nil {
		return nil, err
	}

	for _, a := range acctList {
		running, _ := pm.IsRunning(a.DataDir())
		initialized := paths.PathExists(filepath.Join(a.DataDir(), "config.json"))
		chatsShared := codesessions.IsLinked(a, rc.CodeSessions)
		ownConfig := paths.PathExists(claudeconfig.Dir(a))

		accounts = append(accounts, AccountHealth{
			Slot:        a.Meta.Slot,
			Name:        a.Meta.Name,
			Email:       a.Meta.Email,
			Running:     running,
			Initialized: initialized,
			ChatsShared: chatsShared,
			OwnConfig:   ownConfig,
			Unshared:    claudeconfig.Drifted(a, p.ClaudeHome()),
		})
	}

	healthy := appOK && binOK
	return &Report{
		Checks:   checks,
		Accounts: accounts,
		Healthy:  healthy,
	}, nil
}

func PrintHuman(report *Report) {
	fmt.Println("clauds checkup")
	fmt.Println()

	for _, c := range report.Checks {
		symbol := "✗"
		if c.OK {
			symbol = "✓"
		}
		fmt.Printf("  %s %-24s %s\n", symbol, c.Name, c.Detail)
	}

	fmt.Println()
	if len(report.Accounts) == 0 {
		fmt.Println("  No saved accounts yet — run `clauds add <name>`.")
	} else {
		fmt.Println("  Saved accounts:")
		for _, a := range report.Accounts {
			dot := ""
			if a.Running {
				dot = " ● running"
			}
			state := ""
			if !a.Initialized {
				state = "  (not signed in yet — run `clauds open` and log in)"
			}
			shared := ""
			if a.ChatsShared {
				shared = "  ↔ chats shared"
			}
			if a.OwnConfig {
				shared += "  ⧉ own Claude Code config"
			} else {
				shared += "  (shares ~/.claude — reopen to isolate)"
			}
			if a.Email != nil {
				fmt.Printf("    %d. %s  ·  %s%s%s%s\n", a.Slot, a.Name, *a.Email, dot, state, shared)
			} else {
				fmt.Printf("    %d. %s%s%s%s\n", a.Slot, a.Name, dot, state, shared)
			}
			for _, n := range a.Unshared {
				fmt.Printf("       ⚠ %s is no longer linked to ~/.claude/%s\n", n, n)
			}
		}
	}

	overall := "healthy ✓"
	if !report.Healthy {
		overall = "needs attention ✗"
	}
	fmt.Printf("\n  overall: %s\n", overall)
}
