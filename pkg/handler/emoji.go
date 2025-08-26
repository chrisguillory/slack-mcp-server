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

	"github.com/korotovsky/slack-mcp-server/pkg/provider"
	"github.com/mark3labs/mcp-go/mcp"
	"go.uber.org/zap"
)

type EmojiCSV struct {
	Name     string `csv:"name"`
	URL      string `csv:"url"`
	IsCustom string `csv:"is_custom"`
	Aliases  string `csv:"aliases"`
	TeamID   string `csv:"team_id"`
	UserID   string `csv:"user_id"`
}

type EmojiHandler struct {
	apiProvider *provider.ApiProvider
	logger      *zap.Logger
}

func NewEmojiHandler(apiProvider *provider.ApiProvider, logger *zap.Logger) *EmojiHandler {
	return &EmojiHandler{
		apiProvider: apiProvider,
		logger:      logger,
	}
}

func (eh *EmojiHandler) EmojiListHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	eh.logger.Debug("EmojiListHandler called")

	// Check if basic API is ready (users and channels)
	if ready, err := eh.apiProvider.IsReady(); !ready {
		eh.logger.Error("API provider not ready", zap.Error(err))
		return mcp.NewToolResultErrorFromErr("API provider not ready", err), nil
	}

	// Check if emojis are ready
	if ready, err := eh.apiProvider.IsEmojisReady(); !ready {
		eh.logger.Error("Emojis cache not ready", zap.Error(err))
		return mcp.NewToolResultErrorFromErr("Emojis cache not ready", err), nil
	}

	// Get parameters with defaults
	emojiType := request.GetString("type", "all")
	cursor := request.GetString("cursor", "")
	limit := request.GetInt("limit", 1000)
	query := request.GetString("query", "")

	eh.logger.Debug("Request parameters",
		zap.String("type", emojiType),
		zap.String("cursor", cursor),
		zap.Int("limit", limit),
		zap.String("query", query),
	)

	// Validate limit range
	if limit <= 0 {
		limit = 1000
		eh.logger.Debug("Invalid or missing limit, using default", zap.Int("limit", limit))
	} else if limit > 1000 {
		eh.logger.Warn("Limit exceeds maximum, capping to 1000", zap.Int("requested", limit))
		limit = 1000
	}

	// Get emojis from cache
	emojisCache := eh.apiProvider.ProvideEmojiMap()
	allEmojis := emojisCache.Emojis
	eh.logger.Debug("Total emojis available", zap.Int("count", len(allEmojis)))

	// Apply search query if provided
	var searchResults map[string]provider.Emoji
	if query != "" {
		searchResults = make(map[string]provider.Emoji)
		queryLower := strings.ToLower(query)

		for name, emoji := range allEmojis {
			// Search in emoji name and aliases (case-insensitive)
			if strings.Contains(strings.ToLower(name), queryLower) {
				searchResults[name] = emoji
				continue
			}
			// Check aliases
			for _, alias := range emoji.Aliases {
				if strings.Contains(strings.ToLower(alias), queryLower) {
					searchResults[name] = emoji
					break
				}
			}
		}

		eh.logger.Debug("Search results",
			zap.String("query", query),
			zap.Int("matches", len(searchResults)),
		)

		// Use search results as the base for further filtering
		allEmojis = searchResults
	}

	// Filter emojis based on type
	var filteredEmojis []provider.Emoji
	for _, emoji := range allEmojis {
		switch emojiType {
		case "custom":
			if emoji.IsCustom {
				filteredEmojis = append(filteredEmojis, emoji)
			}
		case "unicode":
			if !emoji.IsCustom {
				filteredEmojis = append(filteredEmojis, emoji)
			}
		case "all":
			filteredEmojis = append(filteredEmojis, emoji)
		default:
			// Invalid filter, default to all
			filteredEmojis = append(filteredEmojis, emoji)
		}
	}

	eh.logger.Debug("Emojis after filtering", zap.Int("count", len(filteredEmojis)))

	// Sort emojis by name for consistent pagination
	sort.Slice(filteredEmojis, func(i, j int) bool {
		return filteredEmojis[i].Name < filteredEmojis[j].Name
	})

	// Paginate
	var paginatedEmojis []provider.Emoji
	var nextCursor string

	paginatedEmojis, nextCursor = paginateEmojis(filteredEmojis, cursor, limit)

	eh.logger.Debug("Pagination results",
		zap.Int("returned_count", len(paginatedEmojis)),
		zap.Bool("has_next_page", nextCursor != ""),
	)

	// Build CSV
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	// Write headers
	headers := []string{"Name", "URL", "IsCustom", "Aliases", "TeamID", "UserID"}
	if err := writer.Write(headers); err != nil {
		eh.logger.Error("Failed to write CSV headers", zap.Error(err))
		return nil, err
	}

	// Write data rows
	for _, emoji := range paginatedEmojis {
		row := []string{
			emoji.Name,
			emoji.URL,
			fmt.Sprintf("%t", emoji.IsCustom),
			strings.Join(emoji.Aliases, "|"),
			emoji.TeamID,
			emoji.UserID,
		}
		if err := writer.Write(row); err != nil {
			eh.logger.Error("Failed to write CSV row", zap.Error(err))
			return nil, err
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		eh.logger.Error("CSV writer error", zap.Error(err))
		return nil, err
	}

	// Build result with metadata at the beginning
	var result string
	result += fmt.Sprintf("# Total emojis: %d\n", len(filteredEmojis))
	result += fmt.Sprintf("# Returned in this page: %d\n", len(paginatedEmojis))
	if nextCursor != "" {
		result += fmt.Sprintf("# Next cursor: %s\n", nextCursor)
	} else {
		result += "# Next cursor: (none - last page)\n"
	}
	result += buf.String()

	return mcp.NewToolResultText(result), nil
}

func paginateEmojis(emojis []provider.Emoji, cursor string, limit int) ([]provider.Emoji, string) {
	logger := zap.L()

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
			}
		} else {
			logger.Warn("Failed to decode cursor",
				zap.String("cursor", cursor),
				zap.Error(err),
			)
		}
	}

	endIndex := startIndex + limit
	if endIndex > len(emojis) {
		endIndex = len(emojis)
	}

	paged := emojis[startIndex:endIndex]

	var nextCursor string
	if endIndex < len(emojis) {
		// Use simple index-based cursor for consistency
		nextCursor = base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%d", endIndex)))
		logger.Debug("Generated next cursor",
			zap.Int("next_index", endIndex),
			zap.String("next_cursor", nextCursor),
		)
	}

	logger.Debug("Pagination complete",
		zap.Int("total_emojis", len(emojis)),
		zap.Int("start_index", startIndex),
		zap.Int("end_index", endIndex),
		zap.Int("page_size", len(paged)),
		zap.Bool("has_more", nextCursor != ""),
	)

	return paged, nextCursor
}
