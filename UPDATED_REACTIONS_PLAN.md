# Updated Slack MCP Reactions Implementation Plan

## Overview
Comprehensive implementation plan for adding reaction management capabilities to the Slack MCP server, supporting both OAuth bot tokens and browser session tokens. This plan addresses the security considerations, architectural patterns, and browser API capabilities discovered through investigation.

## Key Discoveries

### 1. Browser API Support Confirmed
- **Reactions ARE supported** via web client APIs (`{workspace}.slack.com/api/`)
- **Endpoints available**: `reactions.add` and `reactions.remove`
- **Simple form-based requests** with standard response format
- **Same core parameters** as official Slack API

### 2. Authentication Type Considerations
- **OAuth User Token (xoxp-)**: Uses official Slack API with scope-based permissions
- **Browser Session Token (xoxc/xoxd)**: Uses web client APIs with user session permissions
- **Feature flagging needed** for safety (same pattern as message posting)

## Architecture

### Current Structure
- **Handler**: `ConversationsHandler` manages conversation tools
- **API Provider**: `provider.ApiProvider` wraps Slack API calls
- **Edge Client**: `pkg/provider/edge` handles browser session APIs
- **Tool Registration**: Tools registered in `pkg/server/server.go`

### New Tools to Add
1. `conversations_add_reaction` - Add emoji reactions to messages
2. `conversations_remove_reaction` - Remove emoji reactions from messages

## Implementation Steps

### 1. Extend Edge Client with Reaction Methods

**File: `pkg/provider/edge/reactions.go`**

```go
package edge

import (
    "context"
    "runtime/trace"
    "github.com/slack-go/slack"
)

// conversationsAddReactionForm is the request to reactions.add
type conversationsAddReactionForm struct {
    BaseRequest
    Channel string `json:"channel"`
    Timestamp string `json:"timestamp"`
    Name string `json:"name"`
    WebClientFields
}

type conversationsAddReactionResponse struct {
    baseResponse
}

// conversationsRemoveReactionForm is the request to reactions.remove
type conversationsRemoveReactionForm struct {
    BaseRequest
    Channel string `json:"channel"`
    Timestamp string `json:"timestamp"`
    Name string `json:"name"`
    WebClientFields
}

type conversationsRemoveReactionResponse struct {
    baseResponse
}

func (cl *Client) AddReactionContext(ctx context.Context, name string, item slack.ItemRef) error {
    ctx, task := trace.NewTask(ctx, "AddReactionContext")
    defer task.End()
    trace.Logf(ctx, "params", "channel=%v, timestamp=%v, name=%v", item.Channel, item.Timestamp, name)

    form := conversationsAddReactionForm{
        BaseRequest: BaseRequest{
            Token: cl.token,
        },
        Channel:   item.Channel,
        Timestamp: item.Timestamp,
        Name:      name,
        WebClientFields: webclientReason("changeReactionFromUserAction"),
    }
    
    resp, err := cl.PostForm(ctx, "reactions.add", values(form, true))
    if err != nil {
        return err
    }
    
    var r conversationsAddReactionResponse
    return cl.ParseResponse(&r, resp)
}

func (cl *Client) RemoveReactionContext(ctx context.Context, name string, item slack.ItemRef) error {
    ctx, task := trace.NewTask(ctx, "RemoveReactionContext")
    defer task.End()
    trace.Logf(ctx, "params", "channel=%v, timestamp=%v, name=%v", item.Channel, item.Timestamp, name)

    form := conversationsRemoveReactionForm{
        BaseRequest: BaseRequest{
            Token: cl.token,
        },
        Channel:   item.Channel,
        Timestamp: item.Timestamp,
        Name:      name,
        WebClientFields: webclientReason("changeReactionFromUserAction"),
    }
    
    resp, err := cl.PostForm(ctx, "reactions.remove", values(form, true))
    if err != nil {
        return err
    }
    
    var r conversationsRemoveReactionResponse
    return cl.ParseResponse(&r, resp)
}
```

### 2. Extend API Provider Interface

**File: `pkg/provider/api.go`**

Add to the SlackAPI interface (around line 52-71):
```go
type SlackAPI interface {
    // ... existing methods ...
    AddReactionContext(ctx context.Context, name string, item slack.ItemRef) error
    RemoveReactionContext(ctx context.Context, name string, item slack.ItemRef) error
}
```

