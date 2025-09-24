# Repository Guidelines
## Project Structure & Module Organization
- `cmd/slack-mcp-server`: Main entrypoint (flags: `--transport|-t`).
- `pkg/handler`, `pkg/server`, `pkg/provider`, `pkg/transport`, `pkg/text`, `pkg/version`, `pkg/limiter`: Core modules for tools, server wiring, Slack API access, transports, text utils, versioning, and rate limits.
- `docs/`: Setup, install, and configuration guides.
- `build/`: Compiled binaries and DXT packaging artifacts (not committed).
- `npm/`: NPM wrappers and platform-specific binary packages.
- `images/`: Project assets. Cache files like `.users_cache.json` and `.channels_cache_v2.json` are ignored by Git.
## Build, Test, and Development Commands
- `make help`: List available tasks.
- `make build`: Build binary to `build/slack-mcp-server`.
- `go run ./cmd/slack-mcp-server -t stdio`: Run locally; set `SLACK_MCP_*` envs (e.g., `SLACK_MCP_XOXP_TOKEN` or `SLACK_MCP_XOXC_TOKEN`/`SLACK_MCP_XOXD_TOKEN`).
- `make test`: Run unit tests (filters tests containing "Unit").
- `make test-integration`: Run integration tests (requires `SLACK_MCP_OPENAI_API` and Slack creds).
- `make format` / `make tidy` / `make deps`: Format code, tidy modules, download deps.
## Coding Style & Naming Conventions
- **Language**: Go 1.24; use idiomatic Go.
- **Formatting**: Enforced via `go fmt` (`make format`).
- **Packages**: Short lowercase names; exported identifiers use CamelCase.
- **Tests**: Files end with `_test.go`.
- **Logging**: Prefer injected `zap` logger; honor `SLACK_MCP_LOG_LEVEL`, `SLACK_MCP_LOG_FORMAT`, `SLACK_MCP_LOG_COLOR`.
## Testing Guidelines
- **Frameworks**: `go test` with `testify` assertions.
- **Scope**: Keep unit tests fast/deterministic; isolate external calls in integration tests.
- **Naming**: Include “Unit” or “Integration” in test names (Makefile filters).
- **Examples**: `go test ./pkg/handler -run "Unit"` to run a package’s unit tests.
## Commit & Pull Request Guidelines
- **Commits**: Conventional-ish prefixes (`feat:`, `fix:`, `chore:`, `docs:`, `enhance:`).
- **PRs**: Clear description, linked issues, tests for changes, and doc updates (`README.md`/`docs/`) when behavior changes. Include sample output or screenshots if UX-affecting.
- **Pre-submit**: Ensure `make format`, `make tidy`, and `make test` pass locally.
## Security & Configuration Tips
- **Secrets**: Never commit tokens. Use local env (`.envrc-personal`, shell env vars).
- **Caches**: Slack cache files are ignored by `.gitignore` and should not be committed.
