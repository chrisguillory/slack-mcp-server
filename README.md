# Slack MCP Server
[![Trust Score](https://archestra.ai/mcp-catalog/api/badge/quality/korotovsky/slack-mcp-server)](https://archestra.ai/mcp-catalog/korotovsky__slack-mcp-server)

Model Context Protocol (MCP) server for Slack Workspaces. The most powerful MCP Slack server — supports Stdio and SSE transports, proxy settings, DMs, Group DMs, Smart History fetch (by date or count), may work via OAuth or in complete stealth mode with no permissions and scopes in Workspace 😏.

> [!NOTE]
> **Stealth Mode Technical Details**: *Browser session tokens* (**xoxc**/**xoxd**) are compatible with the **standard Slack API** in addition to the Edge API. The **xoxc token** functions as a bearer token for API authentication, while the **xoxd token** is sent as a cookie. This combination *enables full access* to Slack's standard API endpoints **without installing a Slack app or granting extra permissions**.

> [!IMPORTANT]  
> We need your support! Each month, over 30,000 engineers visit this repository, and more than 9,000 are already using it.
> 
> If you appreciate the work our [contributors](https://github.com/korotovsky/slack-mcp-server/graphs/contributors) have put into this project, please consider giving the repository a star.

This feature-rich Slack MCP Server has:
- **Stealth and OAuth Modes**: Operate the server in **stealth mode** by using browser session tokens (**xoxc**/**xoxd**), which allow access to Slack's standard API **without requiring additional permissions or bot installation**; or in **OAuth mode** with secure OAuth tokens, enabling access without extracting tokens from the browser or needing token refresh.
- **Enterprise Workspaces Support**: Possibility to integrate with Enterprise Slack setups.
- **Channel and Thread Support with `#Name` `@Lookup`**: Fetch messages from channels and threads, including activity messages, and retrieve channels using their names (e.g., #general) as well as their IDs.
- **Smart History**: Fetch messages with pagination by date (d1, 7d, 1m) or message count.
- **Search Messages**: Search messages in channels, threads, and DMs using various filters like date, user, and content.
- **Safe Message Posting**: The `post_message` tool is disabled by default for safety. Enable it via an environment variable, with optional channel restrictions.
- **DM and Group DM support**: Retrieve direct messages and group direct messages.
- **Embedded user information**: Embed user information in messages, for better context.
- **Cache support**: Cache users and channels for faster access.
- **Stdio/SSE Transports & Proxy Support**: Use the server with any MCP client that supports Stdio or SSE transports, and configure it to route outgoing requests through a proxy if needed.

### Analytics Demo

![Analytics](images/feature-1.gif)

### Add Message Demo

![Add Message](images/feature-2.gif)

## Tools

### 1. get_channel_messages
Get messages from a channel or DM
- **Parameters:**
  - `channel_id` (string, required): ID of the channel in format Cxxxxxxxxxx or its name starting with `#...` or `@...` aka `#general` or `@username_dm`.
  - `include_activity_messages` (boolean, default: false): If true, the response will include activity messages such as `channel_join` or `channel_leave`. Default is boolean false.
  - `cursor` (string, optional): Cursor for pagination. Use the value of the last row and column in the response as next_cursor field returned from the previous request.
  - `limit` (string, default: "1d"): Limit of messages to fetch in format of maximum ranges of time (e.g. 1d - 1 day, 1w - 1 week, 30d - 30 days, 90d - 90 days which is a default limit for free tier history) or number of messages (e.g. 50). Must be empty when 'cursor' is provided.
  - `fields` (string, default: "msgID,userUser,realName,text,time"): Comma-separated list of fields to return. Options: `msgID`, `userID`, `userUser`, `realName`, `channelID`, `threadTs`, `text`, `time`, `reactions`. Use `all` for all fields. Default optimizes for common use cases while reducing token usage.

### 2. get_thread_messages
Get messages from a thread
- **Parameters:**
  - `channel_id` (string, required): ID of the channel in format `Cxxxxxxxxxx` or its name starting with `#...` or `@...` aka `#general` or `@username_dm`.
  - `thread_ts` (string, required): Unique identifier of either a thread's parent message or a message in the thread. ts must be the timestamp in format `1234567890.123456` of an existing message with 0 or more replies.
  - `include_activity_messages` (boolean, default: false): If true, the response will include activity messages such as 'channel_join' or 'channel_leave'. Default is boolean false.
  - `cursor` (string, optional): Cursor for pagination. Use the value of the last row and column in the response as next_cursor field returned from the previous request.
  - `limit` (string, default: "1d"): Limit of messages to fetch in format of maximum ranges of time (e.g. 1d - 1 day, 1w - 1 week, 30d - 30 days, 90d - 90 days which is a default limit for free tier history) or number of messages (e.g. 50). Must be empty when 'cursor' is provided.
  - `fields` (string, default: "msgID,userUser,realName,text,time"): Comma-separated list of fields to return. Options: `msgID`, `userID`, `userUser`, `realName`, `channelID`, `threadTs`, `text`, `time`, `reactions`. Use `all` for all fields. Default optimizes for common use cases while reducing token usage.

### 3. post_message
Post a message to a channel or DM

> **Note:** Posting messages is disabled by default for safety. To enable, set the `SLACK_MCP_ADD_MESSAGE_TOOL` environment variable. If set to a comma-separated list of channel IDs, posting is enabled only for those specific channels. See the Environment Variables section below for details.

- **Parameters:**
  - `channel_id` (string, required): ID of the channel in format `Cxxxxxxxxxx` or its name starting with `#...` or `@...` aka `#general` or `@username_dm`.
  - `thread_ts` (string, optional): Unique identifier of either a thread's parent message or a message in the thread_ts must be the timestamp in format `1234567890.123456` of an existing message with 0 or more replies. Optional, if not provided the message will be added to the channel itself, otherwise it will be added to the thread.
  - `text` (string): Message text in Slack mrkdwn format. Required if blocks not provided.
  - `blocks` (string, optional): Block Kit blocks as JSON array string for rich layouts.
  - `reply_broadcast` (boolean, optional, default: false): When replying to a thread (thread_ts provided), set to true to also post the reply to the main channel. Equivalent to checking "also send to #channel-name" in Slack's UI. Use sparingly for important updates. Ignored if thread_ts is not provided.

### 4. post_message_as_bot
Post a message as a bot user instead of the authenticated user

> **Note:** This tool requires a separate bot token (`SLACK_MCP_BOT_TOKEN`) to be configured. Messages will appear with the bot's identity (bot icon and "APP" badge) rather than your personal user. Uses the same channel restrictions as `post_message` (via `SLACK_MCP_ADD_MESSAGE_TOOL` or `SLACK_MCP_BOT_MESSAGE_TOOL`).

- **Parameters:**
  - `channel_id` (string, required): ID of the channel in format `Cxxxxxxxxxx` or its name starting with `#...` or `@...` aka `#general` or `@username_dm`.
  - `thread_ts` (string, optional): Unique identifier of either a thread's parent message or a message in the thread. Timestamp format: `1234567890.123456`. Optional - if not provided, posts to channel; if provided, posts as reply.
  - `text` (string): Message text in Slack mrkdwn format. Required if blocks not provided. Syntax: `*bold*`, `_italic_`, `~strike~`, `` `code` ``, `>quote`, `<URL|text>`, `<@U123>` mentions, `<#C123>` channels.
  - `blocks` (string, optional): Block Kit blocks as JSON array string for rich layouts. Max 50 blocks.
  - `reply_broadcast` (boolean, optional, default: false): When replying to a thread (thread_ts provided), set to true to also post the reply to the main channel. Equivalent to checking "also send to #channel-name" in Slack's UI. Use sparingly for important updates. Ignored if thread_ts is not provided.
- **Use Cases:**
  - When messages should be clearly identified as coming from an AI assistant
  - When you don't want messages to appear from your personal account
  - When you need visual distinction (bot icon, "APP" badge) for AI-generated content

### 5. search_messages
Search for messages across channels and DMs
- **Parameters:**
  - `search_query` (string, optional): Search query to filter messages. Example: 'marketing report' or full URL of Slack message e.g. 'https://slack.com/archives/C1234567890/p1234567890123456', then the tool will return a single message matching given URL, herewith all other parameters will be ignored.
  - `filter_in_channel` (string, optional): Filter messages in a specific channel by its ID or name. Example: `C1234567890` or `#general`. If not provided, all channels will be searched.
  - `filter_in_im_or_mpim` (string, optional): Filter messages in a direct message (DM) or multi-person direct message (MPIM) conversation by its ID or name. Example: `D1234567890` or `@username_dm`. If not provided, all DMs and MPIMs will be searched.
  - `filter_users_with` (string, optional): Filter messages with a specific user in threads and DMs. Must use explicit format: `@username`, `U1234567890` (user ID), `D1234567890` (DM channel ID), or special keyword `me` for current user. Plain usernames without @ are not accepted.
  - `filter_users_from` (string, optional): Filter messages from a specific user. Must use explicit format: `@username`, `U1234567890` (user ID), `D1234567890` (DM channel ID), or special keyword `me` for current user. Plain usernames without @ are not accepted.
  - `filter_date_before` (string, optional): Filter messages sent before a specific date in format `YYYY-MM-DD`. Example: `2023-10-01`, `July`, `Yesterday` or `Today`. If not provided, all dates will be searched.
  - `filter_date_after` (string, optional): Filter messages sent after a specific date in format `YYYY-MM-DD`. Example: `2023-10-01`, `July`, `Yesterday` or `Today`. If not provided, all dates will be searched.
  - `filter_date_on` (string, optional): Filter messages sent on a specific date in format `YYYY-MM-DD`. Example: `2023-10-01`, `July`, `Yesterday` or `Today`. If not provided, all dates will be searched.
  - `filter_date_during` (string, optional): Filter messages sent during a specific period in format `YYYY-MM-DD`. Example: `July`, `Yesterday` or `Today`. If not provided, all dates will be searched.
  - `filter_threads_only` (boolean, default: false): If true, the response will include only messages from threads. Default is boolean false.
  - `cursor` (string, optional): Cursor for pagination. Use the cursor value returned from the previous request.
  - `limit` (number, default: 100): The maximum number of items to return. Must be an integer between 1 and 100.
  - `fields` (string, default: "msgID,userUser,realName,channelID,text,time"): Comma-separated list of fields to return. Options: `msgID`, `userID`, `userUser`, `realName`, `channelID`, `threadTs`, `text`, `time`, `reactions`, `permalink`. Use `all` for all fields. Default excludes `permalink` for token efficiency. To include message permalinks, add `permalink` to the fields list.
  - `sort` (string, default: "relevance"): Sort order for search results. Options: `relevance` (by search score), `newest_first` (by timestamp, most recent first), `oldest_first` (by timestamp, oldest first).
- **Response Format:**
  The response includes metadata comments at the beginning:
  - `# Total messages: X` - Total number of messages matching the search criteria
  - `# Total pages: Y` - Total number of pages available
  - `# Current page: Z` - Current page number
  - `# Items per page: N` - Number of items per page
  - `# Returned in this page: M` - Number of messages in this response
  - `# Item range: A-B` - Range of items in the current page (when available)
  - `# Next cursor: C` - Cursor for the next page, or "(none - last page)" if no more pages

### 6. list_channels
List channels, DMs, and group DMs
- **Parameters:**
  - `query` (string, optional): Search for channels by name. Searches in channel name, topic, and purpose (case-insensitive)
  - `channel_types` (string, required): Comma-separated channel types. Allowed values: `mpim`, `im`, `public_channel`, `private_channel`. Example: `public_channel,private_channel,im`
  - `fields` (string, default: "id,name"): Comma-separated list of fields to return. Options: `id`, `name`, `topic`, `purpose`, `member_count`. Use `all` for all fields (backward compatibility). Default: `id,name`
  - `min_members` (number, default: 0): Only return channels with at least this many members. Use to filter out abandoned/test channels. Default: 0 (no filtering)
  - `sort` (string, optional): Type of sorting. Allowed values: `popularity` - sort by number of members/participants in each channel.
  - `limit` (number, default: 1000): The maximum number of items to return. Must be an integer between 1 and 1000.
  - `cursor` (string, optional): Cursor for pagination. Use the cursor value returned from the previous request.
- **Response Format:**
  The response includes metadata comments at the beginning:
  - `# Total channels: X` - Total number of channels matching the filter criteria
  - `# Returned in this page: Y` - Number of channels in this response
  - `# Next cursor: Z` - Cursor for the next page, or "(none - last page)" if no more pages

### 7. list_users
List users in the workspace
- **Parameters:**
  - `query` (string, optional): Search for users by name. Searches in username, real name, and display name (case-insensitive)
  - `filter` (string, default: "all"): Filter users by status: `all`, `active`, `deleted`, `bots`, `humans`, `admins`. Default: `all`
  - `fields` (string, default: "id,name,real_name,status"): Comma-separated list of fields to return. Options: `id`, `name`, `real_name`, `email`, `status`, `is_bot`, `is_admin`, `time_zone`, `title`, `phone`. Use `all` for all fields. Default: `id,name,real_name,status`
  - `include_deleted` (boolean, default: false): Include deleted/deactivated users in results. Default: false
  - `include_bots` (boolean, default: true): Include bot users in results. Default: true
  - `limit` (number, default: 1000): The maximum number of items to return. Must be an integer between 1 and 1000. Default: 1000
  - `cursor` (string, optional): Cursor for pagination. Use the cursor value returned from the previous request.
- **Response Format:**
  The response includes metadata comments at the beginning:
  - `# Total users: X` - Total number of users matching the filter criteria
  - `# Returned in this page: Y` - Number of users in this response
  - `# Next cursor: Z` - Cursor for the next page, or "(none - last page)" if no more pages

### 8. list_emojis
List available emojis/reactions
- **Parameters:**
  - `query` (string, optional): Search for emojis by name (case-insensitive)
  - `type` (string, default: "all"): Filter by emoji type: `all`, `custom`, `unicode`. Default: `all`
  - `limit` (number, default: 1000): The maximum number of items to return. Must be an integer between 1 and 1000. Default: 1000
  - `cursor` (string, optional): Cursor for pagination. Use the cursor value returned from the previous request.
- **Response Format:**
  The response includes metadata comments at the beginning:
  - `# Total emojis: X` - Total number of emojis matching the filter criteria
  - `# Returned in this page: Y` - Number of emojis in this response
  - `# Next cursor: Z` - Cursor for the next page, or "(none - last page)" if no more pages

### 9. add_reaction
Add an emoji reaction to a message
- **Parameters:**
  - `channel_id` (string, required): Channel ID (C...) or name (#general, @user_dm)
  - `timestamp` (string, required): Message timestamp (e.g., 1234567890.123456)
  - `emoji` (string, required): Emoji name without colons (e.g., thumbsup, rocket)

### 10. remove_reaction
Remove an emoji reaction from a message
- **Parameters:**
  - `channel_id` (string, required): Channel ID (C...) or name (#general, @user_dm)
  - `timestamp` (string, required): Message timestamp (e.g., 1234567890.123456)
  - `emoji` (string, required): Emoji name without colons (e.g., thumbsup, rocket)

### 11. delete_message
Delete a message from a channel

> **Note:** Deleting messages is disabled by default for safety. To enable, set the `SLACK_MCP_DELETE_MESSAGE_TOOL` environment variable. If set to a comma-separated list of channel IDs, deletion is enabled only for those specific channels. See the Environment Variables section below for details.

- **Parameters:**
  - `channel_id` (string, required): Channel ID (C...) or name (#general, @user_dm)
  - `timestamp` (string, required): Message timestamp (e.g., 1234567890.123456)

### 12. update_message
Edit/update an existing message

> **Note:** Updating messages is disabled by default for safety. To enable, set the `SLACK_MCP_UPDATE_MESSAGE_TOOL` environment variable. If set to a comma-separated list of channel IDs, updating is enabled only for those specific channels. See the Environment Variables section below for details.

- **Parameters:**
  - `channel_id` (string, required): Channel ID (C...) or name (#general, @user_dm)
  - `timestamp` (string, required): Message timestamp (e.g., 1234567890.123456)
  - `payload` (string, required): New message content in specified content_type format
  - `content_type` (string, default: "text/plain"): Content type of the message. Allowed values: 'text/plain', 'text/markdown'. Use 'text/plain' for simple text updates to avoid block_mismatch errors.

### 13. get_current_user
Get information about the authenticated user

- **Parameters:** None
- **Response Format:**
  Returns CSV with the following fields:
  - `user_id`: Unique identifier of the authenticated user
  - `user_name`: Username of the authenticated user
  - `team_id`: Unique identifier of the workspace/team
  - `team_name`: Name of the workspace/team
  - `workspace_url`: Full URL of the workspace
  - `enterprise_id`: Enterprise Grid ID (if applicable)

### 14. list_channel_members
List members of a channel, DM, or group DM

- **Parameters:**
  - `channel_id` (string, required): Channel ID (C...) or name (#general, @user_dm)
  - `limit` (number, default: 1000): Maximum number of members to return (1-1000)
  - `cursor` (string, optional): Pagination cursor from previous request
- **Response Format:**
  Returns CSV with metadata comments and the following fields:
  - `user_id`: User ID of the member
  - `username`: Username of the member
  - `real_name`: Real name of the member
  - `display_name`: Display name of the member
  - `is_bot`: Whether the user is a bot
  - `is_admin`: Whether the user is an admin
  - `status_text`: User's status text
  - `status_emoji`: User's status emoji
- **Notes:**
  - For 1:1 DMs, returns information about both participants
  - For channels and group DMs, uses conversations.members API
  - Supports Enterprise Grid workspaces with automatic fallback

### 15. get_user_info
Get detailed information about a specific user

- **Parameters:**
  - `user_id` (string, required): User ID (U...) or username (@username)
  - `fields` (string, default: "id,name,real_name,display_name,email,title,status_text,is_admin,is_bot"): 
    Comma-separated list of fields to return. Options include:
    - Basic: `id`, `name`, `real_name`, `display_name`, `email`
    - Status: `status_text`, `status_emoji`, `status_expiration`
    - Profile: `title`, `phone`, `skype`, `first_name`, `last_name`
    - Admin: `is_admin`, `is_owner`, `is_primary_owner`, `is_restricted`, `is_ultra_restricted`
    - Bot: `is_bot`, `is_app_user`
    - Timezone: `tz`, `tz_label`, `tz_offset`
    - Presence: `presence` (online/away/dnd/offline)
    - Use `all` to return all available fields
- **Response Format:**
  Returns CSV with requested fields as columns
- **Notes:**
  - Combines data from users.info and users.getPresence APIs
  - Some fields may be empty depending on workspace permissions

### 16. create_channel
Create a new public or private channel

- **Parameters:**
  - `name` (string, required): Name for the new channel (lowercase, no spaces, max 80 chars)
  - `is_private` (boolean, default: false): Whether to create a private channel
  - `topic` (string, optional): Initial topic for the channel
  - `purpose` (string, optional): Initial purpose/description for the channel
  - `workspace` (string, optional): Workspace Team ID (e.g., T08U80K08H4) for Enterprise Grid users with multiple workspaces. If not specified and user belongs to multiple workspaces, an error will be returned asking to specify the workspace.
- **Response Format:**
  Returns CSV with metadata comments and the following fields:
  - `channel_id`: ID of the created channel
  - `name`: Name of the channel
  - `is_private`: Whether the channel is private
  - `topic`: Channel topic (if set)
  - `purpose`: Channel purpose/description (if set)
  - `created`: Always "true" for successful creation
- **Notes:**
  - Channel names must be unique within the workspace
  - Topic and purpose are set via separate API calls after creation
  - Creating private channels may require additional permissions

### 17. archive_channel
Archive a channel to preserve its history while removing it from active use

- **Parameters:**
  - `channel_id` (string, required): Channel ID (C...) or name (#channel-name) to archive
- **Response Format:**
  Returns CSV with metadata comments and the following fields:
  - `channel_id`: ID of the archived channel
  - `name`: Name of the channel
  - `archived`: Always "true" for successful archival
- **Notes:**
  - Archived channels can be unarchived later (not yet implemented)
  - Some channels (like #general) cannot be archived
  - Archiving preserves all messages and files

## Resources

The Slack MCP Server exposes two special directory resources for easy access to workspace metadata:

### 1. `slack://<workspace>/channels` — Directory of Channels

A CSV file containing all accessible channels in your workspace. This resource provides a quick way to explore channel structure and membership information without making individual API calls.

### 2. `slack://<workspace>/users` — Directory of Users

A CSV file containing all users in your workspace. This includes both active and deactivated users, along with their basic profile information.

## Installation & Configuration

See the project documentation for detailed installation and configuration instructions.

### Environment Variables (Quick Reference)

| Variable                          | Required? | Default                   | Description                                                                                                                                                                                                                                                                               |
|-----------------------------------|-----------|---------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `SLACK_MCP_XOXC_TOKEN`            | Yes*      | `nil`                     | Slack browser token (`xoxc-...`)                                                                                                                                                                                                                                                          |
| `SLACK_MCP_XOXD_TOKEN`            | Yes*      | `nil`                     | Slack browser cookie `d` (`xoxd-...`)                                                                                                                                                                                                                                                     |
| `SLACK_MCP_XOXP_TOKEN`            | Yes*      | `nil`                     | User OAuth token (`xoxp-...`) — alternative to xoxc/xoxd                                                                                                                                                                                                                                  |
| `SLACK_MCP_PORT`                  | No        | `13080`                   | Port for the MCP server to listen on                                                                                                                                                                                                                                                      |
| `SLACK_MCP_HOST`                  | No        | `127.0.0.1`               | Host for the MCP server to listen on                                                                                                                                                                                                                                                      |
| `SLACK_MCP_SSE_API_KEY`           | No        | `nil`                     | Bearer token for SSE transport                                                                                                                                                                                                                                                            |
| `SLACK_MCP_PROXY`                 | No        | `nil`                     | Proxy URL for outgoing requests                                                                                                                                                                                                                                                           |
| `SLACK_MCP_USER_AGENT`            | No        | `nil`                     | Custom User-Agent (for Enterprise Slack environments)                                                                                                                                                                                                                                     |
| `SLACK_MCP_CUSTOM_TLS`            | No        | `nil`                     | Send custom TLS-handshake to Slack servers based on `SLACK_MCP_USER_AGENT` or default User-Agent. (for Enterprise Slack environments)                                                                                                                                                     |
| `SLACK_MCP_SERVER_CA`             | No        | `nil`                     | Path to CA certificate                                                                                                                                                                                                                                                                    |
| `SLACK_MCP_SERVER_CA_TOOLKIT`     | No        | `nil`                     | Inject HTTPToolkit CA certificate to root trust-store for MitM debugging                                                                                                                                                                                                                  |
| `SLACK_MCP_SERVER_CA_INSECURE`    | No        | `false`                   | Trust all insecure requests (NOT RECOMMENDED)                                                                                                                                                                                                                                             |
| `SLACK_MCP_ADD_MESSAGE_TOOL`      | No        | `nil`                     | Enable message posting via `post_message` by setting it to true for all channels, a comma-separated list of channel IDs to whitelist specific channels, or use `!` before a channel ID to allow all except specified ones, while an empty value disables posting by default. |
| `SLACK_MCP_ADD_REACTION_TOOL`     | No        | `nil`                     | Enable reaction management via `add_reaction` and `remove_reaction` by setting it to true for all channels, a comma-separated list of channel IDs to whitelist specific channels, or use `!` before a channel ID to allow all except specified ones. |
| `SLACK_MCP_DELETE_MESSAGE_TOOL`   | No        | `nil`                     | Enable message deletion via `delete_message` by setting it to true for all channels, a comma-separated list of channel IDs to whitelist specific channels, or use `!` before a channel ID to allow all except specified ones. |
| `SLACK_MCP_UPDATE_MESSAGE_TOOL`   | No        | `nil`                     | Enable message updating via `update_message` by setting it to true for all channels, a comma-separated list of channel IDs to whitelist specific channels, or use `!` before a channel ID to allow all except specified ones. |
| `SLACK_MCP_BOT_TOKEN`             | No        | `nil`                     | Bot token (`xoxb-...`) for posting messages as a bot identity via `post_message_as_bot`. Separate from user authentication tokens. |
| `SLACK_MCP_BOT_MESSAGE_TOOL`      | No        | `nil`                     | Enable bot message posting via `post_message_as_bot`. Falls back to `SLACK_MCP_ADD_MESSAGE_TOOL` if not set. Same format: true, comma-separated channel IDs, or `!` prefix for exclusions. |
| `SLACK_MCP_ADD_MESSAGE_MARK`      | No        | `nil`                     | When the `post_message` tool is enabled, any new message sent will automatically be marked as read.                                                                                                                                                                          |
| `SLACK_MCP_ADD_MESSAGE_UNFURLING` | No        | `nil`                     | Enable to let Slack unfurl posted links or set comma-separated list of domains e.g. `github.com,slack.com` to whitelist unfurling only for them. If text contains whitelisted and unknown domain unfurling will be disabled for security reasons.                                         |
| `SLACK_MCP_USERS_CACHE`           | No        | `.users_cache.json`       | Path to the users cache file. Used to cache Slack user information to avoid repeated API calls on startup.                                                                                                                                                                                |
| `SLACK_MCP_CHANNELS_CACHE`        | No        | `.channels_cache_v2.json` | Path to the channels cache file. Used to cache Slack channel information to avoid repeated API calls on startup.                                                                                                                                                                          |
| `SLACK_MCP_EMOJIS_CACHE`          | No        | `.emojis_cache.json`      | Path to the emojis cache file. Used to cache Slack emoji information to avoid repeated API calls on startup.                                                                                                                                                                              |
| `SLACK_MCP_LOG_LEVEL`             | No        | `info`                    | Log-level for stdout or stderr. Valid values are: `debug`, `info`, `warn`, `error`, `panic` and `fatal`                                                                                                                                                                                   |

*You need either `xoxp` **or** both `xoxc`/`xoxd` tokens for authentication.

### Limitations matrix & Cache

| Users Cache        | Channels Cache     | Limitations                                                                                                                                                                                                                                                                                                                                        |
|--------------------|--------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| :x:                | :x:                | No cache, No LLM context enhancement with user data, tools `list_channels` and `list_users` will be fully not functional. Tools `get_channel_messages` and `get_thread_messages` will have limited capabilities and you won't be able to search messages by `@userHandle` or `#channel-name`, getting messages by `@userHandle` or `#channel-name` won't be available either. |
| :white_check_mark: | :x:                | No channels cache, tool `list_channels` will be fully not functional. Tool `list_users` will work. Tools `get_channel_messages` and `get_thread_messages` will have limited capabilities and you won't be able to search messages by `#channel-name`, getting messages by `#channel-name` won't be available either.                                                          |
| :white_check_mark: | :white_check_mark: | No limitations, fully functional Slack MCP Server with all tools operational.                                                                                                                                                                                                                                                                      |

### Debugging Tools

```bash
# Run the inspector with stdio transport
npx @modelcontextprotocol/inspector go run mcp/mcp-server.go --transport stdio
```