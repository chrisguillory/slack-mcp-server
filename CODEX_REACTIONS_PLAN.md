# CODEX Reactions Implementation Plan

## Overview
- Add reaction management to the Slack MCP server with two MCP tools: `conversations_add_reaction` and `conversations_remove_reaction`.
- Follow existing handler/provider patterns and return an updated message as CSV (including the `reactions` field) after each operation.

## Key Differences vs Existing Plan
- Parameters: use `emoji` (no colons) instead of `reaction_name`; omit `thread_ts` (message `timestamp` uniquely identifies the target in a channel/thread).
- Idempotency: treat `already_reacted` and `no_reaction` as successful no-ops and still return the updated message.
- Config gate: single feature flag `SLACK_MCP_REACTION_TOOL` for both add/remove tools with allow/deny list semantics mirroring message-posting.
- Response enrichment: include an `action` column in the CSV header for add/remove confirmation while keeping message schema unchanged otherwise.

## Architecture Alignment
- Handlers live in `pkg/handler` (see `ConversationsAddMessageHandler` for patterns: parse params, call provider, fetch message, marshal CSV).
- Tools are registered in `pkg/server/server.go` with `mcp.NewTool(...)` and parameter descriptors.
- Slack API calls are wrapped by `pkg/provider` via the `SlackAPI` interface and `MCPSlackClient` implementation.

## Tools
1. `conversations_add_reaction`
2. `conversations_remove_reaction`

### Parameters (both tools)
- `channel_id` (required): Channel ID or name (e.g., `C123…`, `#general`, `@user_dm`).
- `timestamp` (required): Message `ts` (e.g., `1712345678.901234`).
- `emoji` (required): Emoji name without colons (e.g., `rocket`, `thumbsup`).

Rationale: `timestamp` is sufficient for both channel and thread messages; Slack reactions target the message at that `ts`.

## Provider Layer

Extend the `SlackAPI` interface and `MCPSlackClient` to expose Slack reaction operations:

```go
// pkg/provider/api.go
type SlackAPI interface {
    // ...existing methods...
    AddReactionContext(ctx context.Context, name string, item slack.ItemRef) error
    RemoveReactionContext(ctx context.Context, name string, item slack.ItemRef) error
}

// Implementation in MCPSlackClient
func (c *MCPSlackClient) AddReactionContext(ctx context.Context, name string, item slack.ItemRef) error {
    return c.slackClient.AddReactionContext(ctx, name, item)
}

func (c *MCPSlackClient) RemoveReactionContext(ctx context.Context, name string, item slack.ItemRef) error {
    return c.slackClient.RemoveReactionContext(ctx, name, item)
}
```

Notes
- Keep the `slack.ItemRef` signature to match the Slack SDK and avoid duplicating channel/ts logic across layers.

## Handlers

Add two handlers to `pkg/handler/conversations.go` following existing patterns.

```go
// pkg/handler/conversations.go
type reactionParams struct {
    channel   string
    timestamp string
    emoji     string
}

func (ch *ConversationsHandler) parseParamsToolReaction(req mcp.CallToolRequest) (*reactionParams, error) {
    channel := req.GetString("channel_id", "")
    if channel == "" { return nil, errors.New("channel_id must be a string") }
    ts := req.GetString("timestamp", "")
    if ts == "" { return nil, errors.New("timestamp must be a string") }
    emoji := strings.Trim(req.GetString("emoji", ""), ":")
    if emoji == "" { return nil, errors.New("emoji must be a string") }
    return &reactionParams{channel: channel, timestamp: ts, emoji: emoji}, nil
}

func (ch *ConversationsHandler) ConversationsAddReactionHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    params, err := ch.parseParamsToolReaction(req)
    if err != nil { return nil, err }

    // Optional channel gating (see Config section)
    if !isReactionAllowed(params.channel) {
        return nil, fmt.Errorf("reaction tools disabled for channel: %s", params.channel)
    }

    // Add reaction
    item := slack.NewRefToMessage(params.channel, params.timestamp)
    if err := ch.apiProvider.Slack().AddReactionContext(ctx, params.emoji, item); err != nil {
        // Treat idempotent error as success
        if !strings.Contains(err.Error(), "already_reacted") { return nil, err }
    }

    // Fetch updated message (same pattern as ConversationsAddMessageHandler)
    historyParams := slack.GetConversationHistoryParameters{
        ChannelID: params.channel,
        Limit:     1,
        Oldest:    params.timestamp,
        Latest:    params.timestamp,
        Inclusive: true,
    }
    history, err := ch.apiProvider.Slack().GetConversationHistoryContext(ctx, &historyParams)
    if err != nil { return nil, err }
    messages := ch.convertMessagesFromHistory(history.Messages, historyParams.ChannelID, false)
    // Optionally add an action marker in CSV (see Response section)
    return marshalMessagesToCSV(messages)
}

func (ch *ConversationsHandler) ConversationsRemoveReactionHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    params, err := ch.parseParamsToolReaction(req)
    if err != nil { return nil, err }

    if !isReactionAllowed(params.channel) {
        return nil, fmt.Errorf("reaction tools disabled for channel: %s", params.channel)
    }

    item := slack.NewRefToMessage(params.channel, params.timestamp)
    if err := ch.apiProvider.Slack().RemoveReactionContext(ctx, params.emoji, item); err != nil {
        if !strings.Contains(err.Error(), "no_reaction") { return nil, err }
    }

    historyParams := slack.GetConversationHistoryParameters{
        ChannelID: params.channel,
        Limit:     1,
        Oldest:    params.timestamp,
        Latest:    params.timestamp,
        Inclusive: true,
    }
    history, err := ch.apiProvider.Slack().GetConversationHistoryContext(ctx, &historyParams)
    if err != nil { return nil, err }
    messages := ch.convertMessagesFromHistory(history.Messages, historyParams.ChannelID, false)
    return marshalMessagesToCSV(messages)
}
```

