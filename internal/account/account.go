package account

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/ad/clauds/internal/paths"
)

type Meta struct {
	Slot       uint32  `json:"slot"`
	Name       string  `json:"name"`
	Email      *string `json:"email,omitempty"`
	Created    string  `json:"created"`
	LastOpened *string `json:"last_opened,omitempty"`
}

type Account struct {
	Dir  string
	Meta Meta
}

func MetaPath(dir string) string {
	return filepath.Join(dir, "meta.json")
}

func (a *Account) DataDir() string {
	return filepath.Join(a.Dir, "data")
}

func Load(dir string) (*Account, error) {
	mp := MetaPath(dir)
	raw, err := os.ReadFile(mp)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", mp, err)
	}

	var meta Meta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", mp, err)
	}

	return &Account{
		Dir:  dir,
		Meta: meta,
	}, nil
}

func (a *Account) SaveMeta() error {
	data, err := json.MarshalIndent(a.Meta, "", "  ")
	if err != nil {
		return err
	}
	return paths.WriteBytesAtomic(MetaPath(a.Dir), data)
}

func List(p *paths.Paths) ([]*Account, error) {
	dir := p.AccountsDir()
	var accounts []*Account

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return accounts, nil
		}
		return nil, fmt.Errorf("reading %s: %w", dir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		fp := filepath.Join(dir, entry.Name())
		if paths.PathExists(MetaPath(fp)) {
			if acct, err := Load(fp); err == nil {
				accounts = append(accounts, acct)
			}
		}
	}

	sort.Slice(accounts, func(i, j int) bool {
		return accounts[i].Meta.Slot < accounts[j].Meta.Slot
	})

	return accounts, nil
}

func Resolve(p *paths.Paths, query string) (*Account, error) {
	accounts, err := List(p)
	if err != nil {
		return nil, err
	}

	if len(accounts) == 0 {
		return nil, fmt.Errorf("no saved accounts yet — run `clauds add <name>` first")
	}

	if n, err := strconv.ParseUint(query, 10, 32); err == nil {
		for _, a := range accounts {
			if a.Meta.Slot == uint32(n) {
				return a, nil
			}
		}
	}

	q := strings.ToLower(query)
	for _, a := range accounts {
		if strings.ToLower(a.Meta.Name) == q {
			return a, nil
		}
	}

	for _, a := range accounts {
		if a.Meta.Email != nil && strings.ToLower(*a.Meta.Email) == q {
			return a, nil
		}
	}

	var matches []*Account
	for _, a := range accounts {
		if strings.HasPrefix(strings.ToLower(a.Meta.Name), q) {
			matches = append(matches, a)
		}
	}

	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return nil, fmt.Errorf("no saved account matches \"%s\" (try `clauds ls`)", query)
	default:
		var names []string
		for _, a := range matches {
			names = append(names, a.Meta.Name)
		}
		return nil, fmt.Errorf("\"%s\" is ambiguous — matches: %s", query, strings.Join(names, ", "))
	}
}

func NextSlot(p *paths.Paths) (uint32, error) {
	accounts, err := List(p)
	if err != nil {
		return 0, err
	}

	var maxSlot uint32
	for _, a := range accounts {
		if a.Meta.Slot > maxSlot {
			maxSlot = a.Meta.Slot
		}
	}
	return maxSlot + 1, nil
}

func Dirname(slot uint32, name string) string {
	safe := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' || r == '@' {
			return r
		}
		return '_'
	}, name)
	return fmt.Sprintf("%d-%s", slot, safe)
}

func Create(p *paths.Paths, name string, email *string, created string) (*Account, error) {
	accounts, err := List(p)
	if err != nil {
		return nil, err
	}

	for _, a := range accounts {
		if strings.EqualFold(a.Meta.Name, name) {
			return nil, fmt.Errorf("an account named \"%s\" already exists (use `clauds open %s`)", name, name)
		}
	}

	slot, err := NextSlot(p)
	if err != nil {
		return nil, err
	}

	dir := filepath.Join(p.AccountsDir(), Dirname(slot, name))
	account := &Account{
		Dir: dir,
		Meta: Meta{
			Slot:    slot,
			Name:    name,
			Email:   email,
			Created: created,
		},
	}

	dataDir := account.DataDir()
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("creating %s: %w", dataDir, err)
	}

	if err := account.SaveMeta(); err != nil {
		return nil, err
	}

	return account, nil
}

func Delete(account *Account) error {
	if paths.PathExists(account.Dir) {
		if err := os.RemoveAll(account.Dir); err != nil {
			return fmt.Errorf("removing %s: %w", account.Dir, err)
		}
	}
	return nil
}
