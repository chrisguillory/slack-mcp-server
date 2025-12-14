package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/gocarina/gocsv"
	"github.com/korotovsky/slack-mcp-server/pkg/provider"
	"github.com/korotovsky/slack-mcp-server/pkg/text"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/slack-go/slack"
	"go.uber.org/zap"
)

type ChatHandler struct {
	apiProvider *provider.ApiProvider
	logger      *zap.Logger
}

func NewChatHandler(apiProvider *provider.ApiProvider, logger *zap.Logger) *ChatHandler {
	return &ChatHandler{
		apiProvider: apiProvider,
		logger:      logger,
	}
}

type addMessageParams struct {
	channel    string
	threadTs   string
	text       string
	blocksJSON string // Raw JSON array of Block Kit blocks
}

// ChatPostMessageHandler posts a message and returns it as CSV
func (ch *ChatHandler) ChatPostMessageHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ch.logger.Debug("ChatPostMessageHandler called", zap.Any("params", request.Params))

	params, err := ch.parseParamsToolAddMessage(request)
	if err != nil {
		ch.logger.Error("Failed to parse add-message params", zap.Error(err))
		return mcp.NewToolResultErrorFromErr("Failed to parse message parameters", err), nil
	}

	var options []slack.MsgOption
	// Add reply_broadcast support
	if params.threadTs != "" {
		options = append(options, slack.MsgOptionTS(params.threadTs))

		// Check if reply should be broadcast to channel
		replyBroadcast := request.GetBool("reply_broadcast", false)
		if replyBroadcast {
			options = append(options, slack.MsgOptionBroadcast())
		}
	}

	// Add text if provided (also serves as fallback when blocks present)
	if params.text != "" {
		options = append(options, slack.MsgOptionText(params.text, false))
	}

	// Add blocks if provided
	if params.blocksJSON != "" {
		var blocks slack.Blocks
		if err := json.Unmarshal([]byte(params.blocksJSON), &blocks); err != nil {
			ch.logger.Error("Failed to parse blocks JSON", zap.Error(err))
			return mcp.NewToolResultErrorFromErr("Failed to parse blocks JSON", err), nil
		}
		options = append(options, slack.MsgOptionBlocks(blocks.BlockSet...))
	}

	unfurlOpt := os.Getenv("SLACK_MCP_ADD_MESSAGE_UNFURLING")
	if text.IsUnfurlingEnabled(params.text, unfurlOpt, ch.logger) {
		options = append(options, slack.MsgOptionEnableLinkUnfurl())
	} else {
		options = append(options, slack.MsgOptionDisableLinkUnfurl())
		options = append(options, slack.MsgOptionDisableMediaUnfurl())
	}

	ch.logger.Debug("Posting Slack message",
		zap.String("channel", params.channel),
		zap.String("thread_ts", params.threadTs),
		zap.Bool("has_blocks", params.blocksJSON != ""),
	)
	respChannel, respTimestamp, err := ch.apiProvider.Slack().PostMessageContext(ctx, params.channel, options...)
	if err != nil {
		ch.logger.Error("Slack PostMessageContext failed", zap.Error(err))
		return mcp.NewToolResultErrorFromErr("Failed to post message", err), nil
	}

	toolConfig := os.Getenv("SLACK_MCP_ADD_MESSAGE_MARK")
	if toolConfig == "1" || toolConfig == "true" || toolConfig == "yes" {
		err := ch.apiProvider.Slack().MarkConversationContext(ctx, params.channel, respTimestamp)
		if err != nil {
			ch.logger.Error("Slack MarkConversationContext failed", zap.Error(err))
			return mcp.NewToolResultErrorFromErr("Failed to mark conversation as read", err), nil
		}
	}

	// fetch the single message we just posted
	historyParams := slack.GetConversationHistoryParameters{
		ChannelID: respChannel,
		Limit:     1,
		Oldest:    respTimestamp,
		Latest:    respTimestamp,
		Inclusive: true,
	}
	history, err := ch.apiProvider.Slack().GetConversationHistoryContext(ctx, &historyParams)
	if err != nil {
		ch.logger.Error("GetConversationHistoryContext failed", zap.Error(err))
		return mcp.NewToolResultErrorFromErr("Failed to fetch posted message", err), nil
	}
	ch.logger.Debug("Fetched conversation history", zap.Int("message_count", len(history.Messages)))

	messages := ch.convertMessagesFromHistory(history.Messages, historyParams.ChannelID, false)
	return marshalMessagesToCSV(messages)
}

