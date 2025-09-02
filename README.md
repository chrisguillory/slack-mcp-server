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
- **Safe Message Posting**: The `chat_post_message` tool is disabled by default for safety. Enable it via an environment variable, with optional channel restrictions.
- **DM and Group DM support**: Retrieve direct messages and group direct messages.
- **Embedded user information**: Embed user information in messages, for better context.
- **Cache support**: Cache users and channels for faster access.
- **Stdio/SSE Transports & Proxy Support**: Use the server with any MCP client that supports Stdio or SSE transports, and configure it to route outgoing requests through a proxy if needed.

### Analytics Demo

![Analytics](images/feature-1.gif)

### Add Message Demo

![Add Message](images/feature-2.gif)

## Tools

### 1. conversations_history:
Get messages from the channel (or DM) by channel_id, the last row/column in the response is used as 'cursor' parameter for pagination if not empty
- **Parameters:**
  - `channel_id` (string, required):     - `channel_id` (string): ID of the channel in format Cxxxxxxxxxx or its name starting with `#...` or `@...` aka `#general` or `@username_dm`.
  - `include_activity_messages` (boolean, default: false): If true, the response will include activity messages such as `channel_join` or `channel_leave`. Default is boolean false.
  - `cursor` (string, optional): Cursor for pagination. Use the value of the last row and column in the response as next_cursor field returned from the previous request.
  - `limit` (string, default: "1d"): Limit of messages to fetch in format of maximum ranges of time (e.g. 1d - 1 day, 1w - 1 week, 30d - 30 days, 90d - 90 days which is a default limit for free tier history) or number of messages (e.g. 50). Must be empty when 'cursor' is provided.
  - `fields` (string, default: "msgID,userUser,realName,text,time"): Comma-separated list of fields to return. Options: `msgID`, `userID`, `userUser`, `realName`, `channelID`, `threadTs`, `text`, `time`, `reactions`. Use `all` for all fields. Default optimizes for common use cases while reducing token usage.

### 2. conversations_replies:
Get a thread of messages posted to a conversation by channelID and `thread_ts`, the last row/column in the response is used as `cursor` parameter for pagination if not empty.
- **Parameters:**
  - `channel_id` (string, required): ID of the channel in format `Cxxxxxxxxxx` or its name starting with `#...` or `@...` aka `#general` or `@username_dm`.
  - `thread_ts` (string, required): Unique identifier of either a thread’s parent message or a message in the thread. ts must be the timestamp in format `1234567890.123456` of an existing message with 0 or more replies.
  - `include_activity_messages` (boolean, default: false): If true, the response will include activity messages such as 'channel_join' or 'channel_leave'. Default is boolean false.
  - `cursor` (string, optional): Cursor for pagination. Use the value of the last row and column in the response as next_cursor field returned from the previous request.
  - `limit` (string, default: "1d"): Limit of messages to fetch in format of maximum ranges of time (e.g. 1d - 1 day, 1w - 1 week, 30d - 30 days, 90d - 90 days which is a default limit for free tier history) or number of messages (e.g. 50). Must be empty when 'cursor' is provided.
  - `fields` (string, default: "msgID,userUser,realName,text,time"): Comma-separated list of fields to return. Options: `msgID`, `userID`, `userUser`, `realName`, `channelID`, `threadTs`, `text`, `time`, `reactions`. Use `all` for all fields. Default optimizes for common use cases while reducing token usage.

### 3. chat_post_message
Post a message to a public channel, private channel, or direct message (DM, or IM) conversation by channel_id and thread_ts.

> **Note:** Posting messages is disabled by default for safety. To enable, set the `SLACK_MCP_ADD_MESSAGE_TOOL` environment variable. If set to a comma-separated list of channel IDs, posting is enabled only for those specific channels. See the Environment Variables section below for details.

- **Parameters:**
  - `channel_id` (string, required): ID of the channel in format `Cxxxxxxxxxx` or its name starting with `#...` or `@...` aka `#general` or `@username_dm`.
  - `thread_ts` (string, optional): Unique identifier of either a thread’s parent message or a message in the thread_ts must be the timestamp in format `1234567890.123456` of an existing message with 0 or more replies. Optional, if not provided the message will be added to the channel itself, otherwise it will be added to the thread.
  - `payload` (string, required): Message payload in specified content_type format. Example: 'Hello, world!' for text/plain or '# Hello, world!' for text/markdown.
  - `content_type` (string, default: "text/markdown"): Content type of the message. Default is 'text/markdown'. Allowed values: 'text/markdown', 'text/plain'.

