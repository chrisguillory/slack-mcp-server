package handler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/korotovsky/slack-mcp-server/pkg/provider"
	"github.com/korotovsky/slack-mcp-server/pkg/text"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/slack-go/slack"
	slackGoUtil "github.com/takara2314/slack-go-util"
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
	channel     string
	threadTs    string
	text        string
	contentType string
}

// ChatPostMessageHandler posts a message and returns it as CSV
func (ch *ChatHandler) ChatPostMessageHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ch.logger.Debug("ChatPostMessageHandler called", zap.Any("params", request.Params))

	params, err := ch.parseParamsToolAddMessage(request)
	if err != nil {
		ch.logger.Error("Failed to parse add-message params", zap.Error(err))
		return nil, err
	}

	var options []slack.MsgOption
	if params.threadTs != "" {
		options = append(options, slack.MsgOptionTS(params.threadTs))
	}

	switch params.contentType {
	case "text/plain":
		options = append(options, slack.MsgOptionDisableMarkdown())
		options = append(options, slack.MsgOptionText(params.text, false))
	case "text/markdown":
		blocks, err := slackGoUtil.ConvertMarkdownTextToBlocks(params.text)
		if err != nil {
			ch.logger.Warn("Markdown parsing error", zap.Error(err))
			options = append(options, slack.MsgOptionDisableMarkdown())
			options = append(options, slack.MsgOptionText(params.text, false))
		} else {
			options = append(options, slack.MsgOptionBlocks(blocks...))
		}
	default:
		return nil, errors.New("content_type must be either 'text/plain' or 'text/markdown'")
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
		zap.String("content_type", params.contentType),
	)
	respChannel, respTimestamp, err := ch.apiProvider.Slack().PostMessageContext(ctx, params.channel, options...)
	if err != nil {
		ch.logger.Error("Slack PostMessageContext failed", zap.Error(err))
		return nil, err
	}

	toolConfig := os.Getenv("SLACK_MCP_ADD_MESSAGE_MARK")
	if toolConfig == "1" || toolConfig == "true" || toolConfig == "yes" {
		err := ch.apiProvider.Slack().MarkConversationContext(ctx, params.channel, respTimestamp)
		if err != nil {
			ch.logger.Error("Slack MarkConversationContext failed", zap.Error(err))
			return nil, err
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
		return nil, err
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
			"by default, the chat_post_message tool is disabled to guard Slack workspaces against accidental spamming." +
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
		return nil, fmt.Errorf("chat_post_message tool is not allowed for channel %q, applied policy: %s", channel, toolConfig)
	}

	threadTs := request.GetString("thread_ts", "")
	if threadTs != "" && !strings.Contains(threadTs, ".") {
		ch.logger.Error("Invalid thread_ts format", zap.String("thread_ts", threadTs))
		return nil, errors.New("thread_ts must be a valid timestamp in format 1234567890.123456")
	}

	msgText := request.GetString("payload", "")
	if msgText == "" {
		ch.logger.Error("Message text missing")
		return nil, errors.New("text must be a string")
	}

	contentType := request.GetString("content_type", "text/markdown")
	if contentType != "text/plain" && contentType != "text/markdown" {
		ch.logger.Error("Invalid content_type", zap.String("content_type", contentType))
		return nil, errors.New("content_type must be either 'text/plain' or 'text/markdown'")
	}

	return &addMessageParams{
		channel:     channel,
		threadTs:    threadTs,
		text:        msgText,
		contentType: contentType,
	}, nil
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

func (ch *ChatHandler) convertMessagesFromHistory(slackMessages []slack.Message, channel string, includeActivity bool) []Message {
	usersMap := ch.apiProvider.ProvideUsersMap()
	var messages []Message
	warn := false

	for _, msg := range slackMessages {
		if (msg.SubType != "" && msg.SubType != "bot_message") && !includeActivity {
			continue
		}

		userName, realName, ok := getUserInfo(msg.User, usersMap.Users)

		if !ok && msg.SubType == "bot_message" {
			userName, realName, ok = getBotInfo(msg.Username)
		}

		if !ok {
			warn = true
		}

		timestamp, err := text.TimestampToIsoRFC3339(msg.Timestamp)
		if err != nil {
			ch.logger.Error("Failed to convert timestamp to RFC3339", zap.Error(err))
			continue
		}

		msgText := msg.Text + text.AttachmentsTo2CSV(msg.Text, msg.Attachments)

		var reactionParts []string
		for _, r := range msg.Reactions {
			// Include user IDs who reacted: emoji:count:user1,user2
			userList := strings.Join(r.Users, ",")
			reactionParts = append(reactionParts, fmt.Sprintf("%s:%d:%s", r.Name, r.Count, userList))
		}
		reactionsString := strings.Join(reactionParts, "|")

		messages = append(messages, Message{
			MsgID:     msg.Timestamp,
			UserID:    msg.User,
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
