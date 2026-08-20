package commands

import (
	"bufio"
	"fmt"
	"os"

	"github.com/ad/clauds/internal/account"
	"github.com/ad/clauds/internal/paths"
	"github.com/ad/clauds/internal/process"
)

func Picker(p *paths.Paths, pm process.Manager) error {
	accounts, err := account.List(p)
	if err != nil {
		return err
	}

	if len(accounts) == 0 {
		fmt.Println("No saved accounts yet. Run `clauds add <name>` to save one.")
		return nil
	}

	var running []bool
	for _, a := range accounts {
		r, _ := pm.IsRunning(a.DataDir())
		running = append(running, r)
	}

	items := []string{"★ Open ALL accounts"}
	for i, a := range accounts {
		dot := ""
		if running[i] {
			dot = " ●"
		}
		if a.Meta.Email != nil {
			items = append(items, fmt.Sprintf("%d. %s%s  ·  %s", a.Meta.Slot, a.Meta.Name, dot, *a.Meta.Email))
		} else {
			items = append(items, fmt.Sprintf("%d. %s%s", a.Meta.Slot, a.Meta.Name, dot))
		}
	}

	return interactivePicker(items, func(selected int) error {
		if selected == 0 {
			return OpenAll(p, pm)
		}
		return OpenAccount(p, pm, accounts[selected-1])
	})
}

func interactivePicker(items []string, callback func(int) error) error {
	reader := bufio.NewReader(os.Stdin)
	cursor := 0

	fmt.Print("\033[?25l")
	defer fmt.Print("\033[?25h")

	render := func() {
		fmt.Print("\033[?25l")
		for i, item := range items {
			fmt.Printf("\033[1A\033[2K")
			if i == cursor {
				fmt.Printf("\033[1m❯ %s\033[0m\n", item)
			} else {
				fmt.Printf("  %s\n", item)
			}
		}
		fmt.Print("\033[?25h")
	}

	fmt.Println("Open which account?")
	for i, item := range items {
		if i == cursor {
			fmt.Printf("\033[1m❯ %s\033[0m\n", item)
		} else {
			fmt.Printf("  %s\n", item)
		}
	}

	buf := make([]byte, 3)
	for {
		n, err := reader.Read(buf[:3])
		if err != nil {
			return err
		}

		if n == 1 && buf[0] == '\n' {
			fmt.Print("\033[?25h")
			return callback(cursor)
		}

		if n == 3 && buf[0] == '\033' && buf[1] == '[' {
			switch buf[2] {
			case 'A':
				if cursor > 0 {
					cursor--
					render()
				}
			case 'B':
				if cursor < len(items)-1 {
					cursor++
					render()
				}
			}
		}
	}
}
