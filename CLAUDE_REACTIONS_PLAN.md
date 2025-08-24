# Claude's Reactions Implementation Plan

## Overview
Implementation approach for adding reaction management capabilities to the Slack MCP server, allowing users to add and remove reactions via MCP tools.

## Architecture Analysis

### Existing Patterns to Follow
- **Handler Pattern**: `ConversationsHandler` with dependency injection (apiProvider, logger)
- **Tool Registration**: Descriptive parameter definitions in `server.go`
- **Response Format**: Consistent CSV output using `gocsv.MarshalBytes()`
- **Error Handling**: Structured logging with Zap fields
- **Configuration**: Environment variable control for feature flags

### Current Reaction Support
- Messages already include reactions in format: `emoji:count:user1,user2`
- Recent enhancement added user IDs to reaction format (commit 012f4bf)
- Missing: API methods and MCP tools for managing reactions

## Implementation Approach

### 1. API Provider Layer

```go
// Extend SlackAPI interface in pkg/provider/api.go
AddReactionContext(ctx context.Context, emoji string, channel, timestamp string) error
RemoveReactionContext(ctx context.Context, emoji string, channel, timestamp string) error

// Implementation in MCPSlackClient
func (c *MCPSlackClient) AddReactionContext(ctx context.Context, emoji string, channel, timestamp string) error {
    itemRef := slack.NewRefToMessage(channel, timestamp)
    return c.slackClient.AddReactionContext(ctx, emoji, itemRef)
}
```

**Design Decision**: Pass channel and timestamp directly for consistency with other methods rather than using `slack.ItemRef`.

### 2. Handler Implementation

```go
// In pkg/handler/conversations.go

type ReactionParams struct {
    ChannelID string `json:"channel_id"` // Required
    Timestamp string `json:"timestamp"`  // Required  
    Emoji     string `json:"emoji"`      // Required (without colons)
}

func (ch *ConversationsHandler) ConversationsAddReactionHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    // 1. Parse and validate parameters
    // 2. Strip colons from emoji name
    // 3. Check channel membership
    // 4. Call API provider
    // 5. Fetch updated message
    // 6. Return CSV with confirmation and updated message
}
```

### 3. Enhanced Error Handling

- **Idempotent Operations**: Adding existing reaction returns success
- **Specific Error Types**:
  - `already_reacted`: When reaction already exists (still return success)
  - `no_reaction`: When removing non-existent reaction
  - `invalid_emoji`: When emoji format is invalid
  - `not_in_channel`: When bot lacks channel access
  - `message_not_found`: When timestamp doesn't exist

### 4. Parameter Structure

```go
// Simplified, focused parameters
conversations_add_reaction:
  - channel_id (required): Channel ID or name
  - timestamp (required): Message timestamp
  - emoji (required): Emoji name without colons

conversations_remove_reaction:
  - channel_id (required): Channel ID or name
  - timestamp (required): Message timestamp
  - emoji (required): Emoji name without colons
```

**Note**: Removed `thread_ts` as redundant - reactions target specific messages by timestamp.

### 5. Response Format

```csv
action,channel_id,timestamp,emoji,success,msgID,userID,userName,realName,text,reactions
added,C123,1234567890.123,rocket,true,1234567890.123,U456,alice,Alice Smith,"Hello world","rocket:1:U456|thumbsup:2:U789,U012"
```

Enhanced response includes:
- **Action confirmation**: What was done (added/removed)
- **Success status**: Operation result
- **Full message data**: Complete updated message information

### 6. Validation & Security

```go
func validateEmoji(emoji string) (string, error) {
    // Strip colons if present
    emoji = strings.Trim(emoji, ":")
    
    // Validate against known emoji set or pattern
    if !isValidEmoji(emoji) {
        return "", fmt.Errorf("invalid emoji: %s", emoji)
    }
    
    return emoji, nil
}

func (ch *ConversationsHandler) checkChannelAccess(ctx context.Context, channelID string) error {
    // Verify bot is member of channel
    // Cache membership status for performance
}
```

