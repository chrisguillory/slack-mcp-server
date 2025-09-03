package handler

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gocarina/gocsv"
	"github.com/korotovsky/slack-mcp-server/pkg/provider"
	"github.com/korotovsky/slack-mcp-server/pkg/text"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/slack-go/slack"
	"go.uber.org/zap"
)

const (
	defaultConversationsNumericLimit    = 50
	defaultConversationsExpressionLimit = "1d"
)

type Message struct {
	MsgID     string `json:"msgID"`
	UserID    string `json:"userID"`
	UserName  string `json:"userUser"`
	RealName  string `json:"realName"`
	Channel   string `json:"channelID"`
	ThreadTs  string `json:"ThreadTs"`
	Text      string `json:"text"`
	Time      string `json:"time"`
	Reactions string `json:"reactions,omitempty"`
	Cursor    string `json:"cursor,omitempty"`
}

type User struct {
	UserID   string `csv:"user_id"`
	UserName string `csv:"user_name"`
	RealName string `csv:"real_name"`
}

type conversationParams struct {
	channel  string
	limit    int
	oldest   string
	latest   string
	cursor   string
	activity bool
}

type ConversationsHandler struct {
	apiProvider *provider.ApiProvider
	logger      *zap.Logger
}

func NewConversationsHandler(apiProvider *provider.ApiProvider, logger *zap.Logger) *ConversationsHandler {
	return &ConversationsHandler{
		apiProvider: apiProvider,
		logger:      logger,
	}
}

// ConversationsHistoryHandler streams conversation history as CSV
func (ch *ConversationsHandler) ConversationsHistoryHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ch.logger.Debug("ConversationsHistoryHandler called", zap.Any("params", request.Params))

	params, err := ch.parseParamsToolConversations(request)
	if err != nil {
		ch.logger.Error("Failed to parse history params", zap.Error(err))
		return mcp.NewToolResultErrorFromErr("Failed to parse conversation parameters", err), nil
	}
	ch.logger.Debug("History params parsed",
		zap.String("channel", params.channel),
		zap.Int("limit", params.limit),
		zap.String("oldest", params.oldest),
		zap.String("latest", params.latest),
		zap.Bool("include_activity", params.activity),
	)

	// Parse fields parameter
	fields := request.GetString("fields", "msgID,userUser,realName,text,time")
	requestedFields := parseMessageFields(fields)
	ch.logger.Debug("Requested fields", zap.Any("fields", requestedFields))

	historyParams := slack.GetConversationHistoryParameters{
		ChannelID: params.channel,
		Limit:     params.limit,
		Oldest:    params.oldest,
		Latest:    params.latest,
		Cursor:    params.cursor,
		Inclusive: false,
	}
	history, err := ch.apiProvider.Slack().GetConversationHistoryContext(ctx, &historyParams)
	if err != nil {
		ch.logger.Error("GetConversationHistoryContext failed", zap.Error(err))
		return mcp.NewToolResultErrorFromErr("Failed to fetch conversation history", err), nil
	}

	ch.logger.Debug("Fetched conversation history", zap.Int("message_count", len(history.Messages)))

	messages := ch.convertMessagesFromHistoryWithFields(history.Messages, params.channel, params.activity, requestedFields)

	if len(messages) > 0 && history.HasMore && requestedFields["cursor"] {
		messages[len(messages)-1].Cursor = history.ResponseMetaData.NextCursor
	}

	// Use field-aware marshaling
	csvBytes, err := marshalMessagesWithFields(messages, requestedFields, true)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("Failed to format messages as CSV", err), nil
	}
	return mcp.NewToolResultText(string(csvBytes)), nil
}

