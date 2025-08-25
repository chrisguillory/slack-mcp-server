# TODO: Slack MCP Server Improvements

This file tracks planned improvements, missing features, and enhancements for the Slack MCP Server repository.

## 🚨 Missing Core Functionality

### 1. Emoji List Tool
**Priority: Medium** | **Status: Not Implemented**

**Problem**: MCP clients cannot discover available emoji reactions, limiting the usefulness of reaction tools.

**Current State**:
- ✅ `conversations_add_reaction` tool works perfectly
- ✅ `conversations_remove_reaction` tool works perfectly
- ❌ **No way to list available emojis for reactions**

**Impact**:
- Users can't see what emoji options are available
- Limits the discovery of custom workspace emojis
- Makes reaction tools less user-friendly

**Proposed Solution**:
Add an `emoji_list` tool that calls Slack API endpoints:
```go
s.AddTool(mcp.NewTool("emoji_list",
    mcp.WithDescription("Get list of available emojis in the workspace"),
    mcp.WithString("type",
        mcp.Description("Filter by emoji type: 'all', 'custom', 'unicode', 'collections'")),
    mcp.WithNumber("limit",
        mcp.DefaultNumber(100),
        mcp.Description("Maximum number of emojis to return")),
    mcp.WithString("cursor",
        mcp.Description("Cursor for pagination")),
), emojiHandler.EmojiListHandler)
```

**Implementation Notes**:
- Use Slack API `emoji.list` endpoint for custom emojis
- Use `emoji.collections.list` for emoji collections
- Return CSV format for consistency with other tools
- Include emoji name, URL, collection info, etc.
- **IMPORTANT: Implement caching** similar to users/channels:
  - Cache emojis in `.emoji_cache.json`
  - Add `ProvideEmojiMap()` method to ApiProvider
  - Create `newEmojiWatcher()` in main.go
  - Emoji lists can be large (100s of custom emojis) - caching is essential for performance

**Edge Client API Details** (from dev_tools.txt):
- **Endpoint**: `{workspace}.slack.com/api/emoji.collections.list`
- **Required Headers**: 
  - `_x_reason: "emojiPack:dialog"`
  - `_x_mode: "online"`
  - `_x_sonic: "true"`
  - `_x_app_name: "client"`
- **Form Parameters**:
  - `token`: xoxc/xoxd token
  - `installed_only`: 0 (show all available collections)
- **Response Structure**:
  - `installed`: User's installed emoji collections
  - `available`: Workspace-available emoji collections
  - Each collection: `id`, `name`, `author`, `team_id`, `locale`, `date_create`, `is_draft`
  - Emoji format: `emoji_name: "https://emoji.slack-edge.com/collection/item/..."`
- **Implementation**: Use existing edge client pattern in `pkg/provider/edge/`

---

## 🔧 Potential Enhancements

### 3. Better Error Handling for Missing Cache
**Priority: Medium** | **Status: Partial**

**Problem**: When caches are missing, tools fail with generic errors instead of helpful guidance.

**Proposed Solution**:
- Add specific error messages for missing cache files
- Provide clear instructions on how to resolve cache issues
- Add health check endpoints for cache status

### 4. Cache Refresh Tools
**Priority: Medium** | **Status: Not Implemented**

**Problem**: No way to manually refresh caches without restarting the server.

**Proposed Solution**:
- Add `cache_refresh_users` tool
- Add `cache_refresh_channels` tool
- Add cache status monitoring

### 5. Enhanced Search Capabilities
**Priority: Low** | **Status: Partial**

**Problem**: Search could be more powerful with additional filters.

**Proposed Solution**:
- Add date range filtering
- Add user mention filtering
- Add channel-specific search
- Add search result highlighting

---

## 📋 Implementation Checklist

### Emoji List Tool
- [ ] Create `EmojiHandler` struct in `pkg/handler/`
- [ ] Implement `EmojiListHandler` method
- [ ] Add emoji caching infrastructure:
  - [ ] Add emoji cache fields to `ApiProvider` struct
  - [ ] Implement `ProvideEmojiMap()` method
  - [ ] Create `.emoji_cache.json` file handling
  - [ ] Add `newEmojiWatcher()` in main.go
- [ ] Add tool registration in `pkg/server/server.go`
- [ ] Add tests for the new tool
- [ ] Update documentation
- [ ] Test with MCP clients

---

## 🎯 Quick Wins

1. **Emoji List Tool** - Medium impact, low complexity
2. **Better Error Messages** - Improves user experience
3. **Cache Status Endpoints** - Helps with debugging

---

## ✅ Completed Features (Recently Implemented)

### Users List Tool
- ✅ Created `UsersHandler` struct in `pkg/handler/users.go`
- ✅ Implemented search functionality with `query` parameter
- ✅ Added filtering by user type (active, deleted, bots, humans, admins)
- ✅ Added field selection for token optimization
- ✅ Added pagination with cursor support
- ✅ Updated README.md with complete documentation
- ✅ Created comprehensive test suite

### Channels List Enhancement
- ✅ Added `query` parameter for searching channels
- ✅ Search works across name, topic, and purpose fields
- ✅ Updated default limit from 100 to 1000
- ✅ Updated test suite with search test cases

---

## 📝 Notes

- The server now has complete user and channel listing/searching capabilities
- Implementation should follow the established patterns in the codebase
- Consider backward compatibility when adding new tools
- Test thoroughly with different MCP clients (Cursor IDE, etc.)

---

*Last Updated: $(date)*
*Repository: korotovsky/slack-mcp-server*
*Fork: chrisguillory/slack-mcp-server*