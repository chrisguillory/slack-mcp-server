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
- **OAuth Bot Mode (xoxp-)**: Uses official Slack API with scope-based permissions
- **Browser Session Mode (xoxc/xoxd)**: Uses web client APIs with user session permissions
- **Feature flagging needed** for browser sessions (same pattern as message posting)

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

```go
type SlackAPI interface {
    // ... existing methods ...
    AddReactionContext(ctx context.Context, name string, item slack.ItemRef) error
    RemoveReactionContext(ctx context.Context, name string, item slack.ItemRef) error
}
```

**File: `pkg/provider/mcp_slack_client.go`**

```go
func (c *MCPSlackClient) AddReactionContext(ctx context.Context, name string, item slack.ItemRef) error {
    // Follow existing pattern: use edge client for browser sessions, official API for OAuth
    if c.isEnterprise {
        if c.isOAuth {
            // Use official Slack API for OAuth bot tokens
            return c.slackClient.AddReactionContext(ctx, name, item)
        } else {
            // Use edge client for browser session tokens
            return c.edgeClient.AddReactionContext(ctx, name, item)
        }
    }
    
    // Non-enterprise: always use official API (same pattern as GetConversationsContext)
    return c.slackClient.AddReactionContext(ctx, name, item)
}

func (c *MCPSlackClient) RemoveReactionContext(ctx context.Context, name string, item slack.ItemRef) error {
    // Follow existing pattern: use edge client for browser sessions, official API for OAuth
    if c.isEnterprise {
        if c.isOAuth {
            // Use official Slack API for OAuth bot tokens
            return c.slackClient.RemoveReactionContext(ctx, name, item)
        } else {
            // Use edge client for browser session tokens
            return c.edgeClient.RemoveReactionContext(ctx, name, item)
        }
    }
    
    // Non-enterprise: always use official API (same pattern as GetConversationsContext)
    return c.slackClient.RemoveReactionContext(ctx, name, item)
}


```

**Note**: This follows the exact same routing pattern as the existing `GetConversationsContext` method: enterprise + browser session tokens use edge client, enterprise + OAuth tokens use official API, non-enterprise always uses official API.

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
    // Use same pattern as existing isChannelAllowed function
    config := os.Getenv("SLACK_MCP_REACTION_TOOLS")
    if config == "" || config == "true" || config == "1" {
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
    return !isNegated
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
        return nil, fmt.Errorf("reaction tools are disabled for this channel. Set SLACK_MCP_REACTION_TOOLS environment variable to enable.")
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
        return nil, fmt.Errorf("reaction tools are disabled for this channel. Set SLACK_MCP_REACTION_TOOLS environment variable to enable.")
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

### 5. Add Tests

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
# Browser session mode (xoxc/xoxd) - REQUIRES feature flag
export SLACK_MCP_REACTION_TOOLS=true  # Enable for all channels
export SLACK_MCP_REACTION_TOOLS=C123,D456  # Enable only for specific channels
export SLACK_MCP_REACTION_TOOLS=!C123  # Enable for all except specific channels

# OAuth bot mode (xoxp) - NO feature flag needed
# Slack API handles permissions automatically
```

### Feature Flag Behavior

- **OAuth Bot Mode**: No feature flag needed - Slack API enforces permissions
- **Browser Session Mode**: Requires explicit enablement via `SLACK_MCP_REACTION_TOOLS`
- **Same configuration pattern** as `SLACK_MCP_ADD_MESSAGE_TOOL`

## Security Considerations

### OAuth Bot Mode (xoxp- tokens)
- ✅ **Slack API enforces permissions** via OAuth scopes
- ✅ **Bot has specific, limited permissions** (`reactions:write` scope required)
- ✅ **Workspace admins control** bot access
- ✅ **Audit trail** of bot actions
- ✅ **No feature flag needed** - Slack handles security

### Browser Session Mode (xoxc/xoxd tokens)
- ✅ **Uses edge client** for enterprise workspaces
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
- ✅ Works with OAuth bot tokens (xoxp-) via official Slack API
- ✅ Works with browser session tokens (xoxc/xoxd) via edge client (enterprise only)
- ✅ Feature flag respects channel restrictions for browser sessions
- ✅ OAuth bots work without feature flag configuration
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
The implementation follows the exact same routing pattern as existing methods like `GetConversationsContext`:

- **Enterprise + OAuth tokens (xoxp-)**: Uses official Slack API
- **Enterprise + Browser session tokens (xoxc/xoxd)**: Uses edge client with web client APIs
- **Non-enterprise**: Always uses official Slack API (regardless of token type)

### Feature Flag Usage
- **All auth types**: Feature flag required (same pattern as `SLACK_MCP_ADD_MESSAGE_TOOL`)
- **Simple approach**: No auth type differentiation needed - the API provider handles routing automatically

### Browser API Integration
The edge client implementation uses the discovered web client APIs:
- `reactions.add` endpoint for adding reactions
- `reactions.remove` endpoint for removing reactions
- Same parameter structure as official Slack API

---

This plan provides a robust, secure, and maintainable implementation that leverages the existing routing infrastructure while delivering working reaction functionality for both authentication types.