func (ch *ChatHandler) parseParamsToolAddMessage(request mcp.CallToolRequest) (*addMessageParams, error) {
	toolConfig := os.Getenv("SLACK_MCP_ADD_MESSAGE_TOOL")
	if toolConfig == "" {
		ch.logger.Error("Add-message tool disabled by default")
		return nil, errors.New(
			"by default, the post_message tool is disabled to guard Slack workspaces against accidental spamming." +
				"To enable it, set the SLACK_MCP_ADD_MESSAGE_TOOL environment variable to true, 1, or comma separated list of channels" +
				"to limit where the MCP can post messages, e.g. 'SLACK_MCP_ADD_MESSAGE_TOOL=C1234567890,D0987654321', 'SLACK_MCP_ADD_MESSAGE_TOOL=!C1234567890'" +
				"to enable all except one or 'SLACK_MCP_ADD_MESSAGE_TOOL=true' for all channels and DMs",
		)
	}

	channel := request.GetString("channel_id", "")
	if channel == "" {
		ch.logger.Error("channel_id missing in add-message params")
		return nil, errors.New("channel_id must be a string")
	}
	if strings.HasPrefix(channel, "#") || strings.HasPrefix(channel, "@") {
		channelsMaps := ch.apiProvider.ProvideChannelsMaps()
		chn, ok := channelsMaps.ChannelsInv[channel]
		if !ok {
			ch.logger.Error("Channel not found", zap.String("channel", channel))
			return nil, fmt.Errorf("channel %q not found", channel)
		}
		channel = channelsMaps.Channels[chn].ID
	}
	if !isChannelAllowed(channel) {
		ch.logger.Warn("Add-message tool not allowed for channel", zap.String("channel", channel), zap.String("policy", toolConfig))
		return nil, fmt.Errorf("post_message tool is not allowed for channel %q, applied policy: %s", channel, toolConfig)
	}

	threadTs := request.GetString("thread_ts", "")
	if threadTs != "" && !strings.Contains(threadTs, ".") {
		ch.logger.Error("Invalid thread_ts format", zap.String("thread_ts", threadTs))
		return nil, errors.New("thread_ts must be a valid timestamp in format 1234567890.123456")
	}

	// Get text (renamed from "payload")
	msgText := request.GetString("text", "")

	// Get blocks JSON string
	blocksJSON := request.GetString("blocks", "")

	// Validate blocks JSON if provided
	if blocksJSON != "" {
		var rawBlocks []json.RawMessage
		if err := json.Unmarshal([]byte(blocksJSON), &rawBlocks); err != nil {
			ch.logger.Error("Invalid blocks JSON", zap.Error(err))
			return nil, fmt.Errorf("invalid blocks JSON: %w", err)
		}
		if len(rawBlocks) > 50 {
			ch.logger.Error("Too many blocks", zap.Int("count", len(rawBlocks)))
			return nil, errors.New("blocks array exceeds maximum of 50 blocks")
		}
	}

	// Require at least text or blocks
	if msgText == "" && blocksJSON == "" {
		ch.logger.Error("Neither text nor blocks provided")
		return nil, errors.New("either text or blocks must be provided")
	}

	return &addMessageParams{
		channel:    channel,
		threadTs:   threadTs,
		text:       msgText,
		blocksJSON: blocksJSON,
	}, nil
}

