# Reactions Implementation Plan

## Overview
Add reaction management capabilities to the Slack MCP server, allowing users to add/remove reactions via MCP tools.

## Architecture

### Current Structure
- **Handler**: `ConversationsHandler` manages conversation tools
- **API Provider**: `provider.ApiProvider` wraps Slack API calls
- **Tool Registration**: Tools registered in `pkg/server/server.go`

### New Tools to Add
1. `conversations_add_reaction` - Add emoji reactions to messages
2. `conversations_remove_reaction` - Remove emoji reactions from messages

## Implementation Steps

### 1. Extend API Provider
```go
// Add to SlackAPI interface in pkg/provider/api.go
AddReactionContext(ctx context.Context, name string, item slack.ItemRef) error
RemoveReactionContext(ctx context.Context, name string, item slack.ItemRef) error

// Implement in MCPSlackClient
func (c *MCPSlackClient) AddReactionContext(ctx context.Context, name string, item slack.ItemRef) error {
    return c.slackClient.AddReactionContext(ctx, name, item)
}
```

### 2. Add Handler Methods
```go
// In pkg/handler/conversations.go
func (ch *ConversationsHandler) ConversationsAddReactionHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
func (ch *ConversationsHandler) ConversationsRemoveReactionHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
```

### 3. Register Tools
```go
// In pkg/server/server.go
s.AddTool(mcp.NewTool("conversations_add_reaction", ...), conversationsHandler.ConversationsAddReactionHandler)
s.AddTool(mcp.NewTool("conversations_remove_reaction", ...), conversationsHandler.ConversationsRemoveReactionHandler)
```

## Tool Parameters

### `conversations_add_reaction`
- `channel_id` (required): Channel ID or name
- `timestamp` (required): Message timestamp
- `reaction_name` (required): Emoji name (e.g., "rocket", "heart")
- `thread_ts` (optional): Thread timestamp if applicable

### `conversations_remove_reaction`
- Same parameters as add

## Configuration
```bash
# Environment variable control
SLACK_MCP_ADD_REACTION_TOOL=true  # Enable for all channels
SLACK_MCP_ADD_REACTION_TOOL=C1234567890,D0987654321  # Limit to specific channels
```

## Security & Controls
- Rate limiting via existing infrastructure
- Channel restrictions (similar to message posting)
- Permission validation (`reactions:write` scope required)
- Audit logging for all operations

## Return Format
- **Success**: CSV with updated message including new reactions
- **Error**: Clear error message with failure details

## Testing
- Unit tests for parameter parsing
- Integration tests with Slack API
- Error handling validation

## Dependencies
- Slack API `reactions:write` scope (pending approval)
- Existing rate limiting and auth infrastructure
- Current Slack Go libraries already support reactions

## Benefits
✅ Consistent with existing patterns  
✅ Reuses existing infrastructure  
✅ Easy to maintain and extend  
✅ Follows MCP standards  
✅ Configurable enable/disable
