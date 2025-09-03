package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/csv"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/gocarina/gocsv"
	"github.com/korotovsky/slack-mcp-server/pkg/provider"
	"github.com/korotovsky/slack-mcp-server/pkg/server/auth"
	"github.com/korotovsky/slack-mcp-server/pkg/text"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/slack-go/slack"
	"go.uber.org/zap"
)

type Channel struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Topic       string `json:"topic"`
	Purpose     string `json:"purpose"`
	MemberCount int    `json:"memberCount"`
	Cursor      string `json:"cursor"`
}

type ChannelsHandler struct {
	apiProvider *provider.ApiProvider
	validTypes  map[string]bool
	logger      *zap.Logger
}

func NewChannelsHandler(apiProvider *provider.ApiProvider, logger *zap.Logger) *ChannelsHandler {
	validTypes := make(map[string]bool, len(provider.AllChanTypes))
	for _, v := range provider.AllChanTypes {
		validTypes[v] = true
	}

	return &ChannelsHandler{
		apiProvider: apiProvider,
		validTypes:  validTypes,
		logger:      logger,
	}
}

func (ch *ChannelsHandler) ChannelsResource(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	ch.logger.Debug("ChannelsResource called", zap.Any("params", request.Params))

	// mark3labs/mcp-go does not support middlewares for resources.
	if authenticated, err := auth.IsAuthenticated(ctx, ch.apiProvider.ServerTransport(), ch.logger); !authenticated {
		ch.logger.Error("Authentication failed for channels resource", zap.Error(err))
		return nil, err
	}

	var channelList []Channel

	if ready, err := ch.apiProvider.IsReady(); !ready {
		ch.logger.Error("API provider not ready", zap.Error(err))
		return nil, err
	}

	ar, err := ch.apiProvider.Slack().AuthTest()
	if err != nil {
		ch.logger.Error("Auth test failed", zap.Error(err))
		return nil, err
	}

	ws, err := text.Workspace(ar.URL)
	if err != nil {
		ch.logger.Error("Failed to parse workspace from URL",
			zap.String("url", ar.URL),
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to parse workspace from URL: %v", err)
	}

	channels := ch.apiProvider.ProvideChannelsMaps().Channels
	ch.logger.Debug("Retrieved channels from provider", zap.Int("count", len(channels)))

	for _, channel := range channels {
		channelList = append(channelList, Channel{
			ID:          channel.ID,
			Name:        channel.Name,
			Topic:       channel.Topic,
			Purpose:     channel.Purpose,
			MemberCount: channel.MemberCount,
		})
	}

	csvBytes, err := gocsv.MarshalBytes(&channelList)
	if err != nil {
		ch.logger.Error("Failed to marshal channels to CSV", zap.Error(err))
		return nil, err
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "slack://" + ws + "/channels",
			MIMEType: "text/csv",
			Text:     string(csvBytes),
		},
	}, nil
}

