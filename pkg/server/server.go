package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/korotovsky/slack-mcp-server/pkg/handler"
	"github.com/korotovsky/slack-mcp-server/pkg/provider"
	"github.com/korotovsky/slack-mcp-server/pkg/server/auth"
	"github.com/korotovsky/slack-mcp-server/pkg/text"
	"github.com/korotovsky/slack-mcp-server/pkg/version"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.uber.org/zap"
)

type MCPServer struct {
	server      *server.MCPServer
	logger      *zap.Logger
	fileHandler *handler.FileHandler
}

func NewMCPServer(provider *provider.ApiProvider, logger *zap.Logger) *MCPServer {
	s := server.NewMCPServer(
		"Slack MCP Server",
		version.Version,
		server.WithLogging(),
		server.WithRecovery(),
		server.WithToolHandlerMiddleware(buildLoggerMiddleware(logger)),
		server.WithToolHandlerMiddleware(auth.BuildMiddleware(provider.ServerTransport(), logger)),
	)

	conversationsHandler := handler.NewConversationsHandler(provider, logger)
	chatHandler := handler.NewChatHandler(provider, logger)
	reactionsHandler := handler.NewReactionsHandler(provider, logger)
	searchHandler := handler.NewSearchHandler(provider, logger)
	emojiHandler := handler.NewEmojiHandler(provider, logger)
	channelsHandler := handler.NewChannelsHandler(provider, logger)
	usersHandler := handler.NewUsersHandler(provider, logger)
	authHandler := handler.NewAuthHandler(provider, logger)

	// Get download directory from env var (empty string means use temp directory)
	downloadDir := os.Getenv("SLACK_MCP_DOWNLOAD_DIR")
	// Pass empty string to NewFileHandler - it will create a temp directory
	fileHandler := handler.NewFileHandler(provider, logger, downloadDir)

	s.AddTool(mcp.NewTool("get_current_user",
		mcp.WithDescription("Get information about the authenticated user (Slack API: auth.test)"),
	), authHandler.GetCurrentUserHandler)

	s.AddTool(mcp.NewTool("get_channel_messages",
		mcp.WithDescription("Get messages from a channel or DM (Slack API: conversations.history)"),
		mcp.WithString("channel_id",
			mcp.Required(),
			mcp.Description("    - `channel_id` (string): ID of the channel in format Cxxxxxxxxxx or its name starting with #... or @... aka #general or @username_dm."),
		),
		mcp.WithBoolean("include_activity_messages",
			mcp.Description("If true, the response will include activity messages such as 'channel_join' or 'channel_leave'. Default is boolean false."),
			mcp.DefaultBool(false),
		),
		mcp.WithString("cursor",
			mcp.Description("Cursor for pagination. Use the value of the last row and column in the response as next_cursor field returned from the previous request."),
		),
		mcp.WithString("limit",
			mcp.DefaultString("1d"),
			mcp.Description("Limit of messages to fetch in format of maximum ranges of time (e.g. 1d - 1 day, 1w - 1 week, 30d - 30 days, 90d - 90 days which is a default limit for free tier history) or number of messages (e.g. 50). Must be empty when 'cursor' is provided."),
		),
		mcp.WithString("fields",
			mcp.DefaultString("msgID,userUser,realName,text,time"),
			mcp.Description("Comma-separated list of fields to return. Options: 'msgID', 'userID', 'userUser', 'realName', 'channelID', 'threadTs', 'text', 'time', 'reactions', 'files', 'filesFull', 'cursor'. 'files' returns id:name:type:size (efficient), 'filesFull' adds URLs (verbose). Use 'all' for all fields except filesFull. Default: 'msgID,userUser,realName,text,time'"),
		),
	), conversationsHandler.ConversationsHistoryHandler)

	s.AddTool(mcp.NewTool("get_thread_messages",
		mcp.WithDescription("Get messages from a thread (Slack API: conversations.replies)"),
		mcp.WithString("channel_id",
			mcp.Required(),
			mcp.Description("ID of the channel in format Cxxxxxxxxxx or its name starting with #... or @... aka #general or @username_dm."),
		),
		mcp.WithString("thread_ts",
			mcp.Required(),
			mcp.Description("Unique identifier of either a thread's parent message or a message in the thread. ts must be the timestamp in format 1234567890.123456 of an existing message with 0 or more replies."),
		),
		mcp.WithBoolean("include_activity_messages",
			mcp.Description("If true, the response will include activity messages such as 'channel_join' or 'channel_leave'. Default is boolean false."),
			mcp.DefaultBool(false),
		),
		mcp.WithString("cursor",
			mcp.Description("Cursor for pagination. Use the value of the last row and column in the response as next_cursor field returned from the previous request."),
		),
		mcp.WithString("limit",
			mcp.DefaultString("1d"),
			mcp.Description("Limit of messages to fetch in format of maximum ranges of time (e.g. 1d - 1 day, 30d - 30 days, 90d - 90 days which is a default limit for free tier history) or number of messages (e.g. 50). Must be empty when 'cursor' is provided."),
		),
		mcp.WithString("fields",
			mcp.DefaultString("msgID,userUser,realName,text,time"),
			mcp.Description("Comma-separated list of fields to return. Options: 'msgID', 'userID', 'userUser', 'realName', 'channelID', 'threadTs', 'text', 'time', 'reactions', 'files', 'filesFull'. 'files' returns id:name:type:size (efficient), 'filesFull' adds URLs (verbose). Use 'all' for all fields except filesFull. Default: 'msgID,userUser,realName,text,time'"),
		),
	), conversationsHandler.ConversationsRepliesHandler)

	s.AddTool(mcp.NewTool("post_message",
		mcp.WithDescription("Post a message to a channel or DM (Slack API: chat.postMessage). Supports mrkdwn text and/or Block Kit blocks for rich formatting. When using blocks, text serves as fallback for notifications and accessibility."),
		mcp.WithString("channel_id",
			mcp.Required(),
			mcp.Description("ID of the channel in format Cxxxxxxxxxx or its name starting with #... or @... aka #general or @username_dm."),
		),
		mcp.WithString("thread_ts",
			mcp.Description("Unique identifier of either a thread's parent message or a message in the thread. Timestamp format: 1234567890.123456. Optional - if not provided, posts to channel; if provided, posts as reply."),
		),
		mcp.WithString("text",
			mcp.Description("Message text in Slack mrkdwn format. Required if blocks not provided. When blocks are provided, serves as fallback for notifications/accessibility. Syntax: *bold*, _italic_, ~strike~, `code`, ```codeblock```, >quote, <URL|text>, <@U123> mentions, <#C123> channels."),
		),
		mcp.WithString("blocks",
			mcp.Description("Block Kit blocks as JSON array string for rich layouts. Max 50 blocks. Common blocks: {\"type\":\"divider\"} for horizontal rules, {\"type\":\"section\",\"text\":{\"type\":\"mrkdwn\",\"text\":\"content\"}} for text sections, {\"type\":\"header\",\"text\":{\"type\":\"plain_text\",\"text\":\"title\"}} for headers. See: https://api.slack.com/block-kit"),
		),
		mcp.WithBoolean("reply_broadcast",
			mcp.Description("When replying to a thread (thread_ts provided), set to true to also send the reply to the main channel (visible to everyone). Similar to Slack's 'also send to channel' checkbox. Default: false. Only applies when thread_ts is set."),
			mcp.DefaultBool(false),
		),
	), chatHandler.ChatPostMessageHandler)

	// Post message as bot (uses separate bot token)
	s.AddTool(mcp.NewTool("post_message_as_bot",
		mcp.WithDescription("Post a message as the bot user (not as your personal user). Use this when you want messages to be clearly identified as coming from an AI assistant with a bot icon and 'APP' badge. Requires SLACK_MCP_BOT_TOKEN to be configured. Supports mrkdwn text and/or Block Kit blocks for rich formatting."),
		mcp.WithString("channel_id",
			mcp.Required(),
			mcp.Description("ID of the channel in format Cxxxxxxxxxx or its name starting with #... or @... aka #general or @username_dm."),
		),
		mcp.WithString("thread_ts",
			mcp.Description("Unique identifier of either a thread's parent message or a message in the thread. Timestamp format: 1234567890.123456. Optional - if not provided, posts to channel; if provided, posts as reply."),
		),
		mcp.WithString("text",
			mcp.Description("Message text in Slack mrkdwn format. Required if blocks not provided. When blocks are provided, serves as fallback for notifications/accessibility. Syntax: *bold*, _italic_, ~strike~, `code`, ```codeblock```, >quote, <URL|text>, <@U123> mentions, <#C123> channels."),
		),
		mcp.WithString("blocks",
			mcp.Description("Block Kit blocks as JSON array string for rich layouts. Max 50 blocks. Common blocks: {\"type\":\"divider\"} for horizontal rules, {\"type\":\"section\",\"text\":{\"type\":\"mrkdwn\",\"text\":\"content\"}} for text sections, {\"type\":\"header\",\"text\":{\"type\":\"plain_text\",\"text\":\"title\"}} for headers. See: https://api.slack.com/block-kit"),
		),
		mcp.WithBoolean("reply_broadcast",
			mcp.Description("When replying to a thread (thread_ts provided), set to true to also send the reply to the main channel (visible to everyone). Similar to Slack's 'also send to channel' checkbox. Default: false. Only applies when thread_ts is set."),
			mcp.DefaultBool(false),
		),
	), chatHandler.ChatPostMessageAsBotHandler)

	// Add reaction tool
	s.AddTool(mcp.NewTool("add_reaction",
		mcp.WithDescription("Add an emoji reaction to a message (Slack API: reactions.add)"),
		mcp.WithString("channel_id",
			mcp.Required(),
			mcp.Description("Channel ID (C...) or name (#general, @user_dm)")),
		mcp.WithString("timestamp",
			mcp.Required(),
			mcp.Description("Message timestamp (e.g., 1234567890.123456)")),
		mcp.WithString("emoji",
			mcp.Required(),
			mcp.Description("Emoji name without colons (e.g., thumbsup, rocket)")),
	), reactionsHandler.ReactionsAddHandler)

	// Remove reaction tool
	s.AddTool(mcp.NewTool("remove_reaction",
		mcp.WithDescription("Remove an emoji reaction from a message (Slack API: reactions.remove)"),
		mcp.WithString("channel_id",
			mcp.Required(),
			mcp.Description("Channel ID (C...) or name (#general, @user_dm)")),
		mcp.WithString("timestamp",
			mcp.Required(),
			mcp.Description("Message timestamp (e.g., 1234567890.123456)")),
		mcp.WithString("emoji",
			mcp.Required(),
			mcp.Description("Emoji name without colons (e.g., thumbsup, rocket)")),
	), reactionsHandler.ReactionsRemoveHandler)

	// Delete message tool
	s.AddTool(mcp.NewTool("delete_message",
		mcp.WithDescription("Delete a message from a channel (Slack API: chat.delete)"),
		mcp.WithString("channel_id",
			mcp.Required(),
			mcp.Description("Channel ID (C...) or name (#general, @user_dm)")),
		mcp.WithString("timestamp",
			mcp.Required(),
			mcp.Description("Message timestamp (e.g., 1234567890.123456)")),
	), chatHandler.ChatDeleteMessageHandler)

	// Update message tool
	s.AddTool(mcp.NewTool("update_message",
		mcp.WithDescription("Edit/update an existing message (Slack API: chat.update). Supports mrkdwn text and/or Block Kit blocks for rich formatting. When using blocks, text serves as fallback for notifications and accessibility."),
		mcp.WithString("channel_id",
			mcp.Required(),
			mcp.Description("Channel ID (C...) or name (#general, @user_dm)")),
		mcp.WithString("timestamp",
			mcp.Required(),
			mcp.Description("Message timestamp (e.g., 1234567890.123456)")),
		mcp.WithString("text",
			mcp.Description("New message text in Slack mrkdwn format. Required if blocks not provided. When blocks are provided, serves as fallback for notifications/accessibility. Syntax: *bold*, _italic_, ~strike~, `code`, ```codeblock```, >quote, <URL|text>, <@U123> mentions, <#C123> channels.")),
		mcp.WithString("blocks",
			mcp.Description("Block Kit blocks as JSON array string for rich layouts. Max 50 blocks. Common blocks: {\"type\":\"divider\"} for horizontal rules, {\"type\":\"section\",\"text\":{\"type\":\"mrkdwn\",\"text\":\"content\"}} for text sections, {\"type\":\"header\",\"text\":{\"type\":\"plain_text\",\"text\":\"title\"}} for headers. See: https://api.slack.com/block-kit")),
	), chatHandler.ChatUpdateHandler)

	// Update message as bot tool
	s.AddTool(mcp.NewTool("update_message_as_bot",
		mcp.WithDescription("Edit/update an existing bot message (Slack API: chat.update). Use this to update messages previously posted with post_message_as_bot. Requires SLACK_MCP_BOT_TOKEN to be configured. Supports mrkdwn text and/or Block Kit blocks for rich formatting."),
		mcp.WithString("channel_id",
			mcp.Required(),
			mcp.Description("Channel ID (C...) or name (#general, @user_dm)")),
		mcp.WithString("timestamp",
			mcp.Required(),
			mcp.Description("Message timestamp (e.g., 1234567890.123456)")),
		mcp.WithString("text",
			mcp.Description("New message text in Slack mrkdwn format. Required if blocks not provided. When blocks are provided, serves as fallback for notifications/accessibility.")),
		mcp.WithString("blocks",
			mcp.Description("Block Kit blocks as JSON array string for rich layouts. Max 50 blocks.")),
	), chatHandler.ChatUpdateMessageAsBotHandler)

	// Delete message as bot tool
	s.AddTool(mcp.NewTool("delete_message_as_bot",
		mcp.WithDescription("Delete a bot message (Slack API: chat.delete). Use this to delete messages previously posted with post_message_as_bot. Requires SLACK_MCP_BOT_TOKEN to be configured."),
		mcp.WithString("channel_id",
			mcp.Required(),
			mcp.Description("Channel ID (C...) or name (#general, @user_dm)")),
		mcp.WithString("timestamp",
			mcp.Required(),
			mcp.Description("Message timestamp (e.g., 1234567890.123456)")),
	), chatHandler.ChatDeleteMessageAsBotHandler)

	s.AddTool(mcp.NewTool("search_messages",
		mcp.WithDescription("Search for messages across channels and DMs (Slack API: search.messages)"),
		mcp.WithString("search_query",
			mcp.Description("Search query to filter messages. Example: 'marketing report' or full URL of Slack message e.g. 'https://slack.com/archives/C1234567890/p1234567890123456', then the tool will return a single message matching given URL, herewith all other parameters will be ignored."),
		),
		mcp.WithString("filter_in_channel",
			mcp.Description("Filter messages in a specific channel by its ID or name. Example: 'C1234567890' or '#general'. If not provided, all channels will be searched."),
		),
		mcp.WithString("filter_in_im_or_mpim",
			mcp.Description("Filter messages in a direct message (DM) or multi-person direct message (MPIM) conversation by its ID or name. Example: 'D1234567890' or '@username_dm'. If not provided, all DMs and MPIMs will be searched."),
		),
		mcp.WithString("filter_users_with",
			mcp.Description("Filter messages with a specific user in threads and DMs. Must use explicit format: '@username', 'U1234567890' (user ID), 'D1234567890' (DM channel ID), or special keyword 'me' for current user. Plain usernames without @ are not accepted."),
		),
		mcp.WithString("filter_users_from",
			mcp.Description("Filter messages from a specific user. Must use explicit format: '@username', 'U1234567890' (user ID), 'D1234567890' (DM channel ID), or special keyword 'me' for current user. Plain usernames without @ are not accepted."),
		),
		mcp.WithString("filter_date_before",
			mcp.Description("Filter messages sent before a specific date in format 'YYYY-MM-DD'. Example: '2023-10-01', 'July', 'Yesterday' or 'Today'. If not provided, all dates will be searched."),
		),
		mcp.WithString("filter_date_after",
			mcp.Description("Filter messages sent after a specific date in format 'YYYY-MM-DD'. Example: '2023-10-01', 'July', 'Yesterday' or 'Today'. If not provided, all dates will be searched."),
		),
		mcp.WithString("filter_date_on",
			mcp.Description("Filter messages sent on a specific date in format 'YYYY-MM-DD'. Example: '2023-10-01', 'July', 'Yesterday' or 'Today'. If not provided, all dates will be searched."),
		),
		mcp.WithString("filter_date_during",
			mcp.Description("Filter messages sent during a specific period in format 'YYYY-MM-DD'. Example: 'July', 'Yesterday' or 'Today'. If not provided, all dates will be searched."),
		),
		mcp.WithBoolean("filter_threads_only",
			mcp.Description("If true, the response will include only messages from threads. Default is boolean false."),
		),
		mcp.WithString("cursor",
			mcp.DefaultString(""),
			mcp.Description("Cursor for pagination. Use the value of the last row and column in the response as next_cursor field returned from the previous request."),
		),
		mcp.WithNumber("limit",
			mcp.DefaultNumber(20),
			mcp.Description("The maximum number of items to return. Must be an integer between 1 and 100."),
		),
		mcp.WithString("fields",
			mcp.DefaultString("msgID,userUser,realName,channelID,text,time"),
			mcp.Description("Comma-separated list of fields to return. Options: 'msgID', 'userID', 'userUser', 'realName', 'channelID', 'threadTs', 'text', 'time', 'reactions', 'permalink'. Use 'all' for all available fields. Default: 'msgID,userUser,realName,channelID,text,time'. Note: 'files' and 'filesFull' are NOT supported by search_messages - use get_channel_messages or get_thread_messages to retrieve file metadata."),
		),
		mcp.WithString("sort",
			mcp.DefaultString("relevance"),
			mcp.Description("Sort order for search results. Options: 'relevance' (default, by search score), 'newest_first' (by timestamp, most recent first), 'oldest_first' (by timestamp, oldest first). Default: 'relevance'"),
		),
	), searchHandler.SearchMessagesHandler)

	s.AddTool(mcp.NewTool("list_channels",
		mcp.WithDescription("List channels, DMs, and group DMs (Slack API: conversations.list)"),
		mcp.WithString("query",
			mcp.Description("Search for channels by name. Searches in channel name, topic, and purpose (case-insensitive)"),
		),
		mcp.WithString("channel_types",
			mcp.Required(),
			mcp.Description("Comma-separated channel types. Allowed values: 'mpim', 'im', 'public_channel', 'private_channel'. Example: 'public_channel,private_channel,im'"),
		),
		mcp.WithString("fields",
			mcp.DefaultString("id,name"),
			mcp.Description("Comma-separated list of fields to return. Options: 'id', 'name', 'topic', 'purpose', 'member_count'. Use 'all' for all fields (backward compatibility). Default: 'id,name'"),
		),
		mcp.WithNumber("min_members",
			mcp.DefaultNumber(0),
			mcp.Description("Only return channels with at least this many members. Use to filter out abandoned/test channels. Default: 0 (no filtering)"),
		),
		mcp.WithString("sort",
			mcp.Description("Type of sorting. Allowed values: 'popularity' - sort by number of members/participants in each channel."),
		),
		mcp.WithNumber("limit",
			mcp.DefaultNumber(1000),
			mcp.Description("The maximum number of items to return. Must be an integer between 1 and 1000."),
		),
		mcp.WithString("cursor",
			mcp.Description("Cursor for pagination. Use the cursor value returned from the previous request."),
		),
	), channelsHandler.ChannelsHandler)

	s.AddTool(mcp.NewTool("list_channel_members",
		mcp.WithDescription("List members of a channel, DM, or group DM (Slack API: conversations.members, conversations.info)"),
		mcp.WithString("channel_id",
			mcp.Required(),
			mcp.Description("Channel ID (C..., D..., G...) or name (#general, @user_dm)"),
		),
		mcp.WithNumber("limit",
			mcp.DefaultNumber(100),
			mcp.Description("The maximum number of members to return. Must be an integer between 1 and 1000. Default: 100"),
		),
		mcp.WithString("cursor",
			mcp.Description("Cursor for pagination. Use the cursor value returned from the previous request."),
		),
	), channelsHandler.ListChannelMembersHandler)

	s.AddTool(mcp.NewTool("list_users",
		mcp.WithDescription("List users in the workspace (Slack API: users.list). Returns transparency headers showing org member vs external user counts."),
		mcp.WithString("query",
			mcp.Description("Search for users by name. Searches in username, real name, and display name (case-insensitive)"),
		),
		mcp.WithString("user_type",
			mcp.DefaultString("all"),
			mcp.Description("Filter by enterprise user type: 'all' (everything), 'org_member' (your org's employees), 'external' (Slack Connect users from other orgs), 'deleted' (deactivated org members / former employees). Default: 'all'"),
		),
		mcp.WithString("filter",
			mcp.DefaultString("all"),
			mcp.Description("Filter users by status: 'all', 'active', 'deleted', 'bots', 'humans', 'admins'. Default: 'all'. Note: This filter applies AFTER user_type filtering. Use filter=deleted with user_type=all to see all deleted users; use user_type=deleted to see only deactivated org members."),
		),
		mcp.WithString("fields",
			mcp.DefaultString("id,name,real_name,status"),
			mcp.Description("Comma-separated list of fields to return. Options: 'id', 'name', 'real_name', 'email', 'status', 'is_bot', 'is_admin', 'time_zone', 'title', 'phone', 'enterprise_id', 'enterprise_name', 'team_id', 'is_org_member'. Use 'all' for all fields. Default: 'id,name,real_name,status'"),
		),
		mcp.WithBoolean("include_deleted",
			mcp.DefaultBool(false),
			mcp.Description("Include deleted/deactivated users in results. Default: false"),
		),
		mcp.WithBoolean("include_bots",
			mcp.DefaultBool(true),
			mcp.Description("Include bot users in results. Default: true"),
		),
		mcp.WithNumber("limit",
			mcp.DefaultNumber(1000),
			mcp.Description("The maximum number of items to return. Must be an integer between 1 and 1000. Default: 1000"),
		),
		mcp.WithString("cursor",
			mcp.Description("Cursor for pagination. Use the cursor value returned from the previous request."),
		),
	), usersHandler.UsersHandler)

	s.AddTool(mcp.NewTool("get_user_info",
		mcp.WithDescription("Get detailed information about a specific user (Slack API: users.info, users.getPresence)"),
		mcp.WithString("user_id",
			mcp.Required(),
			mcp.Description("User ID (U...) or username (@username)"),
		),
		mcp.WithString("fields",
			mcp.DefaultString("id,name,real_name,display_name,email,title,status_text,is_admin,is_bot"),
			mcp.Description("Comma-separated list of fields to return. Options include: id, team_id, name, real_name, display_name, email, phone, title, status_text, status_emoji, tz, is_admin, is_bot, is_restricted, image_192, presence, and many more. Use 'extended' for common fields, 'all' for all available fields. Default: basic set of commonly used fields"),
		),
	), usersHandler.GetUserInfoHandler)

	s.AddTool(mcp.NewTool("get_org_overview",
		mcp.WithDescription("Get a summary of the organization's user composition. Shows native vs external user counts, breakdown by title, and helps answer questions like 'how many employees do we have?' or 'what engineering roles exist?'"),
		mcp.WithString("group_by",
			mcp.DefaultString("title"),
			mcp.Description("How to group the summary. Options: 'title' (default, groups native active users by job title). More groupings may be added later."),
		),
	), usersHandler.GetOrgOverviewHandler)

	s.AddTool(mcp.NewTool("create_channel",
		mcp.WithDescription("Create a new public or private channel (Slack API: conversations.create)"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name for the new channel (lowercase, no spaces, max 80 chars)"),
		),
		mcp.WithBoolean("is_private",
			mcp.DefaultBool(false),
			mcp.Description("Whether to create a private channel. Default: false (public channel)"),
		),
		mcp.WithString("topic",
			mcp.Description("Initial topic for the channel (optional)"),
		),
		mcp.WithString("purpose",
			mcp.Description("Initial purpose/description for the channel (optional)"),
		),
		mcp.WithString("workspace",
			mcp.Description("Workspace Team ID (e.g., T08U80K08H4) for Enterprise Grid users with multiple workspaces (optional)"),
		),
	), channelsHandler.CreateChannelHandler)

	s.AddTool(mcp.NewTool("archive_channel",
		mcp.WithDescription("Archive a channel (Slack API: conversations.archive)"),
		mcp.WithString("channel_id",
			mcp.Required(),
			mcp.Description("Channel ID (C...) or name (#channel-name) to archive"),
		),
	), channelsHandler.ArchiveChannelHandler)

	s.AddTool(mcp.NewTool("list_emojis",
		mcp.WithDescription("List available emojis/reactions (Slack API: emoji.list)"),
		mcp.WithString("query",
			mcp.Description("Search for emojis by name (case-insensitive)"),
		),
		mcp.WithString("type",
			mcp.DefaultString("all"),
			mcp.Description("Filter by emoji type: 'all', 'custom', 'unicode'. Default: 'all'"),
		),
		mcp.WithNumber("limit",
			mcp.DefaultNumber(1000),
			mcp.Description("The maximum number of items to return. Must be an integer between 1 and 1000. Default: 1000"),
		),
		mcp.WithString("cursor",
			mcp.Description("Cursor for pagination. Use the cursor value returned from the previous request."),
		),
	), emojiHandler.EmojiListHandler)

	s.AddTool(mcp.NewTool("download_file",
		mcp.WithDescription("Download Slack files to local filesystem. Use file IDs from message 'files' or 'filesFull' fields. Files are downloaded with authentication and saved to the specified directory. Maximum file size: 50MB."),
		mcp.WithString("file_ids",
			mcp.Required(),
			mcp.Description("Array of file IDs to download (e.g., ['F09RFRJ8QSV', 'F09R0TL40DC']). File IDs are obtained from the 'files' or 'filesFull' fields in message responses. Can also be a single file ID string."),
		),
		mcp.WithString("output_dir",
			mcp.Description("Directory to save downloaded files. Defaults to './downloads' or SLACK_MCP_DOWNLOAD_DIR environment variable if set. Directory will be created if it doesn't exist."),
		),
	), fileHandler.DownloadFileHandler)

	s.AddTool(mcp.NewTool("get_file_info",
		mcp.WithDescription("Get file metadata including sharing status, permalink, and visibility (Slack API: files.info). Use to check if a file is public/private, which channels it's shared in, and get its permalink. File IDs come from the 'files' or 'filesFull' fields in message responses."),
		mcp.WithString("file_id",
			mcp.Required(),
			mcp.Description("Slack file ID (e.g., F09RFRJ8QSV). Obtained from the 'files' or 'filesFull' fields in message responses."),
		),
	), fileHandler.GetFileInfoHandler)

	s.AddTool(mcp.NewTool("upload_file",
		mcp.WithDescription("Upload a local file to a Slack channel (Slack API: files.uploadV2). The file must exist on the local filesystem (e.g., previously downloaded via download_file). The uploaded file is scoped to the target channel - it is private by default, visible only to channel members. Use the download_file + upload_file flow to copy a file from one conversation to another without changing the original file's permissions. Maximum file size: 50MB."),
		mcp.WithString("file_path",
			mcp.Required(),
			mcp.Description("Local filesystem path to the file to upload. Use the local_path value returned by download_file."),
		),
		mcp.WithString("channel_id",
			mcp.Required(),
			mcp.Description("Channel ID (C...), DM ID (D...), or name (#channel-name) to upload the file to."),
		),
		mcp.WithString("title",
			mcp.Description("Title for the uploaded file. Defaults to the filename if not provided."),
		),
		mcp.WithString("initial_comment",
			mcp.Description("Message text to accompany the file upload."),
		),
		mcp.WithString("thread_ts",
			mcp.Description("Thread timestamp to upload the file as a thread reply. Format: 1234567890.123456"),
		),
	), fileHandler.UploadFileHandler)

	s.AddTool(mcp.NewTool("make_file_public",
		mcp.WithDescription("Make a Slack file publicly accessible (Slack API: files.sharedPublicURL). Activates the file's public URL so it can be used in Block Kit image blocks or shared externally. WARNING: Anyone with the URL can view the file. Returns the public permalink."),
		mcp.WithString("file_id",
			mcp.Required(),
			mcp.Description("Slack file ID to make public (e.g., F09RFRJ8QSV)."),
		),
	), fileHandler.MakeFilePublicHandler)

	s.AddTool(mcp.NewTool("get_slack_templates",
		mcp.WithDescription("Get curated Block Kit templates for professional Slack messages. Returns the SLACK_TEMPLATES.md file with examples for status updates, alerts, meeting summaries, announcements, requests, reports, errors, and empty states. Use these templates as a starting point when composing well-formatted messages."),
	), chatHandler.GetSlackTemplatesHandler)

	logger.Info("Authenticating with Slack API...",
		zap.String("context", "console"),
	)
	ar, err := provider.Slack().AuthTest()
	if err != nil {
		logger.Fatal("Failed to authenticate with Slack",
			zap.String("context", "console"),
			zap.Error(err),
		)
	}

	logger.Info("Successfully authenticated with Slack",
		zap.String("context", "console"),
		zap.String("team", ar.Team),
		zap.String("user", ar.User),
		zap.String("enterprise", ar.EnterpriseID),
		zap.String("url", ar.URL),
	)

	ws, err := text.Workspace(ar.URL)
	if err != nil {
		logger.Fatal("Failed to parse workspace from URL",
			zap.String("context", "console"),
			zap.String("url", ar.URL),
			zap.Error(err),
		)
	}

	s.AddResource(mcp.NewResource(
		"slack://"+ws+"/channels",
		"Directory of Slack channels",
		mcp.WithResourceDescription("This resource provides a directory of Slack channels."),
		mcp.WithMIMEType("text/csv"),
	), channelsHandler.ChannelsResource)

	s.AddResource(mcp.NewResource(
		"slack://"+ws+"/users",
		"Directory of Slack users",
		mcp.WithResourceDescription("This resource provides a directory of Slack users."),
		mcp.WithMIMEType("text/csv"),
	), conversationsHandler.UsersResource)

	return &MCPServer{
		server:      s,
		logger:      logger,
		fileHandler: fileHandler,
	}
}

