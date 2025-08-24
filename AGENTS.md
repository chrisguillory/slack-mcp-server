# Repository Guidelines

## Project Structure & Module Organization
- `cmd/slack-mcp-server`: Main entrypoint (flags: `--transport| -t`).
- `pkg/handler`, `pkg/server`, `pkg/provider`, `pkg/transport`, `pkg/text`, `pkg/version`, `pkg/limiter`: Core modules (tools, server wiring, Slack API access, transport, text utils, versioning, rate limits).
- `docs/`: Setup, install, and configuration guides.
- `build/`: Compiled binaries and DXT packaging artifacts (not committed).
- `npm/`: NPM wrappers and platform-specific binary packages.
- `images/`: Project assets. Cache files like `.users_cache.json` and `.channels_cache_v2.json` are ignored by Git.

## Build, Test, and Development Commands
- `make help`: Show available tasks.
- `make build`: Build binary to `build/slack-mcp-server`.
- `go run ./cmd/slack-mcp-server -t stdio`: Run locally; set `SLACK_MCP_*` envs (e.g., `SLACK_MCP_XOXP_TOKEN` or `SLACK_MCP_XOXC_TOKEN`/`SLACK_MCP_XOXD_TOKEN`).
- `make test`: Run unit tests (filters tests containing “Unit”).
- `make test-integration`: Run integration tests (requires `SLACK_MCP_OPENAI_API` and Slack creds).
- `make format` / `make tidy` / `make deps`: Format code, tidy modules, download deps.
- Advanced: `make build-all-platforms`, `make build-dxt`, `make npm-publish`.

## Coding Style & Naming Conventions
- Language: Go (1.24). Use idiomatic Go; formatting enforced via `go fmt` (`make format`).
- Packages: short lowercase names; exported identifiers use CamelCase; test files end with `_test.go`.
- Logging: prefer injected `zap` logger; honor `SLACK_MCP_LOG_LEVEL`, `SLACK_MCP_LOG_FORMAT`, and `SLACK_MCP_LOG_COLOR`.

## Testing Guidelines
- Frameworks: `go test` with `testify` assertions. Keep unit tests fast and deterministic; isolate external calls in integration tests.
- Naming: include “Unit” or “Integration” in test names to match Makefile filters.
- Examples: run a package’s unit tests with `go test ./pkg/handler -run "Unit"`.

## Commit & Pull Request Guidelines
- Commit style: conventional-ish prefixes (`feat:`, `fix:`, `chore:`, `docs:`, `enhance:`) observed in history.
- PRs: clear description, linked issues, tests for changes, and doc updates (`README.md`/`docs/`) when behavior changes. Include sample output or screenshots if UX-affecting.
- Pre-submit: `make format`, `make tidy`, and `make test` should pass locally.

## Security & Configuration Tips
- Never commit tokens; use local env (`.envrc-personal`, shell env vars). Cached Slack data files are ignored by `.gitignore`.