func (ch *ChannelsHandler) ChannelsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ch.logger.Debug("ChannelsHandler called")

	if ready, err := ch.apiProvider.IsReady(); !ready {
		ch.logger.Error("API provider not ready", zap.Error(err))
		return mcp.NewToolResultErrorFromErr("API provider not ready", err), nil
	}

	query := request.GetString("query", "")
	sortType := request.GetString("sort", "popularity")
	types := request.GetString("channel_types", provider.PubChanType)
	cursor := request.GetString("cursor", "")
	// Changed: use 1000 as default instead of 0, since 0 triggers another default later
	limit := request.GetInt("limit", 1000)
	fields := request.GetString("fields", "id,name")
	minMembers := request.GetInt("min_members", 0)

	// Debug: log raw request to see what's being passed
	ch.logger.Info("Raw request received",
		zap.Any("request", request),
		zap.Int("parsed_limit", limit),
	)

	ch.logger.Debug("Request parameters",
		zap.String("query", query),
		zap.String("sort", sortType),
		zap.String("channel_types", types),
		zap.String("cursor", cursor),
		zap.Int("limit", limit),
		zap.String("fields", fields),
		zap.Int("min_members", minMembers),
	)

	// Parse fields parameter
	requestedFields := make(map[string]bool)
	if fields == "all" {
		// Backward compatibility - include all fields
		requestedFields["id"] = true
		requestedFields["name"] = true
		requestedFields["topic"] = true
		requestedFields["purpose"] = true
		requestedFields["member_count"] = true
	} else {
		for _, field := range strings.Split(fields, ",") {
			field = strings.TrimSpace(strings.ToLower(field))
			// Normalize field names
			if field == "membercount" {
				field = "member_count"
			}
			requestedFields[field] = true
		}
	}

	ch.logger.Debug("Requested fields", zap.Any("fields", requestedFields))

	// MCP Inspector v0.14.0 has issues with Slice type
	// introspection, so some type simplification makes sense here
	channelTypes := []string{}
	for _, t := range strings.Split(types, ",") {
		t = strings.TrimSpace(t)
		if ch.validTypes[t] {
			channelTypes = append(channelTypes, t)
		} else if t != "" {
			ch.logger.Warn("Invalid channel type ignored", zap.String("type", t))
		}
	}

	if len(channelTypes) == 0 {
		ch.logger.Debug("No valid channel types provided, using defaults")
		channelTypes = append(channelTypes, provider.PubChanType)
		channelTypes = append(channelTypes, provider.PrivateChanType)
	}

	ch.logger.Debug("Validated channel types", zap.Strings("types", channelTypes))

	// Validate limit range
	if limit <= 0 {
		limit = 1000
		ch.logger.Debug("Invalid or missing limit, using default", zap.Int("limit", limit))
	} else if limit > 1000 {
		ch.logger.Warn("Limit exceeds maximum, capping to 1000", zap.Int("requested", limit))
		limit = 1000
	}

	var nextcur string

	allChannels := ch.apiProvider.ProvideChannelsMaps().Channels
	ch.logger.Debug("Total channels available", zap.Int("count", len(allChannels)))

	// Apply search query if provided
	if query != "" {
		searchResults := make(map[string]provider.Channel)
		queryLower := strings.ToLower(query)

		for id, channel := range allChannels {
			// Search in channel name, topic, and purpose (case-insensitive)
			if strings.Contains(strings.ToLower(channel.Name), queryLower) ||
				strings.Contains(strings.ToLower(channel.Topic), queryLower) ||
				strings.Contains(strings.ToLower(channel.Purpose), queryLower) {
				searchResults[id] = channel
			}
		}

		ch.logger.Debug("Search results",
			zap.String("query", query),
			zap.Int("matches", len(searchResults)),
		)

		// Use search results as the base for further filtering
		allChannels = searchResults
	}

	channels := filterChannelsByTypes(allChannels, channelTypes)
	ch.logger.Debug("Channels after filtering by type", zap.Int("count", len(channels)))

	// Apply min_members filter
	if minMembers > 0 {
		var filtered []provider.Channel
		for _, ch := range channels {
			if ch.MemberCount >= minMembers {
				filtered = append(filtered, ch)
			}
		}
		ch.logger.Debug("Channels after min_members filter",
			zap.Int("before", len(channels)),
			zap.Int("after", len(filtered)),
			zap.Int("min_members", minMembers),
		)
		channels = filtered
	}

	// Sort BEFORE pagination to ensure consistent ordering
	switch sortType {
	case "popularity":
		ch.logger.Debug("Sorting channels by popularity (member count)")
		sort.Slice(channels, func(i, j int) bool {
			return channels[i].MemberCount > channels[j].MemberCount
		})
	default:
		// Default sort by ID happens in paginateChannels
		ch.logger.Debug("No custom sorting applied", zap.String("sort_type", sortType))
	}

	var chans []provider.Channel

	chans, nextcur = paginateChannels(
		channels,
		cursor,
		limit,
	)

	ch.logger.Debug("Pagination results",
		zap.Int("returned_count", len(chans)),
		zap.Bool("has_next_page", nextcur != ""),
	)

	// Build dynamic CSV with only requested fields
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	// Determine field order and headers
	var headers []string
	var fieldOrder []string

	// Define consistent field order
	possibleFields := []struct {
		key    string
		header string
	}{
		{"id", "ID"},
		{"name", "Name"},
		{"topic", "Topic"},
		{"purpose", "Purpose"},
		{"member_count", "MemberCount"},
	}

	for _, field := range possibleFields {
		if requestedFields[field.key] {
			fieldOrder = append(fieldOrder, field.key)
			headers = append(headers, field.header)
		}
	}

	// If no fields requested, use defaults
	if len(fieldOrder) == 0 {
		fieldOrder = []string{"id", "name"}
		headers = []string{"ID", "Name"}
	}

	// Write headers
	if err := writer.Write(headers); err != nil {
		ch.logger.Error("Failed to write CSV headers", zap.Error(err))
		return mcp.NewToolResultErrorFromErr("Failed to format channel results", err), nil
	}

	// Write data rows
	for _, channel := range chans {
		var row []string
		for _, field := range fieldOrder {
			switch field {
			case "id":
				row = append(row, channel.ID)
			case "name":
				row = append(row, channel.Name)
			case "topic":
				row = append(row, channel.Topic)
			case "purpose":
				row = append(row, channel.Purpose)
			case "member_count":
				row = append(row, fmt.Sprintf("%d", channel.MemberCount))
			}
		}
		if err := writer.Write(row); err != nil {
			ch.logger.Error("Failed to write CSV row", zap.Error(err))
			return mcp.NewToolResultErrorFromErr("Failed to format channel results", err), nil
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		ch.logger.Error("CSV writer error", zap.Error(err))
		return mcp.NewToolResultErrorFromErr("Failed to format channel results", err), nil
	}

	// Build result with metadata at the beginning
	var result string
	result += fmt.Sprintf("# Total channels: %d\n", len(channels))
	result += fmt.Sprintf("# Returned in this page: %d\n", len(chans))
	if nextcur != "" {
		result += fmt.Sprintf("# Next cursor: %s\n", nextcur)
	} else {
		result += "# Next cursor: (none - last page)\n"
	}
	result += buf.String()

	return mcp.NewToolResultText(result), nil
}