// ChatDeleteMessageHandler deletes a message from a channel
func (ch *ChatHandler) ChatDeleteMessageHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ch.logger.Debug("ChatDeleteMessageHandler called", zap.Any("params", request.Params))

	// Check if delete tool is enabled
	toolConfig := os.Getenv("SLACK_MCP_DELETE_MESSAGE_TOOL")
	if toolConfig == "" {
		ch.logger.Error("Delete-message tool disabled by default")
		return mcp.NewToolResultError(
			"by default, the delete_message tool is disabled to prevent accidental message deletion. " +
				"To enable it, set the SLACK_MCP_DELETE_MESSAGE_TOOL environment variable to true, 1, or comma separated list of channels " +
				"to limit where the MCP can delete messages, e.g. 'SLACK_MCP_DELETE_MESSAGE_TOOL=C1234567890,D0987654321', " +
				"'SLACK_MCP_DELETE_MESSAGE_TOOL=!C1234567890' to enable all except one or 'SLACK_MCP_DELETE_MESSAGE_TOOL=true' for all channels and DMs",
		), nil
	}

	// Get and validate channel_id
	channel := request.GetString("channel_id", "")
	if channel == "" {
		ch.logger.Error("channel_id missing in delete-message params")
		return mcp.NewToolResultError("channel_id must be provided"), nil
	}

	// Handle channel name resolution
	if strings.HasPrefix(channel, "#") || strings.HasPrefix(channel, "@") {
		channelsMaps := ch.apiProvider.ProvideChannelsMaps()
		chn, ok := channelsMaps.ChannelsInv[channel]
		if !ok {
			ch.logger.Error("Channel not found", zap.String("channel", channel))
			return mcp.NewToolResultError(fmt.Sprintf("channel %q not found", channel)), nil
		}
		channel = channelsMaps.Channels[chn].ID
	}

	// Check if channel is allowed for deletion
	if !isChannelAllowedForDeletion(channel) {
		ch.logger.Warn("Delete-message tool not allowed for channel", zap.String("channel", channel), zap.String("policy", toolConfig))
		return mcp.NewToolResultError(fmt.Sprintf("delete_message tool is not allowed for channel %q, applied policy: %s", channel, toolConfig)), nil
	}

	// Get and validate timestamp
	timestamp := request.GetString("timestamp", "")
	if timestamp == "" {
		ch.logger.Error("timestamp missing in delete-message params")
		return mcp.NewToolResultError("timestamp must be provided"), nil
	}
	if !strings.Contains(timestamp, ".") {
		ch.logger.Error("Invalid timestamp format", zap.String("timestamp", timestamp))
		return mcp.NewToolResultError("timestamp must be a valid timestamp in format 1234567890.123456"), nil
	}

	// Delete the message
	ch.logger.Debug("Deleting Slack message",
		zap.String("channel", channel),
		zap.String("timestamp", timestamp),
	)

	respChannel, respTimestamp, err := ch.apiProvider.Slack().DeleteMessageContext(ctx, channel, timestamp)
	if err != nil {
		ch.logger.Error("Slack DeleteMessageContext failed", zap.Error(err))
		return mcp.NewToolResultErrorFromErr("Failed to delete message", err), nil
	}

	// Return a simple success message in CSV format
	type DeleteResult struct {
		Channel   string `csv:"Channel"`
		Timestamp string `csv:"Timestamp"`
		Status    string `csv:"Status"`
	}

	result := []DeleteResult{{
		Channel:   respChannel,
		Timestamp: respTimestamp,
		Status:    "deleted",
	}}

	// Marshal to CSV
	csvBytes, err := gocsv.MarshalBytes(result)
	if err != nil {
		ch.logger.Error("Failed to marshal delete result to CSV", zap.Error(err))
		return mcp.NewToolResultErrorFromErr("Failed to format delete result", err), nil
	}

	return mcp.NewToolResultText(string(csvBytes)), nil
}

