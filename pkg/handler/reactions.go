package handler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/korotovsky/slack-mcp-server/pkg/provider"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/slack-go/slack"
	"go.uber.org/zap"
)

type ReactionsHandler struct {
	apiProvider *provider.ApiProvider
	logger      *zap.Logger
}

func NewReactionsHandler(apiProvider *provider.ApiProvider, logger *zap.Logger) *ReactionsHandler {
	return &ReactionsHandler{
		apiProvider: apiProvider,
		logger:      logger,
	}
}

type reactionParams struct {
	channelID string
	timestamp string
	emoji     string
}

// Add reaction handler
func (rh *ReactionsHandler) ReactionsAddHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if rh.logger != nil {
		rh.logger.Debug("ReactionsAddHandler called", zap.Any("params", request.Params))
	}

	params, err := rh.parseReactionParams(request)
	if err != nil {
		if rh.logger != nil {
			rh.logger.Error("Failed to parse add-reaction params", zap.Error(err))
		}
		return mcp.NewToolResultErrorFromErr("Failed to parse reaction parameters", err), nil
	}

	// Check if reactions are enabled
	if !rh.isReactionAllowed(params.channelID) {
		return mcp.NewToolResultError("reaction tools are disabled for this channel. Set SLACK_MCP_ADD_REACTION_TOOL environment variable to enable."), nil
	}

	// Create Slack item reference
	item := slack.NewRefToMessage(params.channelID, params.timestamp)

	if rh.logger != nil {
		rh.logger.Debug("Adding Slack reaction",
			zap.String("channel", params.channelID),
			zap.String("timestamp", params.timestamp),
			zap.String("emoji", params.emoji),
		)
	}

	// Add reaction (works with both auth types)
	if err := rh.apiProvider.Slack().AddReactionContext(ctx, params.emoji, item); err != nil {
		if !strings.Contains(err.Error(), "already_reacted") {
			if rh.logger != nil {
				rh.logger.Error("Slack AddReactionContext failed", zap.Error(err))
			}
			return mcp.NewToolResultErrorFromErr("Failed to add reaction", err), nil
		}
		// Log but continue if already reacted
		if rh.logger != nil {
			rh.logger.Debug("Reaction already exists",
				zap.String("emoji", params.emoji),
				zap.String("channel", params.channelID),
				zap.String("timestamp", params.timestamp))
		}
	}

	// Fetch updated message to return (same pattern as add message)
	historyParams := slack.GetConversationHistoryParameters{
		ChannelID: params.channelID,
		Limit:     1,
		Oldest:    params.timestamp,
		Latest:    params.timestamp,
		Inclusive: true,
	}

	history, err := rh.apiProvider.Slack().GetConversationHistoryContext(ctx, &historyParams)
	if err != nil {
		if rh.logger != nil {
			rh.logger.Error("GetConversationHistoryContext failed", zap.Error(err))
		}
		return mcp.NewToolResultErrorFromErr("Failed to fetch message after adding reaction", err), nil
	}
	if rh.logger != nil {
		rh.logger.Debug("Fetched conversation history", zap.Int("message_count", len(history.Messages)))
	}

	if len(history.Messages) == 0 {
		return mcp.NewToolResultError("message not found after adding reaction"), nil
	}

	// Convert and return as CSV (same pattern as add message)
	conversationsHandler := NewConversationsHandler(rh.apiProvider, rh.logger)
	messages := conversationsHandler.convertMessagesFromHistory(history.Messages, params.channelID, false)
	return marshalMessagesToCSV(messages)
}