func filterChannelsByTypes(channels map[string]provider.Channel, types []string) []provider.Channel {
	logger := zap.L()

	var result []provider.Channel
	typeSet := make(map[string]bool)

	for _, t := range types {
		typeSet[t] = true
	}

	publicCount := 0
	privateCount := 0
	imCount := 0
	mpimCount := 0

	for _, ch := range channels {
		if typeSet["public_channel"] && !ch.IsPrivate && !ch.IsIM && !ch.IsMpIM {
			result = append(result, ch)
			publicCount++
		}
		if typeSet["private_channel"] && ch.IsPrivate && !ch.IsIM && !ch.IsMpIM {
			result = append(result, ch)
			privateCount++
		}
		if typeSet["im"] && ch.IsIM {
			result = append(result, ch)
			imCount++
		}
		if typeSet["mpim"] && ch.IsMpIM {
			result = append(result, ch)
			mpimCount++
		}
	}

	logger.Debug("Channel filtering complete",
		zap.Int("total_input", len(channels)),
		zap.Int("total_output", len(result)),
		zap.Int("public_channels", publicCount),
		zap.Int("private_channels", privateCount),
		zap.Int("ims", imCount),
		zap.Int("mpims", mpimCount),
	)

	return result
}

func paginateChannels(channels []provider.Channel, cursor string, limit int) ([]provider.Channel, string) {
	logger := zap.L()

	// Only sort if not already sorted (e.g., by popularity)
	// Check if channels are sorted by ID by checking first few elements
	needsSort := true
	if len(channels) > 1 {
		// If already sorted by member count descending (popularity), don't re-sort
		if channels[0].MemberCount >= channels[len(channels)-1].MemberCount {
			needsSort = false
		}
	}

	if needsSort {
		sort.Slice(channels, func(i, j int) bool {
			return channels[i].ID < channels[j].ID
		})
	}

	startIndex := 0
	if cursor != "" {
		if decoded, err := base64.StdEncoding.DecodeString(cursor); err == nil {
			// For simple index-based pagination
			if idx, err := strconv.Atoi(string(decoded)); err == nil {
				startIndex = idx
				logger.Debug("Using index-based cursor",
					zap.String("cursor", cursor),
					zap.Int("start_index", startIndex),
				)
			} else {
				// Fallback to ID-based pagination
				lastID := string(decoded)
				for i, ch := range channels {
					if ch.ID > lastID {
						startIndex = i
						break
					}
				}
				logger.Debug("Using ID-based cursor",
					zap.String("cursor", cursor),
					zap.String("decoded_id", lastID),
					zap.Int("start_index", startIndex),
				)
			}
		} else {
			logger.Warn("Failed to decode cursor",
				zap.String("cursor", cursor),
				zap.Error(err),
			)
		}
	}

	endIndex := startIndex + limit
	if endIndex > len(channels) {
		endIndex = len(channels)
	}

	paged := channels[startIndex:endIndex]

	var nextCursor string
	if endIndex < len(channels) {
		// Use simple index-based cursor for consistency
		nextCursor = base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%d", endIndex)))
		logger.Debug("Generated next cursor",
			zap.Int("next_index", endIndex),
			zap.String("next_cursor", nextCursor),
		)
	}

	logger.Debug("Pagination complete",
		zap.Int("total_channels", len(channels)),
		zap.Int("start_index", startIndex),
		zap.Int("end_index", endIndex),
		zap.Int("page_size", len(paged)),
		zap.Bool("has_more", nextCursor != ""),
	)

	return paged, nextCursor
}