### 4. search_messages
Search messages in a public channel, private channel, or direct message (DM, or IM) conversation using filters. All filters are optional, if not provided then search_query is required.
- **Parameters:**
  - `search_query` (string, optional): Search query to filter messages. Example: 'marketing report' or full URL of Slack message e.g. 'https://slack.com/archives/C1234567890/p1234567890123456', then the tool will return a single message matching given URL, herewith all other parameters will be ignored.
  - `filter_in_channel` (string, optional): Filter messages in a specific channel by its ID or name. Example: `C1234567890` or `#general`. If not provided, all channels will be searched.
  - `filter_in_im_or_mpim` (string, optional): Filter messages in a direct message (DM) or multi-person direct message (MPIM) conversation by its ID or name. Example: `D1234567890` or `@username_dm`. If not provided, all DMs and MPIMs will be searched.
  - `filter_users_with` (string, optional): Filter messages with a specific user by their ID or display name in threads and DMs. Example: `U1234567890` or `@username`. If not provided, all threads and DMs will be searched.
  - `filter_users_from` (string, optional): Filter messages from a specific user by their ID or display name. Example: `U1234567890` or `@username`. If not provided, all users will be searched.
  - `filter_date_before` (string, optional): Filter messages sent before a specific date in format `YYYY-MM-DD`. Example: `2023-10-01`, `July`, `Yesterday` or `Today`. If not provided, all dates will be searched.
  - `filter_date_after` (string, optional): Filter messages sent after a specific date in format `YYYY-MM-DD`. Example: `2023-10-01`, `July`, `Yesterday` or `Today`. If not provided, all dates will be searched.
  - `filter_date_on` (string, optional): Filter messages sent on a specific date in format `YYYY-MM-DD`. Example: `2023-10-01`, `July`, `Yesterday` or `Today`. If not provided, all dates will be searched.
  - `filter_date_during` (string, optional): Filter messages sent during a specific period in format `YYYY-MM-DD`. Example: `July`, `Yesterday` or `Today`. If not provided, all dates will be searched.
  - `filter_threads_only` (boolean, default: false): If true, the response will include only messages from threads. Default is boolean false.
  - `cursor` (string, optional): Cursor for pagination. Use the cursor value returned from the previous request.
  - `limit` (number, default: 100): The maximum number of items to return. Must be an integer between 1 and 100.
  - `fields` (string, default: "msgID,userUser,realName,channelID,text,time"): Comma-separated list of fields to return. Options: `msgID`, `userID`, `userUser`, `realName`, `channelID`, `threadTs`, `text`, `time`, `reactions`, `permalink`. Use `all` for all fields. Default excludes `permalink` for token efficiency. To include message permalinks, add `permalink` to the fields list.
- **Response Format:**
  The response includes metadata comments at the beginning:
  - `# Total messages: X` - Total number of messages matching the search criteria
  - `# Total pages: Y` - Total number of pages available
  - `# Current page: Z` - Current page number
  - `# Items per page: N` - Number of items per page
  - `# Returned in this page: M` - Number of messages in this response
  - `# Item range: A-B` - Range of items in the current page (when available)
  - `# Next cursor: C` - Cursor for the next page, or "(none - last page)" if no more pages

### 5. channels_list:
Get list of channels with search, filtering, and optimized field selection for token efficiency
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

### 6. users_list:
Get list of users in the workspace with flexible filtering, search, and field selection
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