// Remove reaction handler
func (rh *ReactionsHandler) ReactionsRemoveHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if rh.logger != nil {
		rh.logger.Debug("ReactionsRemoveHandler called", zap.Any("params", request.Params))
	}

	params, err := rh.parseReactionParams(request)
	if err != nil {
		if rh.logger != nil {
			rh.logger.Error("Failed to parse remove-reaction params", zap.Error(err))
		}
		return mcp.NewToolResultErrorFromErr("Failed to parse reaction parameters", err), nil
	}

	// Check if reactions are enabled
	if !rh.isReactionAllowed(params.channelID) {
		return mcp.NewToolResultError("reaction tools are disabled for this channel. Set SLACK_MCP_ADD_REACTION_TOOL environment variable to enable."), nil
	}

	// Create Slack item reference
	item := slack.NewRefToMessage(params.channelID, params.timestamp)

	if rh.logger != nil {
		rh.logger.Debug("Removing Slack reaction",
			zap.String("channel", params.channelID),
			zap.String("timestamp", params.timestamp),
			zap.String("emoji", params.emoji),
		)
	}

	// Remove reaction (works with both auth types)
	if err := rh.apiProvider.Slack().RemoveReactionContext(ctx, params.emoji, item); err != nil {
		if !strings.Contains(err.Error(), "no_reaction") {
			if rh.logger != nil {
				rh.logger.Error("Slack RemoveReactionContext failed", zap.Error(err))
			}
			return mcp.NewToolResultErrorFromErr("Failed to remove reaction", err), nil
		}
		// Log but continue if no reaction exists
		if rh.logger != nil {
			rh.logger.Debug("Reaction doesn't exist",
				zap.String("emoji", params.emoji),
				zap.String("channel", params.channelID),
				zap.String("timestamp", params.timestamp))
		}
	}

	// Fetch updated message to return (same pattern as add message)
	historyParams := slack.GetConversationHistoryParameters{
		ChannelID: params.channelID,
		Limit:     1,
		Oldest:    params.timestamp,
		Latest:    params.timestamp,
		Inclusive: true,
	}

	history, err := rh.apiProvider.Slack().GetConversationHistoryContext(ctx, &historyParams)
	if err != nil {
		if rh.logger != nil {
			rh.logger.Error("GetConversationHistoryContext failed", zap.Error(err))
		}
		return mcp.NewToolResultErrorFromErr("Failed to fetch message after adding reaction", err), nil
	}
	if rh.logger != nil {
		rh.logger.Debug("Fetched conversation history", zap.Int("message_count", len(history.Messages)))
	}

	if len(history.Messages) == 0 {
		return mcp.NewToolResultError("message not found after removing reaction"), nil
	}

	// Convert and return as CSV (same pattern as add message)
	conversationsHandler := NewConversationsHandler(rh.apiProvider, rh.logger)
	messages := conversationsHandler.convertMessagesFromHistory(history.Messages, params.channelID, false)
	return marshalMessagesToCSV(messages)
}

// Parse reaction parameters
func (rh *ReactionsHandler) parseReactionParams(req mcp.CallToolRequest) (*reactionParams, error) {
	channelID := req.GetString("channel_id", "")
	if channelID == "" {
		if rh.logger != nil {
			rh.logger.Error("channel_id missing in add-reaction params")
		}
		return nil, errors.New("channel_id must be a string")
	}

	// Handle channel name resolution (same pattern as add message)
	if strings.HasPrefix(channelID, "#") || strings.HasPrefix(channelID, "@") {
		channelsMaps := rh.apiProvider.ProvideChannelsMaps()
		chn, ok := channelsMaps.ChannelsInv[channelID]
		if !ok {
			if rh.logger != nil {
				rh.logger.Error("Channel not found", zap.String("channel", channelID))
			}
			return nil, fmt.Errorf("channel %q not found", channelID)
		}
		channelID = channelsMaps.Channels[chn].ID
	}

	timestamp := req.GetString("timestamp", "")
	if timestamp == "" {
		if rh.logger != nil {
			rh.logger.Error("timestamp missing in add-reaction params")
		}
		return nil, errors.New("timestamp must be a string")
	}

	// Validate timestamp format (must contain a dot, like 1234567890.123456)
	if !strings.Contains(timestamp, ".") {
		if rh.logger != nil {
			rh.logger.Error("invalid timestamp format", zap.String("timestamp", timestamp))
		}
		return nil, fmt.Errorf("invalid timestamp format: %s (must be like 1234567890.123456)", timestamp)
	}

	emoji := req.GetString("emoji", "")
	if emoji == "" {
		if rh.logger != nil {
			rh.logger.Error("emoji missing in add-reaction params")
		}
		return nil, errors.New("emoji must be a string")
	}

	// Strip colons if present
	emoji = strings.Trim(emoji, ":")

	return &reactionParams{
		channelID: channelID,
		timestamp: timestamp,
		emoji:     emoji,
	}, nil
}

// Check if reactions are allowed for a channel
func (rh *ReactionsHandler) isReactionAllowed(channelID string) bool {
	config := os.Getenv("SLACK_MCP_ADD_REACTION_TOOL")
	// Default to disabled for safety
	if config == "" {
		return false
	}

	// Explicitly enabled for all channels
	if config == "true" || config == "1" {
		return true
	}

	items := strings.Split(config, ",")
	isNegated := strings.HasPrefix(strings.TrimSpace(items[0]), "!")

	for _, item := range items {
		item = strings.TrimSpace(item)
		if isNegated {
			if strings.TrimPrefix(item, "!") == channelID {
				return false
			}
		} else {
			if item == channelID {
				return true
			}
		}
	}
	// If negation list, allow by default; if allowlist, deny by default
	return isNegated
}