## Tool Registration

Register tools in `pkg/server/server.go` alongside other conversation tools.

```go
// pkg/server/server.go
s.AddTool(mcp.NewTool("conversations_add_reaction",
    mcp.WithDescription("Add an emoji reaction to a message"),
    mcp.WithString("channel_id", mcp.Required(), mcp.Description("Channel ID or name (C…, #channel, @user_dm)")),
    mcp.WithString("timestamp", mcp.Required(), mcp.Description("Message timestamp (ts)")),
    mcp.WithString("emoji", mcp.Required(), mcp.Description("Emoji name without colons")),
), conversationsHandler.ConversationsAddReactionHandler)

s.AddTool(mcp.NewTool("conversations_remove_reaction",
    mcp.WithDescription("Remove an emoji reaction from a message"),
    mcp.WithString("channel_id", mcp.Required(), mcp.Description("Channel ID or name (C…, #channel, @user_dm)")),
    mcp.WithString("timestamp", mcp.Required(), mcp.Description("Message timestamp (ts)")),
    mcp.WithString("emoji", mcp.Required(), mcp.Description("Emoji name without colons")),
), conversationsHandler.ConversationsRemoveReactionHandler)
```

## Response
- Return the same CSV schema used by history/replies/add_message; the `reactions` column already exists and is formatted as `emoji:count:user1,user2|…`.
- Optional: prepend an `action` column with values `added` or `removed` to make intent explicit in single-row results. If added, keep it last to minimize disruption and document it.

Example (single-row add):

```csv
msgID,userID,userUser,realName,channelID,ThreadTs,text,time,reactions,cursor,action
1712345678.901234,U456,alice,Alice Smith,#general,,"Hello","2025-02-12T19:55:12Z","rocket:1:U456|thumbsup:2:U789,U012",,added
```

## Configuration
- `SLACK_MCP_REACTION_TOOL` controls enablement for both tools.
  - `true` / `1` / empty → enabled for all channels.
  - Comma-separated allow-list of channel IDs/names → only enabled for listed channels.
  - Negated list starting with `!` → disabled only for listed channels.

Implementation note: replicate the logic from `isChannelAllowed` or extract a helper that accepts an env key (e.g., `isChannelAllowedFor(key, channel)`).

## Security & Permissions
- OAuth tokens must have `reactions:write` scope to add/remove reactions.
- Session-based tokens (xoxc/xoxd) rely on the authenticated user session; document that workspace policies may still restrict reactions in some channels.
- Existing auth middleware and logging stay in place; handlers should log tool invocation with parameters and outcomes.

## Error Handling & Idempotency
- Normalize emoji by trimming leading/trailing `:`.
- Treat Slack errors containing `already_reacted` (add) and `no_reaction` (remove) as successful no-ops.
- Pass through other errors (e.g., `not_in_channel`, `invalid_name`, `message_not_found`).
- Always attempt to fetch and return the updated message on success/no-op.

## Rate Limiting
- Keep initial implementation simple, relying on Slack SDK backoff.
- If needed later, add a provider-level limiter wrapper for reaction calls similar to `GetConversationsContext` usage of `ap.rateLimiter`.

## Testing
- Unit tests (match repo style: include “Unit” in names)
  - Parameter validation and emoji normalization.
  - Idempotent paths: simulate `already_reacted` / `no_reaction` error strings.
  - CSV rendering contains updated `reactions` data.
- Integration tests (include “Integration” in names)
  - Post a message, add reaction, verify, remove reaction, verify (guarded by env for Slack creds).

## Documentation
- Update `README.md` and/or `docs/`:
  - Tool descriptions, parameter tables, examples.
  - `reactions` column format in returned CSV.
  - New env var `SLACK_MCP_REACTION_TOOL` semantics and examples.
  - Required Slack scopes for XOXP.

## Implementation Order
1. Provider interface + MCPSlackClient methods (Add/Remove reaction).
2. Handler param parsing + emoji normalization.
3. Add/remove handlers with fetch-updated-message path.
4. Tool registration in `server.go` with parameter descriptors.
5. Unit tests for handlers and parsing.
6. Docs and examples.
7. Optional: Action column addition in CSV and tests (if chosen).

## Acceptance Criteria
- Tools registered and callable via MCP with clear parameter docs.
- Add/remove reaction operations succeed and are idempotent.
- Returned CSV contains the updated message with correct `reactions` string.
- Feature flag behaves as documented (global enable, allow-list, negation).
- Unit tests pass with `make test`; integration tests pass with valid creds.