// Member represents a channel/DM member
type Member struct {
	UserID   string `csv:"user_id"`
	UserName string `csv:"user_name"`
	RealName string `csv:"real_name"`
	IsBot    bool   `csv:"is_bot"`
	IsAdmin  bool   `csv:"is_admin"`
	Status   string `csv:"status"`
}

// ListChannelMembersHandler lists members of a channel, MPIM, or DM
func (ch *ChannelsHandler) ListChannelMembersHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ch.logger.Debug("ListChannelMembersHandler called", zap.Any("params", request.Params))

	// Get channel_id parameter
	channelID := request.GetString("channel_id", "")
	if channelID == "" {
		ch.logger.Error("channel_id missing in list_channel_members params")
		return mcp.NewToolResultError("channel_id must be provided"), nil
	}

	// Handle channel name resolution
	if strings.HasPrefix(channelID, "#") || strings.HasPrefix(channelID, "@") {
		channelsMaps := ch.apiProvider.ProvideChannelsMaps()
		chn, ok := channelsMaps.ChannelsInv[channelID]
		if !ok {
			ch.logger.Error("Channel not found", zap.String("channel", channelID))
			return mcp.NewToolResultError(fmt.Sprintf("channel %q not found", channelID)), nil
		}
		channelID = channelsMaps.Channels[chn].ID
	}

	// Get pagination parameters
	cursor := request.GetString("cursor", "")
	limit := request.GetInt("limit", 100)
	if limit > 1000 {
		limit = 1000
	}

	// Check if this is a 1:1 DM by checking our channel cache
	channelsMaps := ch.apiProvider.ProvideChannelsMaps()
	channelInfo, exists := channelsMaps.Channels[channelID]

	var userIDs []string
	var nextCursor string

	if exists && channelInfo.IsIM {
		// This is a 1:1 DM - use conversations.info to get the two participants
		ch.logger.Debug("Channel is a 1:1 DM, using conversations.info", zap.String("channel_id", channelID))

		info, err := ch.apiProvider.Slack().GetConversationInfoContext(ctx, &slack.GetConversationInfoInput{
			ChannelID:         channelID,
			IncludeLocale:     false,
			IncludeNumMembers: false,
		})
		if err != nil {
			ch.logger.Error("Failed to get conversation info", zap.String("channel_id", channelID), zap.Error(err))
			return mcp.NewToolResultErrorFromErr("Failed to get DM participants", err), nil
		}

		// For DMs, the User field contains the other user's ID
		// We need to include both the current user and the other user
		authResp, err := ch.apiProvider.Slack().AuthTestContext(ctx)
		if err != nil {
			ch.logger.Error("Failed to get current user", zap.Error(err))
			return mcp.NewToolResultErrorFromErr("Failed to get current user", err), nil
		}

		userIDs = []string{authResp.UserID}
		if info.User != "" && info.User != authResp.UserID {
			userIDs = append(userIDs, info.User)
		}

		ch.logger.Debug("DM participants retrieved",
			zap.String("channel_id", channelID),
			zap.Strings("user_ids", userIDs))
	} else {
		// This is a regular channel or MPIM - use conversations.members
		ch.logger.Debug("Using conversations.members for channel/MPIM", zap.String("channel_id", channelID))

		params := &slack.GetUsersInConversationParameters{
			ChannelID: channelID,
			Cursor:    cursor,
			Limit:     limit,
		}

		var err error
		userIDs, nextCursor, err = ch.apiProvider.Slack().GetUsersInConversationContext(ctx, params)
		if err != nil {
			ch.logger.Error("Failed to get channel members", zap.String("channel_id", channelID), zap.Error(err))
			return mcp.NewToolResultErrorFromErr("Failed to get channel members", err), nil
		}

		ch.logger.Debug("Channel members retrieved",
			zap.String("channel_id", channelID),
			zap.Int("count", len(userIDs)),
			zap.String("next_cursor", nextCursor))
	}

	// Enrich user IDs with user details from cache
	usersMap := ch.apiProvider.ProvideUsersMap()
	members := make([]Member, 0, len(userIDs))

	for _, userID := range userIDs {
		user, exists := usersMap.Users[userID]
		if !exists {
			// User not in cache, create minimal entry
			members = append(members, Member{
				UserID:   userID,
				UserName: "unknown",
				RealName: "Unknown User",
				IsBot:    false,
				IsAdmin:  false,
				Status:   "unknown",
			})
			continue
		}

		// Determine user status
		status := "active"
		if user.Deleted {
			status = "deleted"
		} else if user.IsBot {
			status = "bot"
		} else if user.IsRestricted {
			status = "restricted"
		} else if user.IsUltraRestricted {
			status = "guest"
		}

		members = append(members, Member{
			UserID:   user.ID,
			UserName: user.Name,
			RealName: user.RealName,
			IsBot:    user.IsBot,
			IsAdmin:  user.IsAdmin,
			Status:   status,
		})
	}

	// Convert to CSV
	csvBytes, err := gocsv.MarshalBytes(&members)
	if err != nil {
		ch.logger.Error("Failed to marshal members to CSV", zap.Error(err))
		return mcp.NewToolResultErrorFromErr("Failed to format members as CSV", err), nil
	}

	// Build result with metadata at the beginning (as comments)
	var result strings.Builder
	result.WriteString(fmt.Sprintf("# Channel: %s\n", channelID))
	result.WriteString(fmt.Sprintf("# Total members returned: %d\n", len(members)))
	if nextCursor != "" {
		result.WriteString(fmt.Sprintf("# Next cursor: %s\n", nextCursor))
	} else {
		result.WriteString("# Next cursor: (none - all members returned)\n")
	}
	result.Write(csvBytes)

	ch.logger.Debug("Successfully retrieved channel members",
		zap.String("channel_id", channelID),
		zap.Int("member_count", len(members)),
		zap.Bool("has_more", nextCursor != ""))

	return mcp.NewToolResultText(result.String()), nil
}

