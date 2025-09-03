# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build Commands

```bash
# Build the project
make build

# Build for all platforms
make build-all-platforms

# Run tests
make test

# Run integration tests (requires SLACK_MCP_OPENAI_API env var)
make test-integration

# Format code
make format

# Tidy Go modules
make tidy

# Clean build artifacts
make clean

# Create release
make release TAG=v1.2.3
```

## Architecture Overview

This is a Model Context Protocol (MCP) server for Slack workspaces written in Go. The codebase follows a clean architecture pattern with clear separation of concerns:

### Core Components

- **cmd/slack-mcp-server/**: Entry point containing main.go
- **pkg/server/**: MCP server setup and tool registration
- **pkg/handler/**: Business logic for MCP tools (conversations, channels)
- **pkg/provider/**: Slack API abstraction layer with edge client implementation
- **pkg/transport/**: Transport layer handling stdio/SSE protocols

### Key Patterns

1. **Handler Pattern**: All MCP tools follow `ConversationsHandler` pattern with dependency injection
2. **Response Format**: Consistent CSV output using `gocsv.MarshalBytes()`
3. **Error Handling**: Structured logging with Zap throughout
4. **Configuration**: Environment variables control features (e.g., `SLACK_MCP_ADD_MESSAGE_TOOL`)

### Authentication

The server supports multiple authentication modes:
- **Stealth Mode**: Uses `xoxc` + `xoxd` browser tokens
- **OAuth Mode**: Uses `xoxp` user OAuth tokens
- **Enterprise Support**: Custom TLS handshake and user-agent configuration

### Message Reactions Format

Messages include reactions in the format: `emoji:count:user1,user2` (enhanced in commit 012f4bf)

### Enterprise Grid Implementation Pattern

For additional functionality: Try standard API with Enterprise Grid parameters (e.g., `team_id`) first. If that fails, fallback to implementing in Edge client using curl request/response examples from user.

## Development Workflow

### Current Branch Strategy
- Main branch: `master`
- Feature branches follow pattern: `initials/feature-name`

### Testing Approach
- Unit tests: Standard Go testing in `*_test.go` files
- Integration tests: Require `SLACK_MCP_OPENAI_API` environment variable
- Test single file: `go test -v ./pkg/handler/conversations_test.go`

### Environment Variables

Critical variables for development:
- `SLACK_MCP_XOXC_TOKEN` / `SLACK_MCP_XOXD_TOKEN`: Browser auth tokens
- `SLACK_MCP_XOXP_TOKEN`: OAuth token (alternative to xoxc/xoxd)
- `SLACK_MCP_ADD_MESSAGE_TOOL`: Enable message posting (comma-separated channel IDs)
- `SLACK_MCP_REACTION_TOOLS`: Enable reaction management (planned feature)