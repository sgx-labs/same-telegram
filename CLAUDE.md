# same-telegram

Telegram plugin for SAME (Stateless Agent Memory Engine). Turns Telegram into a remote management GUI for SAME vaults.

## Architecture

- **Single binary, dual mode**: `same-telegram hook` (plugin) + `same-telegram serve` (daemon)
- **Hook -> daemon**: Unix socket at `~/.same/telegram.sock` (length-prefixed JSON)
- **Management**: Daemon shells out to `same` CLI — no SAME internals imported
- **Config**: `~/.same/telegram.toml` (TOML)

## Build

```bash
make build    # builds ./same-telegram
make test     # runs all tests
make install  # copies to ~/go/bin/
```

## Key Patterns

- Plugin receives `HookInput` JSON on stdin, outputs `HookOutput` JSON on stdout
- Daemon lifecycle mirrors SAME's serve_cmd.go: PID file, background re-exec, SIGTERM shutdown
- All Telegram interactions are whitelist-gated by user ID
- Notifications silently drop if daemon isn't running (hooks must not block)