Add the implementation methods to MCPSlackClient in the same file (after line 256):
```go
func (c *MCPSlackClient) AddReactionContext(ctx context.Context, name string, item slack.ItemRef) error {
    // Route by token type, not enterprise status
    // The official Slack reactions API doesn't work with browser session tokens (xoxc/xoxd)
    if c.isOAuth {
        // OAuth user tokens (xoxp-) use official Slack API
        return c.slackClient.AddReactionContext(ctx, name, item)
    } else {
        // Browser session tokens (xoxc/xoxd) must use edge client
        return c.edgeClient.AddReactionContext(ctx, name, item)
    }
}

func (c *MCPSlackClient) RemoveReactionContext(ctx context.Context, name string, item slack.ItemRef) error {
    // Route by token type, not enterprise status
    // The official Slack reactions API doesn't work with browser session tokens (xoxc/xoxd)
    if c.isOAuth {
        // OAuth user tokens (xoxp-) use official Slack API
        return c.slackClient.RemoveReactionContext(ctx, name, item)
    } else {
        // Browser session tokens (xoxc/xoxd) must use edge client
        return c.edgeClient.RemoveReactionContext(ctx, name, item)
    }
}


```

**Note**: Add these methods directly to the existing `MCPSlackClient` struct in `pkg/provider/api.go` (not in a separate file). Unlike `GetConversationsContext` which has special handling for `conversations.list`, reactions APIs require routing purely by token type since the official Slack reactions API doesn't support browser session tokens.

### 3. Add Handler Methods

**File: `pkg/handler/conversations.go`**

