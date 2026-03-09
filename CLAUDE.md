# SAME Telegram Bot

Telegram interface for SAME (Stateless Agent Memory Engine). Provides remote vault management through Telegram.

**License:** BSL-1.1

## Architecture

- **Dual mode**: `same-telegram hook` (SAME plugin) and `same-telegram serve` (standalone daemon)
- **Communication**: Unix socket at `~/.same/telegram.sock` (length-prefixed JSON)
- **Management**: Shells out to the `same` CLI — no SAME internals imported
- **Config**: `~/.same/telegram.toml`
- **Security**: All interactions are allowlist-gated by Telegram user ID

## Build and Test

```bash
go build ./cmd/same-telegram   # build the binary
go test ./...                  # run all tests
```

Requires Go 1.25+.

## Project Structure

| Directory | Purpose |
|-----------|---------|
| `cmd/same-telegram/` | CLI entrypoint |
| `internal/bot/` | Telegram bot logic, command handlers |
| `internal/daemon/` | Daemon lifecycle, socket server |

## Code Style

- Standard Go conventions (`gofmt`, `go vet`)
- Conventional commits (e.g., `feat:`, `fix:`, `docs:`)

## Contributing

1. Fork and create a feature branch
2. Ensure `go test ./...` passes
3. Open a pull request against `main`

## Contact

dev@thirty3labs.com
