# GEMINI Reactions Implementation Plan

## Overview

This document outlines a plan to add reaction management capabilities to the Slack MCP server. This will allow MCP clients to add, remove, and get reactions on Slack messages.

This plan is an alternative to `REACTIONS_IMPLEMENTATION_PLAN.md`, and is based on a thorough analysis of the existing codebase.

## New Tools

We will add three new tools:

1.  `conversations_add_reaction`: Adds an emoji reaction to a message.
2.  `conversations_remove_reaction`: Removes an emoji reaction from a message.
3.  `conversations_get_reactions`: Retrieves all reactions for a given message.

## Implementation Details

### 1. API Provider (`pkg/provider/api.go`)

The `SlackAPI` interface will be extended to include methods for managing reactions.

```go
// In pkg/provider/api.go

type SlackAPI interface {
    // ... existing methods
    AddReactionContext(ctx context.Context, name string, item slack.ItemRef) error
    RemoveReactionContext(ctx context.Context, name string, item slack.ItemRef) error
    GetReactionsContext(ctx context.Context, item slack.ItemRef, params slack.GetReactionsParameters) ([]slack.ItemReaction, error)
}
```

These new methods will be implemented in the `MCPSlackClient` using the existing `slack-go/slack` library.

### 2. Handler (`pkg/handler/conversations.go`)

The `ConversationsHandler` will be updated to include handlers for the new reaction tools.

```go
// In pkg/handler/conversations.go

func (ch *ConversationsHandler) ConversationsAddReactionHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    // ... implementation
}

func (ch *ConversationsHandler) ConversationsRemoveReactionHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    // ... implementation
}

func (ch *ConversationsHandler) ConversationsGetReactionsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    // ... implementation
}
```

These handlers will be responsible for parsing tool parameters, calling the API provider, and formatting the results.

### 3. Tool Registration (`pkg/server/server.go`)

The new tools will be registered in the `NewMCPServer` function.

```go
// In pkg/server/server.go

func NewMCPServer(provider *provider.ApiProvider, logger *zap.Logger) *MCPServer {
    // ... existing tool registrations

    s.AddTool(mcp.NewTool("conversations_add_reaction",
        mcp.WithDescription("Add a reaction to a message."),
        mcp.WithString("channel_id", mcp.Required(), mcp.Description("ID of the channel containing the message.")),
        mcp.WithString("timestamp", mcp.Required(), mcp.Description("Timestamp of the message to react to.")),
        mcp.WithString("reaction_name", mcp.Required(), mcp.Description("Name of the emoji to add (e.g., 'thumbsup').")),
    ), conversationsHandler.ConversationsAddReactionHandler)

    s.AddTool(mcp.NewTool("conversations_remove_reaction",
        mcp.WithDescription("Remove a reaction from a message."),
        mcp.WithString("channel_id", mcp.Required(), mcp.Description("ID of the channel containing the message.")),
        mcp.WithString("timestamp", mcp.Required(), mcp.Description("Timestamp of the message to remove the reaction from.")),
        mcp.WithString("reaction_name", mcp.Required(), mcp.Description("Name of the emoji to remove.")),
    ), conversationsHandler.ConversationsRemoveReactionHandler)

    s.AddTool(mcp.NewTool("conversations_get_reactions",
        mcp.WithDescription("Get all reactions for a message."),
        mcp.WithString("channel_id", mcp.Required(), mcp.Description("ID of the channel containing the message.")),
        mcp.WithString("timestamp", mcp.Required(), mcp.Description("Timestamp of the message to get reactions from.")),
    ), conversationsHandler.ConversationsGetReactionsHandler)

    // ... rest of the function
}
```

### 4. Return Format

*   **`conversations_add_reaction` / `conversations_remove_reaction`**:
    *   On success, will return a simple success message.
    *   On failure, will return a detailed error message.
*   **`conversations_get_reactions`**:
    *   On success, will return a CSV-formatted string with the reactions. The CSV will have the following columns: `reaction_name`, `users`.
    *   On failure, will return a detailed error message.

### 5. Testing

*   **Unit Tests**: We will add unit tests for the new handler methods in `pkg/handler/conversations_test.go` to ensure they correctly parse parameters and handle errors.
*   **Integration Tests**: We will add integration tests that interact with a real Slack workspace to verify that the new tools work as expected.

## Comparison with `REACTIONS_IMPLEMENTATION_PLAN.md`

This plan is largely in agreement with the existing `REACTIONS_IMPLEMENTATION_PLAN.md`. The main differences are:

*   The addition of the `conversations_get_reactions` tool, which provides a more complete solution for managing reactions.
*   More detailed implementation steps, including the exact code for tool registration.
*   This plan is based on a more recent analysis of the codebase.
