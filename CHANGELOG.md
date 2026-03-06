# Changelog

## Unreleased

### Added

- Multi-backend AI: store API keys per backend, switch with `/ai` command
- Conversation memory: sliding window (10 pairs, 8KB, 30-min TTL)
- `/new`, `/clear`, `/reset` commands for conversation management
- `/cancel` command for in-flight AI requests
- Typing indicators on long operations
- 32KB prompt length limit
- Unsupported message type replies
- Thinking model compatibility: `stripThinkingTokens()` on all 4 AI backends
- Dockerfile with non-root user for Fly.io deployment
- Fly.io deployment config (fly.toml, docs/DEPLOY.md)

### Fixed

- `/ai off` now actually disables AI (was in-memory only, persistent store not updated)
- `/ai on` shows correct backend name (was always showing "claude")
- Gemini default model updated from deprecated `gemini-2.0-flash` to `gemini-2.5-flash`
- Rate limiter is now non-blocking
- Conversation history strips thinking tokens before storing
- Public mode: restricted command set, onboarding skips CLI option
- Various security fixes: callback bypass, audit log sanitization, background memory eviction

### Changed

- `/claude` deprecated, now alias for `/ai`