// ConversationsRepliesHandler streams thread replies as CSV
func (ch *ConversationsHandler) ConversationsRepliesHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ch.logger.Debug("ConversationsRepliesHandler called", zap.Any("params", request.Params))

	params, err := ch.parseParamsToolConversations(request)
	if err != nil {
		ch.logger.Error("Failed to parse replies params", zap.Error(err))
		return mcp.NewToolResultErrorFromErr("Failed to parse conversation parameters", err), nil
	}
	threadTs := request.GetString("thread_ts", "")
	if threadTs == "" {
		ch.logger.Error("thread_ts not provided for replies", zap.String("thread_ts", threadTs))
		return mcp.NewToolResultError("thread_ts must be provided"), nil
	}

	// Parse fields parameter
	fields := request.GetString("fields", "msgID,userUser,realName,text,time")
	requestedFields := parseMessageFields(fields)
	ch.logger.Debug("Requested fields", zap.Any("fields", requestedFields))

	repliesParams := slack.GetConversationRepliesParameters{
		ChannelID: params.channel,
		Timestamp: threadTs,
		Limit:     params.limit,
		Oldest:    params.oldest,
		Latest:    params.latest,
		Cursor:    params.cursor,
		Inclusive: false,
	}
	replies, _, _, err := ch.apiProvider.Slack().GetConversationRepliesContext(ctx, &repliesParams)
	if err != nil {
		ch.logger.Error("GetConversationRepliesContext failed", zap.Error(err))
		return mcp.NewToolResultErrorFromErr("Failed to fetch conversation replies", err), nil
	}
	ch.logger.Debug("Fetched conversation replies", zap.Int("count", len(replies)))

	messages := ch.convertMessagesFromHistoryWithFields(replies, params.channel, params.activity, requestedFields)

	// Note: cursor field is not applicable for replies, so we pass false for includeCursor
	// Use field-aware marshaling
	csvBytes, err := marshalMessagesWithFields(messages, requestedFields, false)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("Failed to format messages as CSV", err), nil
	}
	return mcp.NewToolResultText(string(csvBytes)), nil
}

// UsersResource streams a CSV of all users
func (ch *ConversationsHandler) UsersResource(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	ch.logger.Debug("UsersResource called", zap.Any("params", request.Params))

	// collect users
	usersMaps := ch.apiProvider.ProvideUsersMap()
	users := usersMaps.Users
	usersList := make([]User, 0, len(users))
	for _, user := range users {
		usersList = append(usersList, User{
			UserID:   user.ID,
			UserName: user.Name,
			RealName: user.RealName,
		})
	}

	// marshal CSV
	csvBytes, err := gocsv.MarshalBytes(&usersList)
	if err != nil {
		ch.logger.Error("Failed to marshal users to CSV", zap.Error(err))
		return nil, err
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      request.Params.URI,
			MIMEType: "text/csv",
			Text:     string(csvBytes),
		},
	}, nil
}

func (ch *ConversationsHandler) convertMessagesFromHistory(slackMessages []slack.Message, channelID string, includeActivity bool) []Message {
	// Default behavior - include all fields for backwards compatibility
	allFields := make(map[string]bool)
	for _, field := range []string{"msgID", "userID", "userUser", "realName", "channelID", "threadTs", "text", "time", "reactions"} {
		allFields[field] = true
	}
	return ch.convertMessagesFromHistoryWithFields(slackMessages, channelID, includeActivity, allFields)
}

