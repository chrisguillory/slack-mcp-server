#!/bin/bash

# MCP server wrapper that runs the Slack MCP server in Docker with live logs
# This version uses Docker Compose for better log visibility and management

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
else
    export SLACK_MCP_LOG_LEVEL="debug"
fi

# Verify Docker is available
if ! command -v docker &> /dev/null; then
    echo "Error: Docker is not installed or not in PATH" >&2
    # Show dialog to user
    osascript -e 'display dialog "Docker Not Found\n\nDocker is not installed or not in PATH.\n\nPlease install Docker Desktop from:\nhttps://www.docker.com/products/docker-desktop\n\nOr ensure it is in your PATH." buttons {"OK"} default button "OK" with icon stop'
    exit 1
fi

# Verify Docker Compose is available
if ! command -v docker-compose &> /dev/null && ! docker compose version &> /dev/null; then
    echo "Error: Docker Compose is not installed" >&2
    osascript -e 'display dialog "Docker Compose Not Found\n\nDocker Compose is not installed.\n\nIt should come with Docker Desktop.\n\nPlease ensure Docker Desktop is properly installed." buttons {"OK"} default button "OK" with icon stop'
    exit 1
fi

# Determine docker-compose command (v1 vs v2)
if docker compose version &> /dev/null; then
    DOCKER_COMPOSE="docker compose"
else
    DOCKER_COMPOSE="docker-compose"
fi

# Pull/update the Go image if needed
echo "Ensuring Go Docker image is up to date..."
docker pull golang:1.24

# Clean up any existing container
echo "Cleaning up any existing container..."
$DOCKER_COMPOSE -f docker-compose.local.yml down 2>/dev/null || true

# Start the MCP server with Docker Compose
echo "Starting Slack MCP server with Docker Compose..."
echo "======================================"
echo "Logs will appear below. Press Ctrl+C to stop."
echo "======================================"
echo ""

# Run with docker-compose run for proper stdio handling with Cursor
# The 'run' command connects stdin/stdout properly for MCP communication
exec $DOCKER_COMPOSE -f docker-compose.local.yml run --rm slack-mcp-local