// Cleanup removes temporary files and directories created by the server.
// Should be called when the server exits (via defer in main.go).
func (s *MCPServer) Cleanup() {
	s.logger.Info("MCPServer.Cleanup() called", zap.String("context", "console"))
	if s.fileHandler != nil {
		s.fileHandler.Cleanup()
	} else {
		s.logger.Warn("No fileHandler to cleanup", zap.String("context", "console"))
	}
	s.logger.Info("MCPServer.Cleanup() finished", zap.String("context", "console"))
}

func (s *MCPServer) ServeSSE(addr string) *server.SSEServer {
	s.logger.Info("Creating SSE server",
		zap.String("context", "console"),
		zap.String("version", version.Version),
		zap.String("build_time", version.BuildTime),
		zap.String("commit_hash", version.CommitHash),
		zap.String("address", addr),
	)
	return server.NewSSEServer(s.server,
		server.WithBaseURL(fmt.Sprintf("http://%s", addr)),
		server.WithSSEContextFunc(func(ctx context.Context, r *http.Request) context.Context {
			ctx = auth.AuthFromRequest(s.logger)(ctx, r)

			return ctx
		}),
	)
}

func (s *MCPServer) ServeStdio() error {
	s.logger.Info("Starting STDIO server",
		zap.String("version", version.Version),
		zap.String("build_time", version.BuildTime),
		zap.String("commit_hash", version.CommitHash),
	)
	err := server.ServeStdio(s.server)
	if err != nil {
		s.logger.Error("STDIO server error", zap.Error(err))
	}
	return err
}

func buildLoggerMiddleware(logger *zap.Logger) server.ToolHandlerMiddleware {
	return func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			logger.Info("Request received",
				zap.String("tool", req.Params.Name),
				zap.Any("params", req.Params),
			)

			startTime := time.Now()

			res, err := next(ctx, req)

			duration := time.Since(startTime)

			logger.Info("Request finished",
				zap.String("tool", req.Params.Name),
				zap.Duration("duration", duration),
			)

			return res, err
		}
	}
}