// ChatUpdateHandler updates an existing message and returns it as CSV
func (ch *ChatHandler) ChatUpdateHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ch.logger.Debug("ChatUpdateHandler called", zap.Any("params", request.Params))

	// Check if update-message tool is enabled
	toolConfig := os.Getenv("SLACK_MCP_UPDATE_MESSAGE_TOOL")
	if toolConfig == "" {
		ch.logger.Error("Update-message tool disabled by default")
		return mcp.NewToolResultError(
			"by default, the update_message tool is disabled to prevent accidental message modification. " +
				"To enable it, set the SLACK_MCP_UPDATE_MESSAGE_TOOL environment variable to true, 1, or comma separated list of channels " +
				"to limit where the MCP can update messages, e.g. 'SLACK_MCP_UPDATE_MESSAGE_TOOL=C1234567890,D0987654321', " +
				"'SLACK_MCP_UPDATE_MESSAGE_TOOL=!C1234567890' to enable all except one or 'SLACK_MCP_UPDATE_MESSAGE_TOOL=true' for all channels and DMs",
		), nil
	}

	// Get and validate channel_id
	channel := request.GetString("channel_id", "")
	if channel == "" {
		ch.logger.Error("channel_id missing in update-message params")
		return mcp.NewToolResultError("channel_id must be provided"), nil
	}

	// Handle channel name resolution and user ID to DM conversation ID conversion
	if strings.HasPrefix(channel, "#") || strings.HasPrefix(channel, "@") {
		channelsMaps := ch.apiProvider.ProvideChannelsMaps()
		chn, ok := channelsMaps.ChannelsInv[channel]
		if !ok {
			ch.logger.Error("Channel not found", zap.String("channel", channel))
			return mcp.NewToolResultError(fmt.Sprintf("channel %q not found", channel)), nil
		}
		channel = channelsMaps.Channels[chn].ID
	} else if strings.HasPrefix(channel, "U") {
		// User IDs are not valid for message updates - need DM conversation ID
		ch.logger.Error("Invalid channel format - user ID provided instead of DM conversation ID",
			zap.String("user_id", channel))
		return mcp.NewToolResultError(
			fmt.Sprintf("Invalid channel_id format: %s appears to be a user ID. "+
				"To update messages in DMs, use the DM conversation ID (starts with 'D') "+
				"instead of the user ID (starts with 'U'). "+
				"You can find the DM conversation ID in the channel_id field of messages from that DM.",
				channel),
		), nil
	}

	// Check if channel is allowed for updates
	if !isChannelAllowedForUpdate(channel) {
		ch.logger.Warn("Update-message tool not allowed for channel", zap.String("channel", channel), zap.String("policy", toolConfig))
		return mcp.NewToolResultError(fmt.Sprintf("update_message tool is not allowed for channel %q, applied policy: %s", channel, toolConfig)), nil
	}

	// Get and validate timestamp
	timestamp := request.GetString("timestamp", "")
	if timestamp == "" {
		ch.logger.Error("timestamp missing in update-message params")
		return mcp.NewToolResultError("timestamp must be provided"), nil
	}
	if !strings.Contains(timestamp, ".") {
		ch.logger.Error("Invalid timestamp format", zap.String("timestamp", timestamp))
		return mcp.NewToolResultError("timestamp must be a valid timestamp in format 1234567890.123456"), nil
	}

	// Get text (renamed from "payload")
	msgText := request.GetString("text", "")

	// Get blocks JSON string
	blocksJSON := request.GetString("blocks", "")

	// Validate blocks JSON if provided
	if blocksJSON != "" {
		var rawBlocks []json.RawMessage
		if err := json.Unmarshal([]byte(blocksJSON), &rawBlocks); err != nil {
			ch.logger.Error("Invalid blocks JSON", zap.Error(err))
			return mcp.NewToolResultError(fmt.Sprintf("invalid blocks JSON: %v", err)), nil
		}
		if len(rawBlocks) > 50 {
			ch.logger.Error("Too many blocks", zap.Int("count", len(rawBlocks)))
			return mcp.NewToolResultError("blocks array exceeds maximum of 50 blocks"), nil
		}
	}

	// Require at least text or blocks
	if msgText == "" && blocksJSON == "" {
		ch.logger.Error("Neither text nor blocks provided")
		return mcp.NewToolResultError("either text or blocks must be provided"), nil
	}

	var options []slack.MsgOption

	// Add text if provided (also serves as fallback when blocks present)
	if msgText != "" {
		options = append(options, slack.MsgOptionText(msgText, false))
	}

	// Add blocks if provided
	if blocksJSON != "" {
		var blocks slack.Blocks
		if err := json.Unmarshal([]byte(blocksJSON), &blocks); err != nil {
			ch.logger.Error("Failed to parse blocks JSON", zap.Error(err))
			return mcp.NewToolResultError(fmt.Sprintf("failed to parse blocks JSON: %v", err)), nil
		}
		options = append(options, slack.MsgOptionBlocks(blocks.BlockSet...))
	}

	// Update the message
	ch.logger.Debug("Updating Slack message",
		zap.String("channel", channel),
		zap.String("timestamp", timestamp),
		zap.Bool("has_blocks", blocksJSON != ""),
	)

	respChannel, respTimestamp, _, err := ch.apiProvider.Slack().UpdateMessageContext(ctx, channel, timestamp, options...)
	if err != nil {
		ch.logger.Error("Slack UpdateMessageContext failed", zap.Error(err))
		return mcp.NewToolResultErrorFromErr("Failed to update message", err), nil
	}

	// Fetch the updated message to return it
	historyParams := slack.GetConversationHistoryParameters{
		ChannelID: respChannel,
		Latest:    respTimestamp,
		Limit:     1,
		Inclusive: true,
	}
	history, err := ch.apiProvider.Slack().GetConversationHistoryContext(ctx, &historyParams)
	if err != nil {
		ch.logger.Error("GetConversationHistoryContext failed after update", zap.Error(err))
		return mcp.NewToolResultErrorFromErr("Failed to fetch updated message", err), nil
	}

	if len(history.Messages) == 0 {
		ch.logger.Error("No message found after update", zap.String("channel", respChannel), zap.String("timestamp", respTimestamp))
		return mcp.NewToolResultError("Updated message not found"), nil
	}

	// Convert message to CSV format
	messages := ch.convertMessagesFromHistory(history.Messages, respChannel, false)
	csvBytes, err := marshalMessagesToCSVBytes(messages)
	if err != nil {
		ch.logger.Error("Failed to marshal updated message to CSV", zap.Error(err))
		return mcp.NewToolResultErrorFromErr("Failed to format updated message", err), nil
	}

	return mcp.NewToolResultText(string(csvBytes)), nil
}