### 7. Configuration

```bash
# Environment variable control
SLACK_MCP_REACTION_TOOLS=true  # Enable globally
SLACK_MCP_REACTION_TOOLS=C1234567890,D0987654321  # Channel-specific

# Rate limiting
SLACK_MCP_REACTION_RATE_LIMIT=10  # Max reactions per minute per channel
```

### 8. Tool Registration

```go
// In pkg/server/server.go
s.AddTool(mcp.NewTool("conversations_add_reaction",
    mcp.WithDescription("Add an emoji reaction to a message"),
    mcp.WithString("channel_id", mcp.Required(), mcp.Description("Channel ID or name")),
    mcp.WithString("timestamp", mcp.Required(), mcp.Description("Message timestamp")),
    mcp.WithString("emoji", mcp.Required(), mcp.Description("Emoji name without colons")),
), conversationsHandler.ConversationsAddReactionHandler)
```

## Testing Strategy

### Unit Tests
```go
// pkg/handler/conversations_test.go
func TestConversationsAddReactionHandler(t *testing.T) {
    // Test parameter validation
    // Test emoji normalization
    // Test error handling
    // Test idempotent behavior
}
```

### Integration Tests
```go
// pkg/integration/reactions_test.go
func TestReactionWorkflow(t *testing.T) {
    // 1. Post test message
    // 2. Add reaction
    // 3. Verify reaction appears
    // 4. Remove reaction
    // 5. Verify reaction removed
}
```

### Edge Cases to Test
- Unicode emoji (👍)
- Custom workspace emoji
- Emoji with skin tone modifiers
- Thread message reactions
- Deleted message handling
- Rate limit behavior

## Implementation Order

1. **Phase 1: Core Implementation**
   - API provider methods
   - Basic handler functions
   - Parameter parsing

2. **Phase 2: Robustness**
   - Validation layer
   - Error handling
   - Channel access checks

3. **Phase 3: Polish**
   - Rate limiting
   - Response enrichment
   - Comprehensive tests

4. **Phase 4: Documentation**
   - API documentation
   - Usage examples
   - Configuration guide

## Key Improvements Over Initial Plan

1. **Simplified Parameters**: Removed redundant `thread_ts` parameter
2. **Idempotent Operations**: Adding existing reactions succeeds silently
3. **Enhanced Validation**: Emoji format validation and normalization
4. **Richer Responses**: Include action confirmation and full message state
5. **Channel Access Checks**: Verify bot membership before operations
6. **Comprehensive Testing**: Unit, integration, and edge case coverage

## Security Considerations

- **Scope Requirements**: `reactions:write` scope required
- **Channel Restrictions**: Configurable per-channel enablement
- **Rate Limiting**: Per-channel rate limits to prevent abuse
- **Audit Logging**: All reaction operations logged with context
- **Permission Validation**: Check bot can access channel and message

## Future Enhancements

### Batch Operations
- `conversations_bulk_add_reactions`: Multiple reactions at once
- `conversations_bulk_remove_reactions`: Remove multiple reactions

### Query Capabilities  
- Filter history by reactions
- Search messages with specific reactions
- Get reaction analytics

### Rich Responses
- Include reactor details in response
- Return reaction timestamps
- Support reaction queries by user

## Success Metrics

- Zero regression in existing functionality
- Sub-100ms response time for reaction operations
- 100% backward compatibility
- Complete test coverage for new code
- Clear error messages for all failure cases

## Dependencies

- Slack Go SDK already supports `AddReaction`/`RemoveReaction`
- Existing rate limiting infrastructure
- Current authentication/authorization middleware
- CSV marshaling libraries in use

This plan maintains architectural consistency while adding robust reaction management capabilities to the Slack MCP server.