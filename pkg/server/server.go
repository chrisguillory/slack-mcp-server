package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
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

const (
	// User's tool names
	ToolGetCurrentUser       = "get_current_user"
	ToolGetChannelMessages   = "get_channel_messages"
	ToolGetThreadMessages    = "get_thread_messages"
	ToolPostMessage          = "post_message"
	ToolPostMessageAsBot     = "post_message_as_bot"
	ToolAddReaction           = "add_reaction"
	ToolRemoveReaction        = "remove_reaction"
	ToolDeleteMessage        = "delete_message"
	ToolUpdateMessage        = "update_message"
	ToolUpdateMessageAsBot   = "update_message_as_bot"
	ToolDeleteMessageAsBot   = "delete_message_as_bot"
	ToolSearchMessages       = "search_messages"
	ToolListChannels         = "list_channels"
	ToolListChannelMembers   = "list_channel_members"
	ToolListUsers            = "list_users"
	ToolGetUserInfo          = "get_user_info"
	ToolGetOrgOverview       = "get_org_overview"
	ToolCreateChannel        = "create_channel"
	ToolArchiveChannel       = "archive_channel"
	ToolListEmojis           = "list_emojis"
	ToolDownloadFile         = "download_file"
	ToolGetFileInfo          = "get_file_info"
	ToolUploadFile           = "upload_file"
	ToolMakeFilePublic       = "make_file_public"
	ToolGetSlackTemplates    = "get_slack_templates"

	// Upstream tool names (new tools not in user's fork)
	ToolConversationsUnreads  = "conversations_unreads"
	ToolConversationsMark     = "conversations_mark"
	ToolUsergroupsList        = "usergroups_list"
	ToolUsergroupsMe          = "usergroups_me"
	ToolUsergroupsCreate      = "usergroups_create"
	ToolUsergroupsUpdate      = "usergroups_update"
	ToolUsergroupsUsersUpdate = "usergroups_users_update"
)

var ValidToolNames = []string{
	ToolGetCurrentUser,
	ToolGetChannelMessages,
	ToolGetThreadMessages,
	ToolPostMessage,
	ToolPostMessageAsBot,
	ToolAddReaction,
	ToolRemoveReaction,
	ToolDeleteMessage,
	ToolUpdateMessage,
	ToolUpdateMessageAsBot,
	ToolDeleteMessageAsBot,
	ToolSearchMessages,
	ToolListChannels,
	ToolListChannelMembers,
	ToolListUsers,
	ToolGetUserInfo,
	ToolGetOrgOverview,
	ToolCreateChannel,
	ToolArchiveChannel,
	ToolListEmojis,
	ToolDownloadFile,
	ToolGetFileInfo,
	ToolUploadFile,
	ToolMakeFilePublic,
	ToolGetSlackTemplates,
	ToolConversationsUnreads,
	ToolConversationsMark,
	ToolUsergroupsList,
	ToolUsergroupsMe,
	ToolUsergroupsCreate,
	ToolUsergroupsUpdate,
	ToolUsergroupsUsersUpdate,
}

func ValidateEnabledTools(tools []string) error {
	validToolSet := make(map[string]bool, len(ValidToolNames))
	for _, name := range ValidToolNames {
		validToolSet[name] = true
	}

	var invalidTools []string
	for _, tool := range tools {
		if !validToolSet[tool] {
			invalidTools = append(invalidTools, tool)
		}
	}
	if len(invalidTools) > 0 {
		return fmt.Errorf("invalid tool name(s): %s. Valid tools are: %s",
			strings.Join(invalidTools, ", "),
			strings.Join(ValidToolNames, ", "))
	}
	return nil
}

func shouldAddTool(name string, enabledTools []string, envVarName string) bool {
	if envVarName == "" {
		if len(enabledTools) == 0 {
			return true
		}
		return slices.Contains(enabledTools, name)
	}

	if len(enabledTools) > 0 && slices.Contains(enabledTools, name) {
		return true
	}

	if len(enabledTools) == 0 {
		return os.Getenv(envVarName) != ""
	}

	return false
}