// ChatPostMessageAsBotHandler posts a message as the bot user (not as the authenticated user)
func (ch *ChatHandler) ChatPostMessageAsBotHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ch.logger.Debug("ChatPostMessageAsBotHandler called", zap.Any("params", request.Params))

	// Check if bot posting is enabled
	if !ch.apiProvider.HasSlackBot() {
		ch.logger.Error("Bot posting not available - SLACK_MCP_BOT_TOKEN not configured")
		return mcp.NewToolResultError(
			"Bot posting is not available. To enable it, set the SLACK_MCP_BOT_TOKEN environment variable " +
				"to a valid Slack bot token (xoxb-...). This token is separate from your user token.",
		), nil
	}

	params, err := ch.parseParamsToolAddMessageAsBot(request)
	if err != nil {
		ch.logger.Error("Failed to parse add-message-as-bot params", zap.Error(err))
		return mcp.NewToolResultErrorFromErr("Failed to parse message parameters", err), nil
	}

	var options []slack.MsgOption
	// Add reply_broadcast support
	if params.threadTs != "" {
		options = append(options, slack.MsgOptionTS(params.threadTs))

		// Check if reply should be broadcast to channel
		replyBroadcast := request.GetBool("reply_broadcast", false)
		if replyBroadcast {
			options = append(options, slack.MsgOptionBroadcast())
		}
	}

	if params.text != "" {
		options = append(options, slack.MsgOptionText(params.text, false))
	}

	if params.blocksJSON != "" {
		var blocks slack.Blocks
		if err := json.Unmarshal([]byte(params.blocksJSON), &blocks); err != nil {
			ch.logger.Error("Failed to parse blocks JSON", zap.Error(err))
			return mcp.NewToolResultErrorFromErr("Failed to parse blocks JSON", err), nil
		}
		options = append(options, slack.MsgOptionBlocks(blocks.BlockSet...))
	}

	// Handle unfurling settings
	unfurlOpt := os.Getenv("SLACK_MCP_ADD_MESSAGE_UNFURLING")
	if text.IsUnfurlingEnabled(params.text, unfurlOpt, ch.logger) {
		options = append(options, slack.MsgOptionEnableLinkUnfurl())
	} else {
		options = append(options, slack.MsgOptionDisableLinkUnfurl())
		options = append(options, slack.MsgOptionDisableMediaUnfurl())
	}

	ch.logger.Debug("Posting Slack message as bot",
		zap.String("channel", params.channel),
		zap.String("thread_ts", params.threadTs),
		zap.Bool("has_blocks", params.blocksJSON != ""),
	)

	// Use the bot client to post
	botClient := ch.apiProvider.SlackBot()
	respChannel, respTimestamp, err := botClient.PostMessageContext(ctx, params.channel, options...)
	if err != nil {
		ch.logger.Error("Slack PostMessageContext (bot) failed", zap.Error(err))
		return mcp.NewToolResultErrorFromErr("Failed to post message as bot", err), nil
	}

	// Optionally mark conversation as read (using regular client, not bot)
	toolConfig := os.Getenv("SLACK_MCP_ADD_MESSAGE_MARK")
	if toolConfig == "1" || toolConfig == "true" || toolConfig == "yes" {
		err := ch.apiProvider.Slack().MarkConversationContext(ctx, params.channel, respTimestamp)
		if err != nil {
			ch.logger.Warn("Slack MarkConversationContext failed (non-fatal)", zap.Error(err))
		}
	}

	// Fetch the posted message to return it
	// Note: We use the regular client here since bot might not have permission to read history
	historyParams := slack.GetConversationHistoryParameters{
		ChannelID: respChannel,
		Limit:     1,
		Oldest:    respTimestamp,
		Latest:    respTimestamp,
		Inclusive: true,
	}
	history, err := ch.apiProvider.Slack().GetConversationHistoryContext(ctx, &historyParams)
	if err != nil {
		ch.logger.Warn("GetConversationHistoryContext failed (returning minimal response)", zap.Error(err))
		// Return a minimal success response if we can't fetch the message
		type PostResult struct {
			Channel   string `csv:"Channel"`
			Timestamp string `csv:"Timestamp"`
			Status    string `csv:"Status"`
		}
		result := []PostResult{{
			Channel:   respChannel,
			Timestamp: respTimestamp,
			Status:    "posted_as_bot",
		}}
		csvBytes, _ := gocsv.MarshalBytes(result)
		return mcp.NewToolResultText(string(csvBytes)), nil
	}

	messages := ch.convertMessagesFromHistory(history.Messages, historyParams.ChannelID, false)
	return marshalMessagesToCSV(messages)
}

