package server

import (
	"context"
	"fmt"
	"net/http"
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
	server *server.MCPServer
	logger *zap.Logger
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
	), conversationsHandler.ConversationsRepliesHandler)

	s.AddTool(mcp.NewTool("post_message",
		mcp.WithDescription("Post a message to a channel or DM (Slack API: chat.postMessage)"),
		mcp.WithString("channel_id",
			mcp.Required(),
			mcp.Description("ID of the channel in format Cxxxxxxxxxx or its name starting with #... or @... aka #general or @username_dm."),
		),
		mcp.WithString("thread_ts",
			mcp.Description("Unique identifier of either a thread's parent message or a message in the thread_ts must be the timestamp in format 1234567890.123456 of an existing message with 0 or more replies. Optional, if not provided the message will be added to the channel itself, otherwise it will be added to the thread."),
		),
		mcp.WithString("payload",
			mcp.Description("Message payload in specified content_type format. Example: 'Hello, world!' for text/plain or '# Hello, world!' for text/markdown."),
		),
		mcp.WithString("content_type",
			mcp.DefaultString("text/markdown"),
			mcp.Description("Content type of the message. Default is 'text/markdown'. Allowed values: 'text/markdown', 'text/plain'."),
		),
	), chatHandler.ChatPostMessageHandler)

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
		mcp.WithDescription("Edit/update an existing message (Slack API: chat.update)"),
		mcp.WithString("channel_id",
			mcp.Required(),
			mcp.Description("Channel ID (C...) or name (#general, @user_dm)")),
		mcp.WithString("timestamp",
			mcp.Required(),
			mcp.Description("Message timestamp (e.g., 1234567890.123456)")),
		mcp.WithString("payload",
			mcp.Required(),
			mcp.Description("New message content in specified content_type format")),
		mcp.WithString("content_type",
			mcp.Description("Content type of the message. Default is 'text/markdown'. Allowed values: 'text/markdown', 'text/plain'")),
	), chatHandler.ChatUpdateHandler)

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
			mcp.Description("Filter messages with a specific user by their ID or display name in threads and DMs. Example: 'U1234567890' or '@username'. If not provided, all threads and DMs will be searched."),
		),
		mcp.WithString("filter_users_from",
			mcp.Description("Filter messages from a specific user by their ID or display name. Example: 'U1234567890' or '@username'. If not provided, all users will be searched."),
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
			mcp.Description("Comma-separated list of fields to return. Options: 'msgID', 'userID', 'userUser', 'realName', 'channelID', 'threadTs', 'text', 'time', 'reactions', 'permalink'. Use 'all' for all fields. Default excludes 'permalink' for token efficiency."),
		),
		mcp.WithString("sort",
			mcp.DefaultString("relevance"),
			mcp.Description("Sort order for search results. Options: 'relevance' (default, by search score), 'chronological' (oldest first by timestamp). Default: 'relevance'"),
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

	s.AddTool(mcp.NewTool("list_users",
		mcp.WithDescription("List users in the workspace (Slack API: users.list)"),
		mcp.WithString("query",
			mcp.Description("Search for users by name. Searches in username, real name, and display name (case-insensitive)"),
		),
		mcp.WithString("filter",
			mcp.DefaultString("all"),
			mcp.Description("Filter users by status: 'all', 'active', 'deleted', 'bots', 'humans', 'admins'. Default: 'all'"),
		),
		mcp.WithString("fields",
			mcp.DefaultString("id,name,real_name,status"),
			mcp.Description("Comma-separated list of fields to return. Options: 'id', 'name', 'real_name', 'email', 'status', 'is_bot', 'is_admin', 'time_zone', 'title', 'phone'. Use 'all' for all fields. Default: 'id,name,real_name,status'"),
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
		server: s,
		logger: logger,
	}
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
