# Slack MCP Server - TODO

## 🔄 Tool Renames (Align with Slack API)

### Priority: HIGH - Fix naming inconsistencies
These tools should be renamed to match Slack's official API naming conventions for clarity and maintainability:

**Message Operations (currently under conversations_, should be chat_):**
- [x] `conversations_add_message` → `chat_post_message` (currently wraps chat.postMessage API)
- [x] `conversations_search_messages` → `search_messages` (wraps search.messages API)

**Reaction Operations (currently under conversations_, should be reactions_):**
- [x] `conversations_add_reaction` → `reactions_add` (wraps reactions.add API)
- [x] `conversations_remove_reaction` → `reactions_remove` (wraps reactions.remove API)

**Keep As-Is (already correct):**
- ✅ `conversations_history` (correctly uses conversations.history API)
- ✅ `conversations_replies` (correctly uses conversations.replies API)
- ✅ `channels_list` (acceptable shorthand for conversations.list)
- ✅ `users_list` (correctly uses users.list API)
- ✅ `emoji_list` (correctly uses emoji.list API)

## 🆕 New Tools to Add

### Message Management
- [ ] `chat_update` - Edit/update existing messages (uses chat.update API)
- [ ] `chat_delete` - Delete messages (uses chat.delete API)

### Channel Information
- [ ] `conversations_members` - List channel members (uses conversations.members API)

### User Information  
- [ ] `users_info` - Get detailed user information (uses users.info API)