// parseParamsToolAddMessageAsBot parses parameters for bot posting
// Uses separate env var SLACK_MCP_BOT_MESSAGE_TOOL for access control
func (ch *ChatHandler) parseParamsToolAddMessageAsBot(request mcp.CallToolRequest) (*addMessageParams, error) {
	// Check bot-specific tool config, fallback to regular message tool config
	toolConfig := os.Getenv("SLACK_MCP_BOT_MESSAGE_TOOL")
	if toolConfig == "" {
		toolConfig = os.Getenv("SLACK_MCP_ADD_MESSAGE_TOOL")
	}

	if toolConfig == "" {
		ch.logger.Error("Bot message tool disabled by default")
		return nil, errors.New(
			"by default, the post_message_as_bot tool is disabled to guard Slack workspaces against accidental spamming. " +
				"To enable it, set the SLACK_MCP_BOT_MESSAGE_TOOL (or SLACK_MCP_ADD_MESSAGE_TOOL) environment variable to true, 1, " +
				"or comma separated list of channels to limit where the MCP can post bot messages.",
		)
	}

	channel := request.GetString("channel_id", "")
	if channel == "" {
		ch.logger.Error("channel_id missing in add-message-as-bot params")
		return nil, errors.New("channel_id must be a string")
	}

	// Handle channel name resolution
	if strings.HasPrefix(channel, "#") || strings.HasPrefix(channel, "@") {
		channelsMaps := ch.apiProvider.ProvideChannelsMaps()
		chn, ok := channelsMaps.ChannelsInv[channel]
		if !ok {
			ch.logger.Error("Channel not found", zap.String("channel", channel))
			return nil, fmt.Errorf("channel %q not found", channel)
		}
		channel = channelsMaps.Channels[chn].ID
	}

	// Check channel allowlist using bot-specific config
	if !isChannelAllowedForBot(channel) {
		ch.logger.Warn("Bot message tool not allowed for channel",
			zap.String("channel", channel),
			zap.String("policy", toolConfig))
		return nil, fmt.Errorf("post_message_as_bot tool is not allowed for channel %q, applied policy: %s", channel, toolConfig)
	}

	threadTs := request.GetString("thread_ts", "")
	if threadTs != "" && !strings.Contains(threadTs, ".") {
		ch.logger.Error("Invalid thread_ts format", zap.String("thread_ts", threadTs))
		return nil, errors.New("thread_ts must be a valid timestamp in format 1234567890.123456")
	}

	msgText := request.GetString("text", "")
	blocksJSON := request.GetString("blocks", "")

	// Validate blocks JSON if provided
	if blocksJSON != "" {
		var rawBlocks []json.RawMessage
		if err := json.Unmarshal([]byte(blocksJSON), &rawBlocks); err != nil {
			ch.logger.Error("Invalid blocks JSON", zap.Error(err))
			return nil, fmt.Errorf("invalid blocks JSON: %w", err)
		}
		if len(rawBlocks) > 50 {
			ch.logger.Error("Too many blocks", zap.Int("count", len(rawBlocks)))
			return nil, errors.New("blocks array exceeds maximum of 50 blocks")
		}
	}

	// Require at least text or blocks
	if msgText == "" && blocksJSON == "" {
		ch.logger.Error("Neither text nor blocks provided")
		return nil, errors.New("either text or blocks must be provided")
	}

	return &addMessageParams{
		channel:    channel,
		threadTs:   threadTs,
		text:       msgText,
		blocksJSON: blocksJSON,
	}, nil
}

