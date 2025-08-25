# TODO: Slack MCP Server Improvements

This file tracks planned improvements, missing features, and enhancements for the Slack MCP Server repository.

## 🚨 Missing Core Functionality

*(Currently empty - all core functionality has been implemented!)*

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

## 🎯 Quick Wins

1. **Better Error Messages** - Improves user experience
2. **Cache Status Endpoints** - Helps with debugging

---

## ✅ Completed Features (Recently Implemented)

### Emoji List Tool
- ✅ Created `EmojiHandler` struct in `pkg/handler/emoji.go`
- ✅ Implemented listing of all available emojis/reactions
- ✅ Added filtering by type (custom, unicode, all)
- ✅ Added search functionality with `query` parameter  
- ✅ Added pagination with cursor support (default limit: 1000)
- ✅ Implemented emoji caching in `.emojis_cache.json`
- ✅ Added Docker volume mounts for cache persistence
- ✅ Created comprehensive test suite in markdown format
- ✅ Updated all documentation (README, docs, manifest)

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