// CreateChannelHandler creates a new channel
func (ch *ChannelsHandler) CreateChannelHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ch.logger.Debug("CreateChannelHandler called")

	if ready, err := ch.apiProvider.IsReady(); !ready {
		ch.logger.Error("API provider not ready", zap.Error(err))
		return mcp.NewToolResultErrorFromErr("API provider not ready", err), nil
	}

	// Get parameters
	name := request.GetString("name", "")
	isPrivate := request.GetBool("is_private", false)
	topic := request.GetString("topic", "")
	purpose := request.GetString("purpose", "")
	workspace := request.GetString("workspace", "") // Optional workspace Team ID

	ch.logger.Debug("Creating channel",
		zap.String("name", name),
		zap.Bool("is_private", isPrivate),
		zap.String("topic", topic),
		zap.String("purpose", purpose),
		zap.String("workspace", workspace))

	// Validate name
	if name == "" {
		return mcp.NewToolResultError("Channel name is required"), nil
	}

	// Create the channel - check if we have the extended interface
	var channel *slack.Channel
	var err error

	// Type assert to check if we have MCPSlackClient with workspace support
	if mcpClient, ok := ch.apiProvider.Slack().(*provider.MCPSlackClient); ok {
		// Use the workspace-aware method
		channel, err = mcpClient.CreateConversationInWorkspaceContext(ctx, name, isPrivate, workspace)
	} else {
		// Fallback to standard method (no workspace support)
		channel, err = ch.apiProvider.Slack().CreateConversationContext(ctx, name, isPrivate)
	}
	if err != nil {
		ch.logger.Error("Failed to create channel",
			zap.String("name", name),
			zap.Error(err))
		return mcp.NewToolResultErrorFromErr("Failed to create channel", err), nil
	}

	ch.logger.Info("Channel created successfully",
		zap.String("channel_id", channel.ID),
		zap.String("name", channel.Name))

	// Set topic if provided
	if topic != "" {
		_, err = ch.apiProvider.Slack().SetTopicOfConversationContext(ctx, channel.ID, topic)
		if err != nil {
			ch.logger.Warn("Failed to set channel topic",
				zap.String("channel_id", channel.ID),
				zap.String("topic", topic),
				zap.Error(err))
			// Don't fail the whole operation if topic setting fails
		} else {
			ch.logger.Debug("Channel topic set", zap.String("channel_id", channel.ID))
		}
	}

	// Set purpose if provided
	if purpose != "" {
		_, err = ch.apiProvider.Slack().SetPurposeOfConversationContext(ctx, channel.ID, purpose)
		if err != nil {
			ch.logger.Warn("Failed to set channel purpose",
				zap.String("channel_id", channel.ID),
				zap.String("purpose", purpose),
				zap.Error(err))
			// Don't fail the whole operation if purpose setting fails
		} else {
			ch.logger.Debug("Channel purpose set", zap.String("channel_id", channel.ID))
		}
	}

	// Prepare response data
	type ChannelCreated struct {
		ID      string `csv:"channel_id"`
		Name    string `csv:"name"`
		Private bool   `csv:"is_private"`
		Topic   string `csv:"topic"`
		Purpose string `csv:"purpose"`
		Created string `csv:"created"`
	}

	result := []ChannelCreated{{
		ID:      channel.ID,
		Name:    channel.Name,
		Private: channel.IsPrivate,
		Topic:   topic,
		Purpose: purpose,
		Created: "true",
	}}

	// Convert to CSV
	csvBytes, err := gocsv.MarshalBytes(&result)
	if err != nil {
		ch.logger.Error("Failed to marshal result to CSV", zap.Error(err))
		return mcp.NewToolResultErrorFromErr("Failed to format result as CSV", err), nil
	}

	// Build result with metadata
	var output strings.Builder
	output.WriteString(fmt.Sprintf("# Channel created successfully\n"))
	output.WriteString(fmt.Sprintf("# Channel ID: %s\n", channel.ID))
	output.WriteString(fmt.Sprintf("# Channel Name: %s\n", channel.Name))
	if channel.IsPrivate {
		output.WriteString("# Type: Private Channel\n")
	} else {
		output.WriteString("# Type: Public Channel\n")
	}
	output.Write(csvBytes)

	return mcp.NewToolResultText(output.String()), nil
}

