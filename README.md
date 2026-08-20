# clauds

Claude Desktop account switcher for macOS. Run multiple Claude accounts in separate windows, each with its own data directory and login state.

```
$ clauds
? Open which account?
❯ ★ Open ALL accounts
  1. work  ·  you@company.com
  2. personal  ●
```

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/ad/clauds/main/install.sh | bash
```

Or build from source:

```bash
git clone https://github.com/ad/clauds
cd clauds
go build -o /usr/local/bin/clauds ./cmd/clauds
```

Requires macOS and Go 1.26+ (for source builds).

## Quick start

```bash
clauds add work --email you@company.com   # create account, opens Claude
clauds add personal                       # another one
clauds ls                                 # see all accounts
clauds work                               # switch to work account
clauds                                    # interactive picker
```

## Commands

| Command | Description |
|---------|-------------|
| `clauds` | Interactive account picker |
| `clauds <name>` | Open that account |
| `clauds all` | Open every account |
| `clauds add <name>` | Create a new account |
| `clauds ls` | List saved accounts |
| `clauds rm <name>` | Remove an account |
| `clauds doctor` | Check environment health |
| `clauds config` | View or set preferences |

### Flags

| Flag | Description |
|------|-------------|
| `--json` | JSON output (works with `ls`, `doctor`) |
| `-h`, `--help` | Show help |
| `-v`, `--version` | Show version |

## Configuration

```bash
clauds config                    # show all settings
clauds config get app-path       # read a setting
clauds config set app-path /path/to/Claude.app
```

| Key | Default | Description |
|-----|---------|-------------|
| `app-path` | `/Applications/Claude.app` | Path to Claude.app |
| `share-code-sessions` | `on` | Symlink chat folders for meld |
| `code-sessions-root` | `~/Library/Application Support/Claude/claude-code-sessions` | Where shared chats live |

### Meld integration

clauds can symlink each account's `claude-code-sessions` folder to a shared location so [Meld](https://meld.claude.ai) sees all your chats across accounts. Enable with:

```bash
clauds config set share-code-sessions on
```

## Account resolution

When you type `clauds work`, it resolves by:

1. Exact slot number (`clauds 1`)
2. Exact name match (`clauds work`)
3. Exact email match (`clauds you@company.com`)
4. Prefix match (`clauds wo` → `work`)

## Health check

```bash
$ clauds doctor
clauds checkup

  ✓ Claude.app              /Applications/Claude.app
  ✓ Claude executable       /Applications/Claude.app/Contents/MacOS/Claude
  ✓ clauds store            /Users/you/.config/clauds
  ✓ meld chat sharing       on → /Users/you/Library/Application Support/Claude/claude-code-sessions

  Saved accounts:
    1. work  ·  you@company.com ● running  ↔ chats shared
    2. personal

  overall: healthy ✓
```

## Project structure

```
clauds/
├── cmd/clauds/
│   └── main.go              # entry point, CLI dispatch
├── internal/
│   ├── account/             # account model, CRUD, resolution
│   ├── codesessions/        # meld/chat symlink management
│   ├── commands/            # command implementations + picker
│   ├── config/              # tool configuration + resolution
│   ├── doctor/              # environment health checks
│   ├── paths/               # filesystem paths + shared helpers
│   └── process/             # process management interface
├── go.mod
├── install.sh
└── README.md
```

### Dependency direction

```
cmd/clauds ──→ commands ──→ account
                  │             ↑
                  ├─→ config ───┘
                  │      ↑
                  ├─→ codesessions
                  │
                  ├─→ doctor ──→ account, config, codesessions
                  │
                  ├─→ process
                  │
                  └─→ paths (leaf — no internal deps)
```

## License

MIT
