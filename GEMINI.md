# Gemini Code Assistant Context

This document provides context for the Gemini Code Assistant to understand the `slack-mcp-server` project.

## Project Overview

`slack-mcp-server` is a [Model Context Protocol (MCP)](https://github.com/mark3labs/mcp) server for Slack workspaces. It allows AI agents and other tools to interact with Slack in a standardized way. The server is written in Go and can be run as a standalone binary.

The server supports two transport methods:

*   **`stdio`**: for local communication with a single client.
*   **`sse`**: for serving multiple clients over HTTP using Server-Sent Events.

It authenticates with Slack using either OAuth tokens (`xoxp-...`) or session-based tokens (`xoxc-...` and `xoxd-...`).

The server provides a set of tools for interacting with Slack, including:

*   `conversations_history`: Get messages from a channel.
*   `conversations_replies`: Get a thread of messages.
*   `conversations_add_message`: Add a message to a channel.
*   `conversations_search_messages`: Search for messages.
*   `channels_list`: Get a list of channels.

It also exposes two resources:

*   `slack://<workspace>/channels`: A CSV directory of all channels.
*   `slack://<workspace>/users`: A CSV directory of all users.

## Building and Running

The project uses a `Makefile` for common tasks.

*   **To build the binary:**
    ```bash
    make build
    ```

*   **To run the server with stdio transport:**
    ```bash
    go run ./cmd/slack-mcp-server --transport stdio
    ```

*   **To run the server with SSE transport:**
    ```bash
    go run ./cmd/slack-mcp-server --transport sse
    ```

The server is configured using environment variables. The most important ones are `SLACK_MCP_XOXP_TOKEN` or `SLACK_MCP_XOXC_TOKEN` and `SLACK_MCP_XOXD_TOKEN` for authentication.

## Development Conventions

The project uses [Go Modules](https://go.dev/blog/using-go-modules) for dependency management. The code is organized into several packages:

*   `cmd/slack-mcp-server`: The main application package.
*   `pkg/provider`: Handles communication with the Slack API.
*   `pkg/server`: Implements the MCP server.
*   `pkg/handler`: Implements the tool handlers.

The code is well-structured and includes unit tests. To run the tests, use the following command:

```bash
make test
```