// ArchiveChannelHandler archives a channel
func (ch *ChannelsHandler) ArchiveChannelHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ch.logger.Debug("ArchiveChannelHandler called")

	if ready, err := ch.apiProvider.IsReady(); !ready {
		ch.logger.Error("API provider not ready", zap.Error(err))
		return mcp.NewToolResultErrorFromErr("API provider not ready", err), nil
	}

	// Get channel ID parameter
	channelID := request.GetString("channel_id", "")

	ch.logger.Debug("Archiving channel", zap.String("channel_id", channelID))

	// Validate channel ID
	if channelID == "" {
		return mcp.NewToolResultError("Channel ID is required"), nil
	}

	// Handle channel name resolution
	if strings.HasPrefix(channelID, "#") {
		channelsMaps := ch.apiProvider.ProvideChannelsMaps()
		chn := strings.TrimPrefix(channelID, "#")
		if _, exists := channelsMaps.Channels[chn]; !exists {
			ch.logger.Error("Channel not found by name",
				zap.String("channel_name", chn))
			return mcp.NewToolResultError(fmt.Sprintf("channel %q not found", channelID)), nil
		}
		channelID = channelsMaps.Channels[chn].ID
	}

	// Get channel info before archiving for response
	channelInfo, err := ch.apiProvider.Slack().GetConversationInfoContext(ctx, &slack.GetConversationInfoInput{
		ChannelID: channelID,
	})
	if err != nil {
		ch.logger.Error("Failed to get channel info before archiving",
			zap.String("channel_id", channelID),
			zap.Error(err))
		// Continue anyway, we might still be able to archive
	}

	// Archive the channel
	err = ch.apiProvider.Slack().ArchiveConversationContext(ctx, channelID)
	if err != nil {
		ch.logger.Error("Failed to archive channel",
			zap.String("channel_id", channelID),
			zap.Error(err))
		return mcp.NewToolResultErrorFromErr("Failed to archive channel", err), nil
	}

	ch.logger.Info("Channel archived successfully",
		zap.String("channel_id", channelID))

	// Prepare response data
	type ChannelArchived struct {
		ID       string `csv:"channel_id"`
		Name     string `csv:"name"`
		Archived string `csv:"archived"`
	}

	channelName := channelID
	if channelInfo != nil {
		channelName = channelInfo.Name
	}

	result := []ChannelArchived{{
		ID:       channelID,
		Name:     channelName,
		Archived: "true",
	}}

	// Convert to CSV
	csvBytes, err := gocsv.MarshalBytes(&result)
	if err != nil {
		ch.logger.Error("Failed to marshal result to CSV", zap.Error(err))
		return mcp.NewToolResultErrorFromErr("Failed to format result as CSV", err), nil
	}

	// Build result with metadata
	var output strings.Builder
	output.WriteString(fmt.Sprintf("# Channel archived successfully\n"))
	output.WriteString(fmt.Sprintf("# Channel ID: %s\n", channelID))
	if channelInfo != nil {
		output.WriteString(fmt.Sprintf("# Channel Name: %s\n", channelInfo.Name))
	}
	output.Write(csvBytes)

	return mcp.NewToolResultText(output.String()), nil
}