func (ch *ConversationsHandler) convertMessagesFromHistoryWithFields(slackMessages []slack.Message, channelID string, includeActivity bool, fields map[string]bool) []Message {
	var messages []Message
	warn := false

	// Check which fields we need to optimize
	needUserLookup := fields["userUser"] || fields["realName"]
	needReactions := fields["reactions"]
	needText := fields["text"]
	needTime := fields["time"]

	for _, msg := range slackMessages {
		// Skip activity messages unless specifically requested
		// Common message subtypes that should be included:
		// - "" (regular message)
		// - "bot_message" (bot posts)
		// - "thread_broadcast" (thread messages sent to channel)
		// - "me_message" (/me commands)
		// - "file_share" (file uploads)
		isActivityMessage := msg.SubType != "" &&
			msg.SubType != "bot_message" &&
			msg.SubType != "thread_broadcast" &&
			msg.SubType != "me_message" &&
			msg.SubType != "file_share"

		if isActivityMessage && !includeActivity {
			continue
		}

		// Only parse reactions if requested
		var parsedReactions string
		if needReactions {
			parsedReactions = ch.parseReactions(msg.Reactions)
		}

		// Only do user lookups if username or real name requested
		var userName, realName string
		var ok bool
		if needUserLookup {
			userName, realName, ok = getUserInfo(msg.User, ch.apiProvider.ProvideUsersMap().Users)

			if !ok && msg.SubType == "bot_message" {
				userName, realName, ok = getBotInfo(msg.Username)
			}

			if !ok {
				warn = true
			}
		}

		// Only convert timestamp if time field requested
		var timestamp string
		if needTime {
			var err error
			timestamp, err = text.TimestampToIsoRFC3339(msg.Timestamp)
			if err != nil {
				ch.logger.Error("Failed to convert timestamp to RFC3339", zap.Error(err))
				continue
			}
		}

		// Only process text with blocks if text field requested
		var msgText string
		if needText {
			msgText = msg.Text + text.AttachmentsTo2CSV(msg.Text, msg.Attachments) + text.BlocksToText(msg.Blocks)
			msgText = text.ProcessText(msgText)
		}

		messages = append(messages, Message{
			MsgID:     msg.Timestamp,
			UserID:    msg.User,
			UserName:  userName,
			RealName:  realName,
			Text:      msgText,
			Channel:   channelID,
			ThreadTs:  msg.ThreadTimestamp,
			Time:      timestamp,
			Reactions: parsedReactions,
		})
	}

	if ready, err := ch.apiProvider.IsReady(); !ready {
		if warn && errors.Is(err, provider.ErrUsersNotReady) {
			ch.logger.Warn(
				"WARNING: Slack users sync is not ready yet, you may experience some limited functionality and see UIDs instead of resolved names as well as unable to query users by their @handles. Users sync is part of channels sync and operations on channels depend on users collection (IM, MPIM). Please wait until users are synced and try again",
				zap.Error(err),
			)
		}
	}
	return messages
}

func (ch *ConversationsHandler) parseReactions(reactions []slack.ItemReaction) string {
	if len(reactions) == 0 {
		return ""
	}

	var reactionParts []string
	for _, r := range reactions {
		// Include user IDs who reacted: emoji:count:user1,user2
		userList := strings.Join(r.Users, ",")
		reactionParts = append(reactionParts, fmt.Sprintf("%s:%d:%s", r.Name, r.Count, userList))
	}

	return strings.Join(reactionParts, "|")
}

func (ch *ConversationsHandler) parseParamsToolConversations(request mcp.CallToolRequest) (*conversationParams, error) {
	channel := request.GetString("channel_id", "")
	if channel == "" {
		ch.logger.Error("channel_id missing in conversations params")
		return nil, errors.New("channel_id must be provided")
	}

	limit := request.GetString("limit", "")
	cursor := request.GetString("cursor", "")
	activity := request.GetBool("include_activity_messages", false)

	var (
		paramLimit  int
		paramOldest string
		paramLatest string
		err         error
	)
	if strings.HasSuffix(limit, "d") || strings.HasSuffix(limit, "w") || strings.HasSuffix(limit, "m") {
		paramLimit, paramOldest, paramLatest, err = limitByExpression(limit, defaultConversationsExpressionLimit)
		if err != nil {
			ch.logger.Error("Invalid duration limit", zap.String("limit", limit), zap.Error(err))
			return nil, err
		}
	} else if cursor == "" {
		paramLimit, err = limitByNumeric(limit, defaultConversationsNumericLimit)
		if err != nil {
			ch.logger.Error("Invalid numeric limit", zap.String("limit", limit), zap.Error(err))
			return nil, err
		}
	}

	if strings.HasPrefix(channel, "#") || strings.HasPrefix(channel, "@") {
		if ready, err := ch.apiProvider.IsReady(); !ready {
			if errors.Is(err, provider.ErrUsersNotReady) {
				ch.logger.Warn(
					"WARNING: Slack users sync is not ready yet, you may experience some limited functionality and see UIDs instead of resolved names as well as unable to query users by their @handles. Users sync is part of channels sync and operations on channels depend on users collection (IM, MPIM). Please wait until users are synced and try again",
					zap.Error(err),
				)
			}
			if errors.Is(err, provider.ErrChannelsNotReady) {
				ch.logger.Warn(
					"WARNING: Slack channels sync is not ready yet, you may experience some limited functionality and be able to request conversation only by Channel ID, not by its name. Please wait until channels are synced and try again.",
					zap.Error(err),
				)
			}
			return nil, fmt.Errorf("channel %q not found (cache not ready)", channel)
		}
		channelsMaps := ch.apiProvider.ProvideChannelsMaps()
		chn, ok := channelsMaps.ChannelsInv[channel]
		if !ok {
			ch.logger.Error("Channel not found in synced cache", zap.String("channel", channel))
			return nil, fmt.Errorf("channel %q not found. Try removing cache file and restarting MCP Server", channel)
		}
		channel = channelsMaps.Channels[chn].ID
	}

	return &conversationParams{
		channel:  channel,
		limit:    paramLimit,
		oldest:   paramOldest,
		latest:   paramLatest,
		cursor:   cursor,
		activity: activity,
	}, nil
}