```go
// Add reaction parameter struct
type reactionParams struct {
    channelID string
    timestamp string
    emoji     string
}

// Parse reaction parameters
func (ch *ConversationsHandler) parseReactionParams(req mcp.CallToolRequest) (*reactionParams, error) {
    channelID := req.GetString("channel_id", "")
    if channelID == "" {
        ch.logger.Error("channel_id missing in add-reaction params")
        return nil, errors.New("channel_id must be a string")
    }
    
    // Handle channel name resolution (same pattern as add message)
    if strings.HasPrefix(channelID, "#") || strings.HasPrefix(channelID, "@") {
        channelsMaps := ch.apiProvider.ProvideChannelsMaps()
        chn, ok := channelsMaps.ChannelsInv[channelID]
        if !ok {
            ch.logger.Error("Channel not found", zap.String("channel", channelID))
            return nil, fmt.Errorf("channel %q not found", channelID)
        }
        channelID = channelsMaps.Channels[chn].ID
    }
    
    timestamp := req.GetString("timestamp", "")
    if timestamp == "" {
        ch.logger.Error("timestamp missing in add-reaction params")
        return nil, errors.New("timestamp must be a string")
    }
    
    // Validate timestamp format (must contain a dot, like 1234567890.123456)
    if !strings.Contains(timestamp, ".") {
        ch.logger.Error("invalid timestamp format", zap.String("timestamp", timestamp))
        return nil, fmt.Errorf("invalid timestamp format: %s (must be like 1234567890.123456)", timestamp)
    }
    
    emoji := req.GetString("emoji", "")
    if emoji == "" {
        ch.logger.Error("emoji missing in add-reaction params")
        return nil, errors.New("emoji must be a string")
    }
    
    // Strip colons if present
    emoji = strings.Trim(emoji, ":")
    
    return &reactionParams{
        channelID: channelID,
        timestamp: timestamp,
        emoji:     emoji,
    }, nil
}

// Check if reactions are allowed for a channel
func (ch *ConversationsHandler) isReactionAllowed(channelID string) bool {
    config := os.Getenv("SLACK_MCP_ADD_REACTION_TOOL")
    // Default to disabled for safety (different from isChannelAllowed)
    if config == "" {
        return false
    }
    
    // Explicitly enabled for all channels
    if config == "true" || config == "1" {
        return true
    }
    
    items := strings.Split(config, ",")
    isNegated := strings.HasPrefix(strings.TrimSpace(items[0]), "!")
    
    for _, item := range items {
        item = strings.TrimSpace(item)
        if isNegated {
            if strings.TrimPrefix(item, "!") == channelID {
                return false
            }
        } else {
            if item == channelID {
                return true
            }
        }
    }
    return isNegated // If negated list, allow by default; if allowlist, deny by default
}

// Add reaction handler
func (ch *ConversationsHandler) ConversationsAddReactionHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    ch.logger.Debug("ConversationsAddReactionHandler called", zap.Any("params", request.Params))

    params, err := ch.parseReactionParams(request)
    if err != nil {
        ch.logger.Error("Failed to parse add-reaction params", zap.Error(err))
        return nil, err
    }

    // Check if reactions are enabled
    if !ch.isReactionAllowed(params.channelID) {
        return nil, fmt.Errorf("reaction tools are disabled for this channel. Set SLACK_MCP_ADD_REACTION_TOOL environment variable to enable.")
    }

    // Create Slack item reference
    item := slack.NewRefToMessage(params.channelID, params.timestamp)

    ch.logger.Debug("Adding Slack reaction",
        zap.String("channel", params.channelID),
        zap.String("timestamp", params.timestamp),
        zap.String("emoji", params.emoji),
    )

    // Add reaction (works with both auth types)
    if err := ch.apiProvider.Slack().AddReactionContext(ctx, params.emoji, item); err != nil {
        if !strings.Contains(err.Error(), "already_reacted") {
            ch.logger.Error("Slack AddReactionContext failed", zap.Error(err))
            return nil, err
        }
        // Log but continue if already reacted
        ch.logger.Debug("Reaction already exists", 
            zap.String("emoji", params.emoji),
            zap.String("channel", params.channelID),
            zap.String("timestamp", params.timestamp))
    }

    // Fetch updated message to return (same pattern as add message)
    historyParams := slack.GetConversationHistoryParameters{
        ChannelID: params.channelID,
        Limit:     1,
        Oldest:    params.timestamp,
        Latest:    params.timestamp,
        Inclusive: true,
    }

    history, err := ch.apiProvider.Slack().GetConversationHistoryContext(ctx, &historyParams)
    if err != nil {
        ch.logger.Error("GetConversationHistoryContext failed", zap.Error(err))
        return nil, err
    }
    ch.logger.Debug("Fetched conversation history", zap.Int("message_count", len(history.Messages)))

    if len(history.Messages) == 0 {
        return nil, fmt.Errorf("message not found after adding reaction")
    }

    // Convert and return as CSV (same pattern as add message)
    messages := ch.convertMessagesFromHistory(history.Messages, params.channelID, false)
    return marshalMessagesToCSV(messages)
}

// Remove reaction handler
func (ch *ConversationsHandler) ConversationsRemoveReactionHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    ch.logger.Debug("ConversationsRemoveReactionHandler called", zap.Any("params", request.Params))

    params, err := ch.parseReactionParams(request)
    if err != nil {
        ch.logger.Error("Failed to parse remove-reaction params", zap.Error(err))
        return nil, err
    }

    // Check if reactions are enabled
    if !ch.isReactionAllowed(params.channelID) {
        return nil, fmt.Errorf("reaction tools are disabled for this channel. Set SLACK_MCP_ADD_REACTION_TOOL environment variable to enable.")
    }

    // Create Slack item reference
    item := slack.NewRefToMessage(params.channelID, params.timestamp)

    ch.logger.Debug("Removing Slack reaction",
        zap.String("channel", params.channelID),
        zap.String("timestamp", params.timestamp),
        zap.String("emoji", params.emoji),
    )

    // Remove reaction (works with both auth types)
    if err := ch.apiProvider.Slack().RemoveReactionContext(ctx, params.emoji, item); err != nil {
        if !strings.Contains(err.Error(), "no_reaction") {
            ch.logger.Error("Slack RemoveReactionContext failed", zap.Error(err))
            return nil, err
        }
        // Log but continue if no reaction exists
        ch.logger.Debug("Reaction doesn't exist", 
            zap.String("emoji", params.emoji),
            zap.String("channel", params.channelID),
            zap.String("timestamp", params.timestamp))
    }

    // Fetch updated message to return (same pattern as add message)
    historyParams := slack.GetConversationHistoryParameters{
        ChannelID: params.channelID,
        Limit:     1,
        Oldest:    params.timestamp,
        Latest:    params.timestamp,
        Inclusive: true,
    }

    history, err := ch.apiProvider.Slack().GetConversationHistoryContext(ctx, &historyParams)
    if err != nil {
        ch.logger.Error("GetConversationHistoryContext failed", zap.Error(err))
        return nil, err
    }
    ch.logger.Debug("Fetched conversation history", zap.Int("message_count", len(history.Messages)))

    if len(history.Messages) == 0 {
        return nil, fmt.Errorf("message not found after removing reaction")
    }

    // Convert and return as CSV (same pattern as add message)
    messages := ch.convertMessagesFromHistory(history.Messages, params.channelID, false)
    return marshalMessagesToCSV(messages)
}
```

### 4. Register Tools

**File: `pkg/server/server.go`**