// isChannelAllowedForBot checks if bot posting is allowed for a channel
func isChannelAllowedForBot(channel string) bool {
	// First check bot-specific config
	config := os.Getenv("SLACK_MCP_BOT_MESSAGE_TOOL")
	if config == "" {
		// Fall back to regular message tool config
		config = os.Getenv("SLACK_MCP_ADD_MESSAGE_TOOL")
	}

	if config == "" {
		return false
	}
	if config == "true" || config == "1" {
		return true
	}

	items := strings.Split(config, ",")
	isNegated := strings.HasPrefix(strings.TrimSpace(items[0]), "!")
	for _, item := range items {
		item = strings.TrimSpace(item)
		if isNegated {
			if strings.TrimPrefix(item, "!") == channel {
				return false
			}
		} else {
			if item == channel {
				return true
			}
		}
	}
	return isNegated
}

func isChannelAllowedForUpdate(channel string) bool {
	config := os.Getenv("SLACK_MCP_UPDATE_MESSAGE_TOOL")
	if config == "" {
		return false
	}
	if config == "true" || config == "1" {
		return true
	}
	items := strings.Split(config, ",")
	isNegated := strings.HasPrefix(strings.TrimSpace(items[0]), "!")
	for _, item := range items {
		item = strings.TrimSpace(item)
		if isNegated {
			item = strings.TrimPrefix(item, "!")
			if item == channel {
				return false
			}
		} else {
			if item == channel {
				return true
			}
		}
	}
	return isNegated
}

func isChannelAllowed(channel string) bool {
	config := os.Getenv("SLACK_MCP_ADD_MESSAGE_TOOL")
	if config == "" || config == "true" || config == "1" {
		return true
	}
	items := strings.Split(config, ",")
	isNegated := strings.HasPrefix(strings.TrimSpace(items[0]), "!")
	for _, item := range items {
		item = strings.TrimSpace(item)
		if isNegated {
			if strings.TrimPrefix(item, "!") == channel {
				return false
			}
		} else {
			if item == channel {
				return true
			}
		}
	}
	return !isNegated
}

func isChannelAllowedForDeletion(channel string) bool {
	config := os.Getenv("SLACK_MCP_DELETE_MESSAGE_TOOL")
	if config == "" {
		return false // Default to disabled
	}
	if config == "true" || config == "1" {
		return true
	}
	items := strings.Split(config, ",")
	isNegated := strings.HasPrefix(strings.TrimSpace(items[0]), "!")
	for _, item := range items {
		item = strings.TrimSpace(item)
		if isNegated {
			if strings.TrimPrefix(item, "!") == channel {
				return false
			}
		} else {
			if item == channel {
				return true
			}
		}
	}
	return !isNegated
}

