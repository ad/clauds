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
| `clauds run <name> [-- cmd]` | Run `cmd` (default `claude`) with that account's Claude Code config |
| `clauds skills sync` | Copy accounts' own claude.ai skills into `~/.claude/skills` |
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

## Claude Code isolation

Claude Desktop alone only separates its own data via `--user-data-dir`. The Code tab and terminal `claude` would still share `~/.claude` and `~/.claude.json`: one machineID/userID, one history, one memory.

So each account also gets its own `CLAUDE_CONFIG_DIR` at `accounts/<slot>-<name>/claude-config/`, passed to Claude Desktop on launch:

- **Shared** (symlinks to `~/.claude`): `settings.json`, `settings.local.json`, `keybindings.json`, top-level `*.md`, `agents`, `commands`, `skills`, `hooks`, `plugins`, `output-styles`, `rules`.
- **Per account**: `.claude.json` (seeded with your `mcpServers` only), sign-in, `projects` (transcripts and memory), history, sessions, telemetry, caches.

Links are created on every open and never overwrite existing files. If Claude Code replaces a link with a regular file, `clauds doctor` shows it.

Existing accounts switch over on their next launch. Quit them first: an already running window keeps the old shared config. Code sessions started before the switch keep their transcripts in `~/.claude/projects`.

For the terminal:

```bash
clauds run work                    # claude as the work account
clauds run work -- claude --resume
```

The first `clauds run` asks you to sign in: Claude Code keys its Keychain entry by config dir.

### Skills

`~/.claude/skills` is shared, so every account's Claude Code sees the same skills.

Skills uploaded to a claude.ai account stay on that account. Claude Desktop caches them locally, and `clauds skills sync` copies the ones you created (not Anthropic's built-ins) into `~/.claude/skills`:

```bash
$ clauds skills sync
✓ golang-pro  ← work
✓ Synced 1 skill(s) into /Users/you/.claude/skills.
```

A skill already in `~/.claude/skills` is never overwritten. To pick up a newer version, delete the local copy and sync again. Open each account in Desktop once beforehand so its cache is current.

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