func (ch *ConversationsHandler) paramFormatChannel(raw string) (string, error) {
	if raw == "" {
		return "", errors.New("channel is required")
	}

	if strings.HasPrefix(raw, "C") || strings.HasPrefix(raw, "D") || strings.HasPrefix(raw, "G") {
		return raw, nil
	}

	if strings.HasPrefix(raw, "#") {
		channelName := strings.TrimPrefix(raw, "#")
		channelsCache := ch.apiProvider.ProvideChannelsMaps()
		for _, channel := range channelsCache.Channels {
			if channel.Name == channelName {
				return channel.ID, nil
			}
		}
		return "", fmt.Errorf("channel not found: %s", raw)
	}

	if strings.HasPrefix(raw, "@") {
		// DM channels should already be in the channels cache
		channelsCache := ch.apiProvider.ProvideChannelsMaps()
		if channelID, exists := channelsCache.ChannelsInv[raw]; exists {
			return channelsCache.Channels[channelID].ID, nil
		}
		return "", fmt.Errorf("DM channel not found: %s", raw)
	}

	return "", fmt.Errorf("invalid channel format: %s", raw)
}

func limitByNumeric(limit string, defaultLimit int) (int, error) {
	if limit == "" {
		return defaultLimit, nil
	}
	n, err := strconv.Atoi(limit)
	if err != nil {
		return 0, fmt.Errorf("invalid numeric limit: %q", limit)
	}
	return n, nil
}

func limitByExpression(limit, defaultLimit string) (slackLimit int, oldest, latest string, err error) {
	if limit == "" {
		limit = defaultLimit
	}
	if len(limit) < 2 {
		return 0, "", "", fmt.Errorf("invalid duration limit %q: too short", limit)
	}
	suffix := limit[len(limit)-1]
	numStr := limit[:len(limit)-1]
	n, err := strconv.Atoi(numStr)
	if err != nil || n <= 0 {
		return 0, "", "", fmt.Errorf("invalid duration limit %q: must be a positive integer followed by 'd', 'w', or 'm'", limit)
	}
	now := time.Now()
	loc := now.Location()
	startOfToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	var oldestTime time.Time
	switch suffix {
	case 'd':
		oldestTime = startOfToday.AddDate(0, 0, -n+1)
	case 'w':
		oldestTime = startOfToday.AddDate(0, 0, -n*7+1)
	case 'm':
		oldestTime = startOfToday.AddDate(0, -n, 0)
	default:
		return 0, "", "", fmt.Errorf("invalid duration limit %q: must end in 'd', 'w', or 'm'", limit)
	}
	latest = fmt.Sprintf("%d.000000", now.Unix())
	oldest = fmt.Sprintf("%d.000000", oldestTime.Unix())
	return 100, oldest, latest, nil
}