func (ch *ChatHandler) convertMessagesFromHistory(slackMessages []slack.Message, channel string, includeActivity bool) []Message {
	usersMap := ch.apiProvider.ProvideUsersMap()
	var messages []Message
	warn := false

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

		// Start with the message's user field
		userID := msg.User
		userName, realName, ok := getUserInfo(msg.User, usersMap.Users)

		if !ok && msg.SubType == "bot_message" {
			// Bot messages should have BotID, not Username
			if msg.Username != "" {
				// Unexpected: bot message has Username set
				ch.logger.Error("UNEXPECTED: Bot message has Username in chat",
					zap.String("username", msg.Username),
					zap.String("bot_id", msg.BotID),
					zap.String("timestamp", msg.Timestamp))
				panic(fmt.Sprintf("Bot message has unexpected Username: %s (BotID: %s)", msg.Username, msg.BotID))
			}
			if msg.BotID == "" {
				// This should never happen for bot messages
				ch.logger.Error("CRITICAL: Bot message missing BotID in chat",
					zap.String("timestamp", msg.Timestamp))
				panic("Bot message missing BotID")
			}

			// Resolve bot to user
			botUser, found := getBotInfo(msg.BotID, ch.apiProvider)
			if found {
				userID = botUser.ID
				userName = botUser.Name
				realName = botUser.RealName
				ok = true
			} else {
				// Fallback: use bot ID for all fields for traceability
				userID = msg.BotID
				userName = msg.BotID
				realName = msg.BotID
				ok = true
			}
		}

		if !ok {
			warn = true
		}

		timestamp, err := text.TimestampToIsoRFC3339(msg.Timestamp)
		if err != nil {
			ch.logger.Error("Failed to convert timestamp to RFC3339", zap.Error(err))
			continue
		}

		msgText := msg.Text + text.AttachmentsTo2CSV(msg.Text, msg.Attachments) + text.BlocksToText(msg.Blocks)

		var reactionParts []string
		for _, r := range msg.Reactions {
			// Include user IDs who reacted: emoji:count:user1,user2
			userList := strings.Join(r.Users, ",")
			reactionParts = append(reactionParts, fmt.Sprintf("%s:%d:%s", r.Name, r.Count, userList))
		}
		reactionsString := strings.Join(reactionParts, "|")

		messages = append(messages, Message{
			MsgID:     msg.Timestamp,
			UserID:    userID,
			UserName:  userName,
			RealName:  realName,
			Text:      text.ProcessText(msgText),
			Channel:   channel,
			ThreadTs:  msg.ThreadTimestamp,
			Time:      timestamp,
			Reactions: reactionsString,
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

// GetSlackTemplatesHandler returns the curated Block Kit templates from SLACK_TEMPLATES.md
func (ch *ChatHandler) GetSlackTemplatesHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ch.logger.Debug("GetSlackTemplatesHandler called")

	// Try multiple paths to find SLACK_TEMPLATES.md
	paths := []string{
		"SLACK_TEMPLATES.md",                                    // Current directory
		"../SLACK_TEMPLATES.md",                                 // Parent directory
		"../../SLACK_TEMPLATES.md",                              // Two levels up
		"/Users/chris/slack-mcp-server/SLACK_TEMPLATES.md",      // Absolute path (fallback)
	}

	var content []byte
	var err error
	var foundPath string

	for _, path := range paths {
		content, err = os.ReadFile(path)
		if err == nil {
			foundPath = path
			break
		}
	}

	if err != nil {
		ch.logger.Error("Failed to read SLACK_TEMPLATES.md from any path", zap.Error(err))
		return mcp.NewToolResultError("Failed to read templates file. Ensure SLACK_TEMPLATES.md exists in the project root."), nil
	}

	ch.logger.Debug("Successfully read SLACK_TEMPLATES.md",
		zap.String("path", foundPath),
		zap.Int("size_bytes", len(content)))
	return mcp.NewToolResultText(string(content)), nil
}
