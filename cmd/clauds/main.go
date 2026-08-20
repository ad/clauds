package main

import (
	"fmt"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"

	"github.com/ad/clauds/internal/account"
	"github.com/ad/clauds/internal/commands"
	"github.com/ad/clauds/internal/config"
	"github.com/ad/clauds/internal/paths"
	"github.com/ad/clauds/internal/process"
)

var version = "dev"

var reserved = []string{"open", "add", "ls", "rm", "doctor", "config", "all"}

func main() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, "\nclauds: interrupted")
		os.Exit(130)
	}()

	if err := dispatch(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "clauds: %s\n", err)
		os.Exit(1)
	}
}

func dispatch(args []string) error {
	p, err := paths.Resolve()
	if err != nil {
		return err
	}

	cfg, err := config.Load(p)
	if err == nil && cfg.AppPath != nil {
		p.App = *cfg.AppPath
	}

	pm := &process.ExecManager{}

	rest := args[1:]
	if len(rest) == 0 {
		return commands.Picker(p, pm)
	}

	first := rest[0]

	switch first {
	case "-h", "--help":
		printUsage()
		return nil
	case "-v", "--version":
		fmt.Printf("clauds %s\n", version)
		return nil
	}

	if strings.HasPrefix(first, "-") || slices.Contains(reserved, first) {
		return parseAndRun(p, pm, args)
	}

	synth := append([]string{args[0], "open"}, rest...)
	return parseAndRun(p, pm, synth)
}

func parseAndRun(p *paths.Paths, pm process.Manager, args []string) error {
	if len(args) < 2 {
		printUsage()
		return nil
	}

	cmd := args[1]
	switch cmd {
	case "-h", "--help":
		printUsage()
		return nil
	case "-v", "--version":
		fmt.Printf("clauds %s\n", version)
		return nil
	case "open":
		if len(args) < 3 {
			return fmt.Errorf("missing account name — usage: clauds open <name>")
		}
		acct, err := account.Resolve(p, args[2])
		if err != nil {
			return err
		}
		return commands.OpenAccount(p, pm, acct)

	case "all":
		return commands.OpenAll(p, pm)

	case "add":
		if len(args) < 3 {
			return fmt.Errorf("missing account name — usage: clauds add <name>")
		}
		name := args[2]
		var email *string
		for i := 3; i < len(args)-1; i++ {
			if args[i] == "--email" && i+1 < len(args) {
				e := args[i+1]
				email = &e
				break
			}
		}
		return commands.AddAccount(p, pm, name, email)

	case "ls":
		return commands.List(p, pm, hasFlag(args, "--json"))

	case "rm":
		if len(args) < 3 {
			return fmt.Errorf("missing account name — usage: clauds rm <name>")
		}
		acct, err := account.Resolve(p, args[2])
		if err != nil {
			return err
		}
		return commands.RemoveAccount(p, pm, acct)

	case "doctor":
		return commands.DoctorCmd(p, pm, hasFlag(args, "--json"))

	case "config":
		return commands.ConfigCmd(p, args[2:])

	default:
		acct, err := account.Resolve(p, cmd)
		if err != nil {
			return err
		}
		return commands.OpenAccount(p, pm, acct)
	}
}

func printUsage() {
	fmt.Println(`clauds — Claude Desktop account switcher

USAGE
  clauds                     interactive account picker
  clauds <name>              open that account's window
  clauds all                 open every account's window
  clauds add <name>          save a new account
  clauds ls                  list saved accounts
  clauds rm <name>           forget an account
  clauds doctor              check environment health
  clauds config [action]     show or set preferences

FLAGS
  -h, --help                show this help
  -v, --version             show version
  --json                    JSON output (ls, doctor)

EXAMPLES
  clauds add work --email you@company.com
  clauds personal
  clauds ls --json`)
}

func hasFlag(args []string, flag string) bool {
	return slices.Contains(args, flag)
}