func NewMCPServer(provider *provider.ApiProvider, logger *zap.Logger, enabledTools []string) *MCPServer {
	s := server.NewMCPServer(
		"Slack MCP Server",
		version.Version,
		server.WithLogging(),
		server.WithRecovery(),
		server.WithToolHandlerMiddleware(buildErrorRecoveryMiddleware(logger)),
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
	usergroupsHandler := handler.NewUsergroupsHandler(provider, logger)

	// Get download directory from env var (empty string means use temp directory)
	downloadDir := os.Getenv("SLACK_MCP_DOWNLOAD_DIR")
	// Pass empty string to NewFileHandler - it will create a temp directory
	fileHandler := handler.NewFileHandler(provider, logger, downloadDir)

	if shouldAddTool(ToolGetCurrentUser, enabledTools, "") {
		s.AddTool(mcp.NewTool(ToolGetCurrentUser,
			mcp.WithDescription("Get information about the authenticated user (Slack API: auth.test)"),
		), authHandler.GetCurrentUserHandler)
	}

	if shouldAddTool(ToolGetChannelMessages, enabledTools, "") {
		s.AddTool(mcp.NewTool(ToolGetChannelMessages,
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
	}

	if shouldAddTool(ToolGetThreadMessages, enabledTools, "") {
		s.AddTool(mcp.NewTool(ToolGetThreadMessages,
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
	}

	if shouldAddTool(ToolPostMessage, enabledTools, "SLACK_MCP_ADD_MESSAGE_TOOL") {
		s.AddTool(mcp.NewTool(ToolPostMessage,
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
	}

	// Post message as bot (uses separate bot token)
	if shouldAddTool(ToolPostMessageAsBot, enabledTools, "SLACK_MCP_ADD_MESSAGE_TOOL") {
		s.AddTool(mcp.NewTool(ToolPostMessageAsBot,
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
	}

	// Add reaction tool
	if shouldAddTool(ToolAddReaction, enabledTools, "SLACK_MCP_REACTION_TOOL") {
		s.AddTool(mcp.NewTool(ToolAddReaction,
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
	}

	// Remove reaction tool
	if shouldAddTool(ToolRemoveReaction, enabledTools, "SLACK_MCP_REACTION_TOOL") {
		s.AddTool(mcp.NewTool(ToolRemoveReaction,
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
	}

	// Delete message tool
	if shouldAddTool(ToolDeleteMessage, enabledTools, "SLACK_MCP_ADD_MESSAGE_TOOL") {
		s.AddTool(mcp.NewTool(ToolDeleteMessage,
			mcp.WithDescription("Delete a message from a channel (Slack API: chat.delete)"),
			mcp.WithString("channel_id",
				mcp.Required(),
				mcp.Description("Channel ID (C...) or name (#general, @user_dm)")),
			mcp.WithString("timestamp",
				mcp.Required(),
				mcp.Description("Message timestamp (e.g., 1234567890.123456)")),
		), chatHandler.ChatDeleteMessageHandler)
	}

	// Update message tool
	if shouldAddTool(ToolUpdateMessage, enabledTools, "SLACK_MCP_ADD_MESSAGE_TOOL") {
		s.AddTool(mcp.NewTool(ToolUpdateMessage,
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
	}

	// Update message as bot tool
	if shouldAddTool(ToolUpdateMessageAsBot, enabledTools, "SLACK_MCP_ADD_MESSAGE_TOOL") {
		s.AddTool(mcp.NewTool(ToolUpdateMessageAsBot,
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
	}

	// Delete message as bot tool
	if shouldAddTool(ToolDeleteMessageAsBot, enabledTools, "SLACK_MCP_ADD_MESSAGE_TOOL") {
		s.AddTool(mcp.NewTool(ToolDeleteMessageAsBot,
			mcp.WithDescription("Delete a bot message (Slack API: chat.delete). Use this to delete messages previously posted with post_message_as_bot. Requires SLACK_MCP_BOT_TOKEN to be configured."),
			mcp.WithString("channel_id",
				mcp.Required(),
				mcp.Description("Channel ID (C...) or name (#general, @user_dm)")),
			mcp.WithString("timestamp",
				mcp.Required(),
				mcp.Description("Message timestamp (e.g., 1234567890.123456)")),
		), chatHandler.ChatDeleteMessageAsBotHandler)
	}

	// Search messages tool - only register for non-bot tokens (bot tokens cannot use search.messages API)
	if !provider.IsBotToken() && shouldAddTool(ToolSearchMessages, enabledTools, "") {
		s.AddTool(mcp.NewTool(ToolSearchMessages,
			mcp.WithDescription("Search for messages across channels and DMs (Slack API: search.messages)"),
			mcp.WithString("search_query",
				mcp.Description("Search query to filter messages. Example: 'marketing report' or full URL of Slack message e.g. 'https://slack.com/archives/C1234567890/p1234567890123456', then the tool will return a single message matching given URL, herewith all other parameters will be ignored."),
			),
			mcp.WithString("filter_in_channel",
				mcp.Description("Filter messages in a specific public/private channel by its ID or name. Example: 'C1234567890', 'G1234567890', or '#general'. If not provided, all channels will be searched."),
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
	}

	if shouldAddTool(ToolListChannels, enabledTools, "") {
		s.AddTool(mcp.NewTool(ToolListChannels,
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
	}

	if shouldAddTool(ToolListChannelMembers, enabledTools, "") {
		s.AddTool(mcp.NewTool(ToolListChannelMembers,
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
	}

	if shouldAddTool(ToolListUsers, enabledTools, "") {
		s.AddTool(mcp.NewTool(ToolListUsers,
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
	}

	if shouldAddTool(ToolGetUserInfo, enabledTools, "") {
		s.AddTool(mcp.NewTool(ToolGetUserInfo,
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
	}

	if shouldAddTool(ToolGetOrgOverview, enabledTools, "") {
		s.AddTool(mcp.NewTool(ToolGetOrgOverview,
			mcp.WithDescription("Get a summary of the organization's user composition. Shows native vs external user counts, breakdown by title, and helps answer questions like 'how many employees do we have?' or 'what engineering roles exist?'"),
			mcp.WithString("group_by",
				mcp.DefaultString("title"),
				mcp.Description("How to group the summary. Options: 'title' (default, groups native active users by job title). More groupings may be added later."),
			),
		), usersHandler.GetOrgOverviewHandler)
	}

	if shouldAddTool(ToolCreateChannel, enabledTools, "") {
		s.AddTool(mcp.NewTool(ToolCreateChannel,
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
	}

	if shouldAddTool(ToolArchiveChannel, enabledTools, "") {
		s.AddTool(mcp.NewTool(ToolArchiveChannel,
			mcp.WithDescription("Archive a channel (Slack API: conversations.archive)"),
			mcp.WithString("channel_id",
				mcp.Required(),
				mcp.Description("Channel ID (C...) or name (#channel-name) to archive"),
			),
		), channelsHandler.ArchiveChannelHandler)
	}

	if shouldAddTool(ToolListEmojis, enabledTools, "") {
		s.AddTool(mcp.NewTool(ToolListEmojis,
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
	}

	if shouldAddTool(ToolDownloadFile, enabledTools, "") {
		s.AddTool(mcp.NewTool(ToolDownloadFile,
			mcp.WithDescription("Download Slack files to local filesystem. Use file IDs from message 'files' or 'filesFull' fields. Files are downloaded with authentication and saved to the specified directory. Maximum file size: 50MB."),
			mcp.WithString("file_ids",
				mcp.Required(),
				mcp.Description("Array of file IDs to download (e.g., ['F09RFRJ8QSV', 'F09R0TL40DC']). File IDs are obtained from the 'files' or 'filesFull' fields in message responses. Can also be a single file ID string."),
			),
			mcp.WithString("output_dir",
				mcp.Description("Directory to save downloaded files. Defaults to './downloads' or SLACK_MCP_DOWNLOAD_DIR environment variable if set. Directory will be created if it doesn't exist."),
			),
		), fileHandler.DownloadFileHandler)
	}

	if shouldAddTool(ToolGetFileInfo, enabledTools, "") {
		s.AddTool(mcp.NewTool(ToolGetFileInfo,
			mcp.WithDescription("Get file metadata including sharing status, permalink, and visibility (Slack API: files.info). Use to check if a file is public/private, which channels it's shared in, and get its permalink. File IDs come from the 'files' or 'filesFull' fields in message responses."),
			mcp.WithString("file_id",
				mcp.Required(),
				mcp.Description("Slack file ID (e.g., F09RFRJ8QSV). Obtained from the 'files' or 'filesFull' fields in message responses."),
			),
		), fileHandler.GetFileInfoHandler)
	}

	if shouldAddTool(ToolUploadFile, enabledTools, "") {
		s.AddTool(mcp.NewTool(ToolUploadFile,
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
	}

	if shouldAddTool(ToolMakeFilePublic, enabledTools, "") {
		s.AddTool(mcp.NewTool(ToolMakeFilePublic,
			mcp.WithDescription("Make a Slack file publicly accessible (Slack API: files.sharedPublicURL). Activates the file's public URL so it can be used in Block Kit image blocks or shared externally. WARNING: Anyone with the URL can view the file. Returns the public permalink."),
			mcp.WithString("file_id",
				mcp.Required(),
				mcp.Description("Slack file ID to make public (e.g., F09RFRJ8QSV)."),
			),
		), fileHandler.MakeFilePublicHandler)
	}

	if shouldAddTool(ToolGetSlackTemplates, enabledTools, "") {
		s.AddTool(mcp.NewTool(ToolGetSlackTemplates,
			mcp.WithDescription("Get curated Block Kit templates for professional Slack messages. Returns the SLACK_TEMPLATES.md file with examples for status updates, alerts, meeting summaries, announcements, requests, reports, errors, and empty states. Use these templates as a starting point when composing well-formatted messages."),
		), chatHandler.GetSlackTemplatesHandler)
	}

	// Upstream tools: unreads, mark, usergroups

	// Register unreads tool - gets all unread messages across channels efficiently.
	// Bot tokens (xoxb) don't support unread tracking, so exclude them (same pattern as search tool).
	if !provider.IsBotToken() && shouldAddTool(ToolConversationsUnreads, enabledTools, "") {
		s.AddTool(mcp.NewTool(ToolConversationsUnreads,
			mcp.WithDescription("Get unread messages across all channels. With browser session tokens (xoxc/xoxd), uses a single API call for complete results. With OAuth user tokens (xoxp), scans a subset of channels per type (limited by max_channels) — results may be partial on large workspaces. Results are prioritized: DMs > group DMs > partner channels > internal channels."),
			mcp.WithTitleAnnotation("Get Unread Messages"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithBoolean("include_messages",
				mcp.Description("If true (default), returns the actual unread messages. If false, returns only a summary of channels with unreads."),
				mcp.DefaultBool(true),
			),
			mcp.WithString("channel_types",
				mcp.Description("Filter by channel type: 'all' (default), 'dm' (direct messages), 'group_dm' (group DMs), 'partner' (ext-* channels), 'internal' (other channels)."),
				mcp.DefaultString("all"),
			),
			mcp.WithNumber("max_channels",
				mcp.Description("Maximum number of channels to fetch unreads from. Default is 50."),
				mcp.DefaultNumber(50),
			),
			mcp.WithNumber("max_messages_per_channel",
				mcp.Description("Maximum messages to fetch per channel. Default is 10."),
				mcp.DefaultNumber(10),
			),
			mcp.WithBoolean("mentions_only",
				mcp.Description("If true, only returns channels where you have @mentions. Default is false."),
				mcp.DefaultBool(false),
			),
			mcp.WithBoolean("include_muted",
				mcp.Description("If true, includes muted channels in results. Default is false (muted channels are excluded, matching Slack app behavior)."),
				mcp.DefaultBool(false),
			),
		), conversationsHandler.ConversationsUnreadsHandler)
	}

	// Register mark tool - marks a channel as read
	if shouldAddTool(ToolConversationsMark, enabledTools, "") {
		s.AddTool(mcp.NewTool(ToolConversationsMark,
			mcp.WithDescription("Mark a channel or DM as read. If no timestamp is provided, marks all messages as read."),
			mcp.WithTitleAnnotation("Mark as Read"),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("channel_id",
				mcp.Required(),
				mcp.Description("ID of the channel in format Cxxxxxxxxxx or its name starting with #... or @... (e.g., #general, @username)."),
			),
			mcp.WithString("ts",
				mcp.Description("Timestamp of the message to mark as read up to. If not provided, marks all messages as read."),
			),
		), conversationsHandler.ConversationsMarkHandler)
	}

	// User groups tools
	if shouldAddTool(ToolUsergroupsList, enabledTools, "") {
		s.AddTool(mcp.NewTool(ToolUsergroupsList,
			mcp.WithDescription("List all user groups (subteams) in the Slack workspace. User groups are mention groups like @engineering or @design that notify all members. Use this to discover available groups, check group membership counts, or find a group's ID before joining/updating it. Returns CSV with columns: id, name, handle, description, user_count, is_external."),
			mcp.WithTitleAnnotation("List User Groups"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithBoolean("include_users",
				mcp.Description("Include list of user IDs in each group. Default is false."),
				mcp.DefaultBool(false),
			),
			mcp.WithBoolean("include_count",
				mcp.Description("Include user count for each group. Default is true."),
				mcp.DefaultBool(true),
			),
			mcp.WithBoolean("include_disabled",
				mcp.Description("Include disabled/archived groups. Default is false."),
				mcp.DefaultBool(false),
			),
		), usergroupsHandler.UsergroupsListHandler)
	}

	if shouldAddTool(ToolUsergroupsMe, enabledTools, "") {
		s.AddTool(mcp.NewTool(ToolUsergroupsMe,
			mcp.WithDescription("Manage your own user group membership. Use action='list' to see which groups you belong to. Use action='join' with a usergroup_id to add yourself to a group (e.g., to receive @mentions). Use action='leave' with a usergroup_id to remove yourself. This is the easiest way to join/leave groups without needing to know the full member list."),
			mcp.WithTitleAnnotation("My User Groups"),
			mcp.WithString("action",
				mcp.Required(),
				mcp.Description("Action to perform: 'list' returns CSV of groups you're a member of, 'join' adds you to a group, 'leave' removes you from a group."),
			),
			mcp.WithString("usergroup_id",
				mcp.Description("ID of the user group (starts with 'S', e.g., 'S0123456789'). Required for 'join' and 'leave' actions. Get IDs from usergroups_list."),
			),
		), usergroupsHandler.UsergroupsMeHandler)
	}

	if shouldAddTool(ToolUsergroupsCreate, enabledTools, "") {
		s.AddTool(mcp.NewTool(ToolUsergroupsCreate,
			mcp.WithDescription("Create a new user group (mention group) in the Slack workspace. After creation, use usergroups_users_update to add members, or users can join themselves with usergroups_me. The handle becomes the @mention (e.g., handle='engineering' creates @engineering)."),
			mcp.WithTitleAnnotation("Create User Group"),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Display name of the user group (e.g., 'Engineering Team', 'Design Squad')."),
			),
			mcp.WithString("handle",
				mcp.Description("The @mention handle without the @ symbol (e.g., 'engineering' for @engineering). Keep it short and lowercase. If omitted, Slack auto-generates one from the name."),
			),
			mcp.WithString("description",
				mcp.Description("Purpose or description shown in group details (e.g., 'Backend and frontend engineers')."),
			),
			mcp.WithString("channels",
				mcp.Description("Comma-separated channel IDs where this group is commonly mentioned. Members get suggestions to join these channels."),
			),
		), usergroupsHandler.UsergroupsCreateHandler)
	}

	if shouldAddTool(ToolUsergroupsUpdate, enabledTools, "") {
		s.AddTool(mcp.NewTool(ToolUsergroupsUpdate,
			mcp.WithDescription("Update a user group's metadata: name, handle (@mention), description, or default channels. Does NOT change members - use usergroups_users_update for that. At least one field must be provided."),
			mcp.WithTitleAnnotation("Update User Group"),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithString("usergroup_id",
				mcp.Required(),
				mcp.Description("ID of the user group to update (starts with 'S', e.g., 'S0123456789'). Get IDs from usergroups_list."),
			),
			mcp.WithString("name",
				mcp.Description("New display name for the group."),
			),
			mcp.WithString("handle",
				mcp.Description("New @mention handle (without @). Changing this changes how users mention the group."),
			),
			mcp.WithString("description",
				mcp.Description("New description for the group."),
			),
			mcp.WithString("channels",
				mcp.Description("New default channel IDs (comma-separated). Replaces existing default channels."),
			),
		), usergroupsHandler.UsergroupsUpdateHandler)
	}

	if shouldAddTool(ToolUsergroupsUsersUpdate, enabledTools, "") {
		s.AddTool(mcp.NewTool(ToolUsergroupsUsersUpdate,
			mcp.WithDescription("Replace all members of a user group with a new list. WARNING: This completely replaces the member list - any user not in the 'users' parameter will be removed. To add/remove just yourself, use usergroups_me instead. To add a single user without removing others, first get current members from usergroups_list with include_users=true, then call this with the combined list."),
			mcp.WithTitleAnnotation("Update User Group Members"),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithString("usergroup_id",
				mcp.Required(),
				mcp.Description("ID of the user group (starts with 'S', e.g., 'S0123456789'). Get IDs from usergroups_list."),
			),
			mcp.WithString("users",
				mcp.Required(),
				mcp.Description("Comma-separated user IDs that will become the COMPLETE member list (e.g., 'U0123456789,U9876543210'). All current members not in this list will be removed."),
			),
		), usergroupsHandler.UsergroupsUsersUpdateHandler)
	}

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

func (s *MCPServer) ServeHTTP(addr string) *server.StreamableHTTPServer {
	s.logger.Info("Creating HTTP server",
		zap.String("context", "console"),
		zap.String("version", version.Version),
		zap.String("build_time", version.BuildTime),
		zap.String("commit_hash", version.CommitHash),
		zap.String("address", addr),
	)
	return server.NewStreamableHTTPServer(s.server,
		server.WithEndpointPath("/mcp"),
		server.WithHTTPContextFunc(func(ctx context.Context, r *http.Request) context.Context {
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

// buildErrorRecoveryMiddleware converts tool handler errors into MCP tool results
// with isError=true, allowing LLMs to see the error and retry with different parameters.
// Without this, errors become JSON-RPC -32603 protocol errors that crash MCP clients.
func buildErrorRecoveryMiddleware(logger *zap.Logger) server.ToolHandlerMiddleware {
	return func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			res, err := next(ctx, req)
			if err != nil {
				logger.Warn("Tool call returned error, converting to isError tool result",
					zap.String("tool", req.Params.Name),
					zap.Error(err),
				)
				return mcp.NewToolResultError(err.Error()), nil
			}
			return res, nil
		}
	}
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
