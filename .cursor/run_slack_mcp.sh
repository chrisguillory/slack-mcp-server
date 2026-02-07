#!/bin/bash

# MCP server wrapper that ensures direnv is loaded before running the Slack MCP server

# Unofficial bash strict mode - http://redsymbol.net/articles/unofficial-bash-strict-mode/
set -euo pipefail

# Get the directory where this script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# The script is in .cursor/, so go up one level to get to the slack-mcp-server root
REPO_ROOT="$(dirname "$SCRIPT_DIR")"

# Change to the repo root to ensure we're in the right context
cd "$REPO_ROOT"

# Load direnv environment (this will load .envrc-personal and any other .envrc files)
eval "$(direnv export bash)"

# Set Slack MCP environment variables
export SLACK_MCP_CUSTOM_TLS="true"
export SLACK_MCP_USER_AGENT="Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36"

# Check if Slack tokens are available from environment or .envrc-personal
if [ -z "${SLACK_MCP_XOXC_TOKEN:-}" ] || [ -z "${SLACK_MCP_XOXD_TOKEN:-}" ]; then
    echo "Error: Slack MCP tokens not found in environment" >&2
    # Show dialog to user
    osascript -e 'display dialog "Missing Slack MCP Tokens\n\nAdd the following to slack-mcp-server/.envrc-personal:\nexport SLACK_MCP_XOXC_TOKEN=your_xoxc_token_here\nexport SLACK_MCP_XOXD_TOKEN=your_xoxd_token_here\n\nOr set them directly in your shell environment." buttons {"OK"} default button "OK" with icon stop'
    exit 1
fi

# Optional: Set additional environment variables if they exist
if [ -n "${SLACK_MCP_ADD_MESSAGE_TOOL:-}" ]; then
	export SLACK_MCP_ADD_MESSAGE_TOOL
fi

if [ -n "${SLACK_MCP_ADD_REACTION_TOOL:-}" ]; then
	export SLACK_MCP_ADD_REACTION_TOOL
fi

if [ -n "${SLACK_MCP_LOG_LEVEL:-}" ]; then
    export SLACK_MCP_LOG_LEVEL
fi

# Verify Go is available
if ! command -v go &> /dev/null; then
    echo "Error: Go is not installed or not in PATH" >&2
    # Show dialog to user
    osascript -e 'display dialog "Go Not Found\n\nGo is not installed or not in PATH.\n\nPlease install Go from:\nhttps://golang.org/dl/\n\nOr ensure it is in your PATH." buttons {"OK"} default button "OK" with icon stop'
    exit 1
fi

# Verify the project dependencies are ready
if [ ! -f "go.mod" ]; then
    echo "Error: go.mod not found. Are you in the correct directory?" >&2
    # Show dialog to user
    osascript -e 'display dialog "Project Structure Error\n\ngo.mod not found. Are you in the correct directory?\n\nExpected location: slack-mcp-server/\nCurrent location: $(pwd)" buttons {"OK"} default button "OK" with icon stop'
    exit 1
fi

# Ensure dependencies are up to date
echo "Ensuring Go dependencies are up to date..."
go mod tidy

# Build and run the MCP server
echo "Building Slack MCP server..." >&2
go build -o ./build/slack-mcp-server ./cmd/slack-mcp-server
echo "Starting Slack MCP server..." >&2
exec ./build/slack-mcp-server mcp-server --transport stdio