```go
// Add in NewMCPServer function, after other conversation tools

// Add reaction tool
s.AddTool(mcp.NewTool("conversations_add_reaction",
    mcp.WithDescription("Add an emoji reaction to a message"),
    mcp.WithString("channel_id", 
        mcp.Required(), 
        mcp.Description("Channel ID (C...) or name (#general, @user_dm)")),
    mcp.WithString("timestamp", 
        mcp.Required(), 
        mcp.Description("Message timestamp (e.g., 1234567890.123456)")),
    mcp.WithString("emoji", 
        mcp.Required(), 
        mcp.Description("Emoji name without colons (e.g., thumbsup, rocket)")),
), conversationsHandler.ConversationsAddReactionHandler)

// Remove reaction tool
s.AddTool(mcp.NewTool("conversations_remove_reaction",
    mcp.WithDescription("Remove an emoji reaction from a message"),
    mcp.WithString("channel_id", 
        mcp.Required(), 
        mcp.Description("Channel ID (C...) or name (#general, @user_dm)")),
    mcp.WithString("timestamp", 
        mcp.Required(), 
        mcp.Description("Message timestamp (e.g., 1234567890.123456)")),
    mcp.WithString("emoji", 
        mcp.Required(), 
        mcp.Description("Emoji name without colons (e.g., thumbsup, rocket)")),
), conversationsHandler.ConversationsRemoveReactionHandler)
```

### 5. Add Config Validation

**File: `cmd/slack-mcp-server/main.go`**

Add after the existing `SLACK_MCP_ADD_MESSAGE_TOOL` validation (around line 40):
```go
// Validate reaction tools configuration
err = validateToolConfig(os.Getenv("SLACK_MCP_ADD_REACTION_TOOL"))
if err != nil {
    logger.Fatal("error in SLACK_MCP_ADD_REACTION_TOOL",
        zap.String("context", "console"),
        zap.Error(err),
    )
}
```

This reuses the existing `validateToolConfig` function that checks for invalid mixed allow/deny configurations.

### 6. Add Tests

**File: `pkg/handler/conversations_test.go`**

```go
// Add unit tests for parameter parsing
func TestUnitParseReactionParams(t *testing.T) {
    handler := &ConversationsHandler{}
    
    tests := []struct {
        name      string
        request   mcp.CallToolRequest
        wantErr   bool
        wantEmoji string
    }{
        {
            name: "valid params",
            request: mcp.CallToolRequest{
                Params: map[string]interface{}{
                    "channel_id": "C1234567890",
                    "timestamp":  "1234567890.123456",
                    "emoji":      "thumbsup",
                },
            },
            wantErr:   false,
            wantEmoji: "thumbsup",
        },
        {
            name: "emoji with colons",
            request: mcp.CallToolRequest{
                Params: map[string]interface{}{
                    "channel_id": "C1234567890",
                    "timestamp":  "1234567890.123456",
                    "emoji":      ":rocket:",
                },
            },
            wantErr:   false,
            wantEmoji: "rocket",
        },
        {
            name: "missing channel_id",
            request: mcp.CallToolRequest{
                Params: map[string]interface{}{
                    "timestamp": "1234567890.123456",
                    "emoji":     "thumbsup",
                },
            },
            wantErr: true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            params, err := handler.parseReactionParams(tt.request)
            if (err != nil) != tt.wantErr {
                t.Errorf("parseReactionParams() error = %v, wantErr %v", err, err)
                return
            }
            if !tt.wantErr && params.emoji != tt.wantEmoji {
                t.Errorf("parseReactionParams() emoji = %v, want %v", params.emoji, tt.wantEmoji)
            }
        })
    }
}
```

## Configuration

### Environment Variables

```bash
# ALL authentication types require the feature flag for safety
export SLACK_MCP_ADD_REACTION_TOOL=true  # Enable for all channels
export SLACK_MCP_ADD_REACTION_TOOL=C123,D456  # Enable only for specific channels
export SLACK_MCP_ADD_REACTION_TOOL=!C123  # Enable for all except specific channels

# Default (when not set): Reactions are DISABLED
```

### Feature Flag Behavior

- **ALL auth types require the feature flag** - This provides consistent safety controls
- **OAuth User Token (xoxp-)**: Feature flag required + Slack API enforces permissions
- **Browser Session Token (xoxc/xoxd)**: Feature flag required for user safety
- **Same configuration pattern** as `SLACK_MCP_ADD_MESSAGE_TOOL` for consistency
- **Default behavior**: Reactions are DISABLED when flag is not set (safe by default)

## Security Considerations

