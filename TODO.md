# TODO: Slack MCP Server Improvements

This file tracks planned improvements, missing features, and enhancements for the Slack MCP Server repository.

## 🚨 Missing Core Functionality

### 1. Users List Tool
**Priority: High** | **Status: Not Implemented**

**Problem**: The server has no way for MCP clients (like Cursor IDE) to list users, even though all user data is cached and available.

**Current State**:
- ✅ User data is cached in `.users_cache.json`
- ✅ User resolution works internally (search, message processing)
- ✅ `channels_list` tool exists and works perfectly
- ❌ **No `users_list` tool exists**

**Impact**:
- MCP clients (like Cursor IDE) cannot discover workspace users
- No way to get user directory information
- Limits the usefulness of the MCP server for user management tasks

**Proposed Solution**:
Add a `users_list` tool similar to `channels_list`:
```go
s.AddTool(mcp.NewTool("users_list",
    mcp.WithDescription("Get list of users in the workspace"),
    mcp.WithString("filter",
        mcp.Description("Filter users by status: 'all', 'active', 'deleted', 'bots'")),
    mcp.WithNumber("limit",
        mcp.DefaultNumber(100),
        mcp.Description("Maximum number of users to return")),
    mcp.WithString("cursor",
        mcp.Description("Cursor for pagination")),
), usersHandler.UsersHandler)
```

**Implementation Notes**:
- Use existing `ch.apiProvider.ProvideUsersMap()` 
- Follow same pattern as `channels_list`
- Return CSV format for consistency
- Include user ID, name, real name, status, etc.

---

### 2. Emoji List Tool
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

## 🚀 Performance & Token Optimization

### 3. Optimize channels_list Token Usage
**Priority: High** | **Status: Not Implemented**

**Problem**: The `channels_list` tool is inefficient with tokens, always returning full topic/purpose fields that can be hundreds of characters each, even when not needed.

**Current State**:
- ❌ Always returns all fields (id, name, topic, purpose, memberCount)
- ❌ Topic/purpose fields often contain hundreds of characters
- ❌ No way to select specific fields
- ❌ Cursor injected into last CSV row (breaks structure)

**Impact**:
- Uses 70-80% more tokens than necessary for typical queries
- AI clients pay for unnecessary data transfer
- Makes the tool expensive to use repeatedly

**Proposed Solution**:
Add parameters to control output verbosity:

```go
s.AddTool(mcp.NewTool("channels_list",
    // ... existing parameters ...
    mcp.WithString("fields",
        mcp.DefaultString("id,name"),
        mcp.Description("Comma-separated list of fields to return. Options: 'id', 'name', 'topic', 'purpose', 'member_count'. Use 'all' for backward compatibility.")),
    mcp.WithNumber("min_members",
        mcp.DefaultNumber(0),
        mcp.Description("Only return channels with at least this many members")),
), channelsHandler.ChannelsHandler)
```

**Implementation Notes**:
- Default `fields="id,name"` reduces tokens by ~80%
- `min_members` filters out abandoned/test channels
- Move cursor to response metadata instead of CSV body
- Keep CSV format (more efficient than JSON for tabular data)
- Maintain backward compatibility with `fields="all"`

**Expected Benefits**:
- 70-80% reduction in token usage for typical queries
- Faster response times
- Lower costs for AI clients
- Cleaner CSV structure without cursor injection

---

## 🔧 Potential Enhancements

### 4. Better Error Handling for Missing Cache
**Priority: Medium** | **Status: Partial**

**Problem**: When caches are missing, tools fail with generic errors instead of helpful guidance.

**Proposed Solution**:
- Add specific error messages for missing cache files
- Provide clear instructions on how to resolve cache issues
- Add health check endpoints for cache status

### 5. Cache Refresh Tools
**Priority: Medium** | **Status: Not Implemented**

**Problem**: No way to manually refresh caches without restarting the server.

**Proposed Solution**:
- Add `cache_refresh_users` tool
- Add `cache_refresh_channels` tool
- Add cache status monitoring

### 6. Enhanced Search Capabilities
**Priority: Low** | **Status: Partial**

**Problem**: Search could be more powerful with additional filters.

**Proposed Solution**:
- Add date range filtering
- Add user mention filtering
- Add channel-specific search
- Add search result highlighting

---

## 📋 Implementation Checklist

### Users List Tool
- [ ] Create `UsersHandler` struct in `pkg/handler/`
- [ ] Implement `UsersHandler` method
- [ ] Add tool registration in `pkg/server/server.go`
- [ ] Add tests for the new tool
- [ ] Update documentation
- [ ] Test with MCP clients

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

### Documentation Updates
- [ ] Update README.md with new `users_list` tool
- [ ] Add examples of user listing usage
- [ ] Update configuration documentation
- [ ] Add troubleshooting section for cache issues

---

## 🎯 Quick Wins

1. **Users List Tool** - High impact, low complexity
2. **Emoji List Tool** - Medium impact, low complexity
3. **Better Error Messages** - Improves user experience
4. **Cache Status Endpoints** - Helps with debugging

---

## 📝 Notes

- The server already has all the infrastructure needed for user listing
- Implementation should follow the established patterns in the codebase
- Consider backward compatibility when adding new tools
- Test thoroughly with different MCP clients (Cursor IDE, etc.)

---

*Last Updated: $(date)*
*Repository: korotovsky/slack-mcp-server*
*Fork: chrisguillory/slack-mcp-server*
