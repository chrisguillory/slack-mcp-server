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
	"github.com/slack-go/slack"
	"go.uber.org/zap"
)

type UserCSV struct {
	ID       string `csv:"ID"`
	Name     string `csv:"Name"`
	RealName string `csv:"RealName"`
	Email    string `csv:"Email"`
	Status   string `csv:"Status"`
	IsBot    string `csv:"IsBot"`
	IsAdmin  string `csv:"IsAdmin"`
	TimeZone string `csv:"TimeZone"`
	Title    string `csv:"Title"`
	Phone    string `csv:"Phone"`
}

type UsersHandler struct {
	apiProvider *provider.ApiProvider
	logger      *zap.Logger
}

func NewUsersHandler(apiProvider *provider.ApiProvider, logger *zap.Logger) *UsersHandler {
	return &UsersHandler{
		apiProvider: apiProvider,
		logger:      logger,
	}
}

func (uh *UsersHandler) UsersHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	uh.logger.Debug("UsersHandler called")

	if ready, err := uh.apiProvider.IsReady(); !ready {
		uh.logger.Error("API provider not ready", zap.Error(err))
		return mcp.NewToolResultErrorFromErr("API provider not ready", err), nil
	}

	// Get parameters with defaults
	query := request.GetString("query", "")
	filter := request.GetString("filter", "all")
	cursor := request.GetString("cursor", "")
	limit := request.GetInt("limit", 1000)
	fields := request.GetString("fields", "id,name,real_name,status")
	includeDeleted := request.GetBool("include_deleted", false)
	includeBots := request.GetBool("include_bots", true)

	uh.logger.Debug("Request parameters",
		zap.String("query", query),
		zap.String("filter", filter),
		zap.String("cursor", cursor),
		zap.Int("limit", limit),
		zap.String("fields", fields),
		zap.Bool("include_deleted", includeDeleted),
		zap.Bool("include_bots", includeBots),
	)

	// Parse fields parameter
	requestedFields := make(map[string]bool)
	if fields == "all" {
		// Include all available fields
		requestedFields["id"] = true
		requestedFields["name"] = true
		requestedFields["real_name"] = true
		requestedFields["email"] = true
		requestedFields["status"] = true
		requestedFields["is_bot"] = true
		requestedFields["is_admin"] = true
		requestedFields["time_zone"] = true
		requestedFields["title"] = true
		requestedFields["phone"] = true
	} else {
		for _, field := range strings.Split(fields, ",") {
			field = strings.TrimSpace(strings.ToLower(field))
			// Normalize field names
			if field == "realname" {
				field = "real_name"
			}
			if field == "isbot" {
				field = "is_bot"
			}
			if field == "isadmin" {
				field = "is_admin"
			}
			if field == "timezone" {
				field = "time_zone"
			}
			requestedFields[field] = true
		}
	}

	uh.logger.Debug("Requested fields", zap.Any("fields", requestedFields))

	// Validate limit range
	if limit <= 0 {
		limit = 1000
		uh.logger.Debug("Invalid or missing limit, using default", zap.Int("limit", limit))
	} else if limit > 1000 {
		uh.logger.Warn("Limit exceeds maximum, capping to 1000", zap.Int("requested", limit))
		limit = 1000
	}

	// Get users from cache
	usersCache := uh.apiProvider.ProvideUsersMap()
	allUsers := usersCache.Users
	uh.logger.Debug("Total users available", zap.Int("count", len(allUsers)))

	// Apply search query if provided
	var searchResults map[string]slack.User
	if query != "" {
		searchResults = make(map[string]slack.User)
		queryLower := strings.ToLower(query)

		for id, user := range allUsers {
			// Search in username, real name, and display name (case-insensitive)
			if strings.Contains(strings.ToLower(user.Name), queryLower) ||
				strings.Contains(strings.ToLower(user.RealName), queryLower) ||
				strings.Contains(strings.ToLower(user.Profile.DisplayName), queryLower) ||
				strings.Contains(strings.ToLower(user.Profile.RealName), queryLower) {
				searchResults[id] = user
			}
		}

		uh.logger.Debug("Search results",
			zap.String("query", query),
			zap.Int("matches", len(searchResults)),
		)

		// Use search results as the base for further filtering
		allUsers = searchResults
	}

	// Filter users based on parameters
	var filteredUsers []slack.User
	for _, user := range allUsers {
		// Skip deleted users if not included
		if user.Deleted && !includeDeleted {
			continue
		}

		// Skip bots if not included
		if user.IsBot && !includeBots {
			continue
		}

		// Apply filter
		switch filter {
		case "active":
			if !user.Deleted {
				filteredUsers = append(filteredUsers, user)
			}
		case "deleted":
			if user.Deleted {
				filteredUsers = append(filteredUsers, user)
			}
		case "bots":
			if user.IsBot {
				filteredUsers = append(filteredUsers, user)
			}
		case "humans":
			if !user.IsBot {
				filteredUsers = append(filteredUsers, user)
			}
		case "admins":
			if user.IsAdmin || user.IsOwner || user.IsPrimaryOwner {
				filteredUsers = append(filteredUsers, user)
			}
		case "all":
			filteredUsers = append(filteredUsers, user)
		default:
			// Invalid filter, default to all
			filteredUsers = append(filteredUsers, user)
		}
	}

	uh.logger.Debug("Users after filtering", zap.Int("count", len(filteredUsers)))

	// Sort users by name for consistent pagination
	sort.Slice(filteredUsers, func(i, j int) bool {
		return filteredUsers[i].Name < filteredUsers[j].Name
	})

	// Paginate
	var paginatedUsers []slack.User
	var nextCursor string

	paginatedUsers, nextCursor = paginateUsers(filteredUsers, cursor, limit)

	uh.logger.Debug("Pagination results",
		zap.Int("returned_count", len(paginatedUsers)),
		zap.Bool("has_next_page", nextCursor != ""),
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
		{"real_name", "RealName"},
		{"email", "Email"},
		{"status", "Status"},
		{"is_bot", "IsBot"},
		{"is_admin", "IsAdmin"},
		{"time_zone", "TimeZone"},
		{"title", "Title"},
		{"phone", "Phone"},
	}

	for _, field := range possibleFields {
		if requestedFields[field.key] {
			fieldOrder = append(fieldOrder, field.key)
			headers = append(headers, field.header)
		}
	}

	// If no fields requested, use defaults
	if len(fieldOrder) == 0 {
		fieldOrder = []string{"id", "name", "real_name", "status"}
		headers = []string{"ID", "Name", "RealName", "Status"}
	}

	// Write headers
	if err := writer.Write(headers); err != nil {
		uh.logger.Error("Failed to write CSV headers", zap.Error(err))
		return nil, err
	}

	// Write data rows
	for _, user := range paginatedUsers {
		var row []string
		for _, field := range fieldOrder {
			switch field {
			case "id":
				row = append(row, user.ID)
			case "name":
				row = append(row, user.Name)
			case "real_name":
				row = append(row, user.RealName)
			case "email":
				if user.Profile.Email != "" {
					row = append(row, user.Profile.Email)
				} else {
					row = append(row, "")
				}
			case "status":
				status := "active"
				if user.Deleted {
					status = "deleted"
				}
				row = append(row, status)
			case "is_bot":
				row = append(row, fmt.Sprintf("%t", user.IsBot))
			case "is_admin":
				isAdmin := user.IsAdmin || user.IsOwner || user.IsPrimaryOwner
				row = append(row, fmt.Sprintf("%t", isAdmin))
			case "time_zone":
				row = append(row, user.TZ)
			case "title":
				row = append(row, user.Profile.Title)
			case "phone":
				row = append(row, user.Profile.Phone)
			}
		}
		if err := writer.Write(row); err != nil {
			uh.logger.Error("Failed to write CSV row", zap.Error(err))
			return nil, err
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		uh.logger.Error("CSV writer error", zap.Error(err))
		return nil, err
	}

	// Build result with metadata at the beginning
	var result string
	result += fmt.Sprintf("# Total users: %d\n", len(filteredUsers))
	result += fmt.Sprintf("# Returned in this page: %d\n", len(paginatedUsers))
	if nextCursor != "" {
		result += fmt.Sprintf("# Next cursor: %s\n", nextCursor)
	} else {
		result += "# Next cursor: (none - last page)\n"
	}
	result += buf.String()

	return mcp.NewToolResultText(result), nil
}

func paginateUsers(users []slack.User, cursor string, limit int) ([]slack.User, string) {
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
			} else {
				// Fallback to ID-based pagination
				lastID := string(decoded)
				for i, user := range users {
					if user.ID > lastID {
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
	if endIndex > len(users) {
		endIndex = len(users)
	}

	paged := users[startIndex:endIndex]

	var nextCursor string
	if endIndex < len(users) {
		// Use simple index-based cursor for consistency
		nextCursor = base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%d", endIndex)))
		logger.Debug("Generated next cursor",
			zap.Int("next_index", endIndex),
			zap.String("next_cursor", nextCursor),
		)
	}

	logger.Debug("Pagination complete",
		zap.Int("total_users", len(users)),
		zap.Int("start_index", startIndex),
		zap.Int("end_index", endIndex),
		zap.Int("page_size", len(paged)),
		zap.Bool("has_more", nextCursor != ""),
	)

	return paged, nextCursor
}