### OAuth User Token Mode (xoxp- tokens)
- ✅ **Slack API enforces permissions** via OAuth scopes
- ✅ **User has specific permissions** (`reactions:write` scope required)
- ✅ **Workspace admins control** user OAuth app access
- ✅ **Audit trail** of actions
- ✅ **Feature flag still required** for additional safety

### Browser Session Token Mode (xoxc/xoxd tokens)
- ✅ **Uses edge client** for all cases (not just enterprise)
- ✅ **Uses web client APIs** (reactions.add, reactions.remove)
- ✅ **Actions appear as the user** in Slack
- ✅ **Feature flag required** - same pattern as message posting
- ✅ **Channel-based restrictions** available

## Response Format

Both tools return the same CSV schema used by other conversation tools:

```csv
msgID,userID,userUser,realName,channelID,ThreadTs,text,time,reactions,cursor
1755999643.467839,U456,alice,Alice Smith,C09BQKT77G8,,"Hello world","2025-01-24T19:55:12Z","tada:1:U456|thumbsup:2:U789,U012",
```

The `reactions` column shows the updated reaction state after the operation.

## Testing Strategy

### Unit Tests
- Parameter validation and emoji normalization
- Idempotent behavior handling
- Feature flag logic for different auth types
- CSV rendering with updated reactions

### Testing Infrastructure
- **Unit tests**: Use `make test` (runs tests with "Unit" in name)
- **Integration tests**: Use `make test-integration` (runs tests with "Integration" in name)
- **Naming convention**: Include "Unit" or "Integration" in test names to match Makefile filters
- **Existing pattern**: Follow the same structure as existing tests in the codebase

### Test Examples
- **Unit tests**: Test parameter parsing, validation, and business logic
- **Integration tests**: Test actual Slack API calls (requires valid credentials)
- **No new test types**: Use only the testing patterns already established in the repo

## Implementation Order

1. **Phase 1: Edge Client Extension**
   - Add reaction methods to edge client
   - Test browser API integration

2. **Phase 2: API Provider Integration**
   - Extend SlackAPI interface
   - Implement auth-type-aware routing

3. **Phase 3: Handler Implementation**
   - Add reaction handlers
   - Implement feature flag logic

4. **Phase 4: Tool Registration**
   - Register tools in server
   - Add parameter validation

5. **Phase 5: Testing & Documentation**
   - Unit tests following existing patterns
   - Update documentation
   - Configuration examples

## Success Criteria

- ✅ Can add reactions to messages in channels, DMs, and threads
- ✅ Can remove reactions from messages
- ✅ Idempotent operations (no errors on duplicate add/remove)
- ✅ Returns updated message with current reactions in CSV format
- ✅ Works with OAuth user tokens (xoxp-) via official Slack API
- ✅ Works with browser session tokens (xoxc/xoxd) via edge client
- ✅ Feature flag respects channel restrictions for all token types
- ✅ Feature flag required for all authentication types (safe by default)
- ✅ Handles errors gracefully with clear messages
- ✅ Unit tests pass
- ✅ No regression in existing functionality

## Dependencies

- Slack Go SDK already supports `AddReaction`/`RemoveReaction`
- Existing edge client infrastructure
- Current authentication/authorization middleware
- CSV marshaling libraries in use
- Feature flag system (same as message posting)

## Future Enhancements

### Query Capabilities
- `conversations_get_reactions`: Retrieve all reactions for a message
- Filter history by reactions
- Search messages with specific reactions

### Batch Operations
- `conversations_bulk_add_reactions`: Multiple reactions at once
- `conversations_bulk_remove_reactions`: Remove multiple reactions

### Rich Responses
- Include reactor details in response
- Return reaction timestamps
- Support reaction queries by user

## Implementation Notes

### Routing Behavior
The implementation uses token-type-based routing (different from `GetConversationsContext`):

- **OAuth user tokens (xoxp-)**: Always use official Slack API
- **Browser session tokens (xoxc/xoxd)**: Always use edge client with web client APIs
- **No enterprise distinction**: Route purely by token type for reactions API

### Feature Flag Usage
- **All auth types**: Feature flag required (same pattern as `SLACK_MCP_ADD_MESSAGE_TOOL`)
- **Simple approach**: No auth type differentiation needed - the API provider handles routing automatically

### Browser API Integration
The edge client implementation uses the discovered web client APIs:
- `reactions.add` endpoint for adding reactions
- `reactions.remove` for removing reactions
- Same parameter structure as official Slack API

---

This plan provides a robust, secure, and maintainable implementation that leverages the existing routing infrastructure while delivering working reaction functionality for both authentication types.