### 7. reactions_add:
Add an emoji reaction to a message
- **Parameters:**
  - `channel_id` (string, required): Channel ID (C...) or name (#general, @user_dm)
  - `timestamp` (string, required): Message timestamp (e.g., 1234567890.123456)
  - `emoji` (string, required): Emoji name without colons (e.g., thumbsup, rocket)

### 8. reactions_remove:
Remove an emoji reaction from a message
- **Parameters:**
  - `channel_id` (string, required): Channel ID (C...) or name (#general, @user_dm)
  - `timestamp` (string, required): Message timestamp (e.g., 1234567890.123456)
  - `emoji` (string, required): Emoji name without colons (e.g., thumbsup, rocket)

### 9. chat_delete_message:
Delete a message from a channel

> **Note:** Deleting messages is disabled by default for safety. To enable, set the `SLACK_MCP_DELETE_MESSAGE_TOOL` environment variable. If set to a comma-separated list of channel IDs, deletion is enabled only for those specific channels. See the Environment Variables section below for details.

- **Parameters:**
  - `channel_id` (string, required): Channel ID (C...) or name (#general, @user_dm)
  - `timestamp` (string, required): Message timestamp (e.g., 1234567890.123456)

### 10. chat_update_message:
Edit/update an existing message in a channel

> **Note:** Updating messages is disabled by default for safety. To enable, set the `SLACK_MCP_UPDATE_MESSAGE_TOOL` environment variable. If set to a comma-separated list of channel IDs, updating is enabled only for those specific channels. See the Environment Variables section below for details.

- **Parameters:**
  - `channel_id` (string, required): Channel ID (C...) or name (#general, @user_dm)
  - `timestamp` (string, required): Message timestamp (e.g., 1234567890.123456)
  - `payload` (string, required): New message content in specified content_type format
  - `content_type` (string, default: "text/markdown"): Content type of the message. Allowed values: 'text/markdown', 'text/plain'

## Resources

The Slack MCP Server exposes two special directory resources for easy access to workspace metadata:

### 1. `slack://<workspace>/channels` — Directory of Channels

Fetches a CSV directory of all channels in the workspace, including public channels, private channels, DMs, and group DMs.

- **URI:** `slack://<workspace>/channels`
- **Format:** `text/csv`
- **Fields:**
  - `id`: Channel ID (e.g., `C1234567890`)
  - `name`: Channel name (e.g., `#general`, `@username_dm`)
  - `topic`: Channel topic (if any)
  - `purpose`: Channel purpose/description
  - `memberCount`: Number of members in the channel

### 2. `slack://<workspace>/users` — Directory of Users

Fetches a CSV directory of all users in the workspace.

- **URI:** `slack://<workspace>/users`
- **Format:** `text/csv`
- **Fields:**
  - `userID`: User ID (e.g., `U1234567890`)
  - `userName`: Slack username (e.g., `john`)
  - `realName`: User’s real name (e.g., `John Doe`)

## Setup Guide

- [Authentication Setup](docs/01-authentication-setup.md)
- [Installation](docs/02-installation.md)
- [Configuration and Usage](docs/03-configuration-and-usage.md)

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
| `SLACK_MCP_ADD_MESSAGE_TOOL`      | No        | `nil`                     | Enable message posting via `chat_post_message` by setting it to true for all channels, a comma-separated list of channel IDs to whitelist specific channels, or use `!` before a channel ID to allow all except specified ones, while an empty value disables posting by default. |
| `SLACK_MCP_ADD_REACTION_TOOL`     | No        | `nil`                     | Enable reaction management via `reactions_add` and `reactions_remove` by setting it to true for all channels, a comma-separated list of channel IDs to whitelist specific channels, or use `!` before a channel ID to allow all except specified ones. |
| `SLACK_MCP_DELETE_MESSAGE_TOOL`   | No        | `nil`                     | Enable message deletion via `chat_delete_message` by setting it to true for all channels, a comma-separated list of channel IDs to whitelist specific channels, or use `!` before a channel ID to allow all except specified ones. |
| `SLACK_MCP_UPDATE_MESSAGE_TOOL`   | No        | `nil`                     | Enable message updating via `chat_update_message` by setting it to true for all channels, a comma-separated list of channel IDs to whitelist specific channels, or use `!` before a channel ID to allow all except specified ones. |
| `SLACK_MCP_ADD_MESSAGE_MARK`      | No        | `nil`                     | When the `chat_post_message` tool is enabled, any new message sent will automatically be marked as read.                                                                                                                                                                          |
| `SLACK_MCP_ADD_MESSAGE_UNFURLING` | No        | `nil`                     | Enable to let Slack unfurl posted links or set comma-separated list of domains e.g. `github.com,slack.com` to whitelist unfurling only for them. If text contains whitelisted and unknown domain unfurling will be disabled for security reasons.                                         |
| `SLACK_MCP_USERS_CACHE`           | No        | `.users_cache.json`       | Path to the users cache file. Used to cache Slack user information to avoid repeated API calls on startup.                                                                                                                                                                                |
| `SLACK_MCP_CHANNELS_CACHE`        | No        | `.channels_cache_v2.json` | Path to the channels cache file. Used to cache Slack channel information to avoid repeated API calls on startup.                                                                                                                                                                          |
| `SLACK_MCP_EMOJIS_CACHE`          | No        | `.emojis_cache.json`      | Path to the emojis cache file. Used to cache Slack emoji information to avoid repeated API calls on startup.                                                                                                                                                                              |
| `SLACK_MCP_LOG_LEVEL`             | No        | `info`                    | Log-level for stdout or stderr. Valid values are: `debug`, `info`, `warn`, `error`, `panic` and `fatal`                                                                                                                                                                                   |

*You need either `xoxp` **or** both `xoxc`/`xoxd` tokens for authentication.

### Limitations matrix & Cache

| Users Cache        | Channels Cache     | Limitations                                                                                                                                                                                                                                                                                                                                        |
|--------------------|--------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| :x:                | :x:                | No cache, No LLM context enhancement with user data, tools `channels_list` and `users_list` will be fully not functional. Tools `conversations_*` will have limited capabilities and you won't be able to search messages by `@userHandle` or `#channel-name`, getting messages by `@userHandle` or `#channel-name` won't be available either. |
| :white_check_mark: | :x:                | No channels cache, tool `channels_list` will be fully not functional. Tool `users_list` will work. Tools `conversations_*` will have limited capabilities and you won't be able to search messages by `#channel-name`, getting messages by `#channel-name` won't be available either.                                                          |
| :white_check_mark: | :white_check_mark: | No limitations, fully functional Slack MCP Server with all tools operational.                                                                                                                                                                                                                                                                      |

### Debugging Tools

```bash
# Run the inspector with stdio transport
npx @modelcontextprotocol/inspector go run mcp/mcp-server.go --transport stdio

# View logs
tail -n 20 -f ~/Library/Logs/Claude/mcp*.log
```

## Security

- Never share API tokens
- Keep .env files secure and private

## License

Licensed under MIT - see [LICENSE](LICENSE) file. This is not an official Slack product.
