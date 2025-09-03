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

// GetUserInfoHandler returns detailed information about a single user
func (uh *UsersHandler) GetUserInfoHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	uh.logger.Debug("GetUserInfoHandler called", zap.Any("params", request.Params))

	// Get user_id parameter
	userID := request.GetString("user_id", "")
	if userID == "" {
		uh.logger.Error("user_id missing in get_user_info params")
		return mcp.NewToolResultError("user_id must be provided"), nil
	}

	// Handle username resolution (@username)
	if strings.HasPrefix(userID, "@") {
		username := strings.TrimPrefix(userID, "@")
		usersMap := uh.apiProvider.ProvideUsersMap()
		if uid, ok := usersMap.UsersInv[username]; ok {
			userID = uid
		} else {
			uh.logger.Error("User not found", zap.String("username", username))
			return mcp.NewToolResultError(fmt.Sprintf("user @%s not found", username)), nil
		}
	}

	// Get fields parameter
	fieldsParam := request.GetString("fields", "id,name,real_name,display_name,email,title,status_text,is_admin,is_bot")

	// Parse requested fields
	requestedFields := make(map[string]bool)
	if fieldsParam == "all" {
		// Request all available fields
		allFields := []string{
			"id", "team_id", "name", "deleted", "color", "updated",
			"real_name", "display_name", "display_name_normalized", "first_name", "last_name",
			"email", "phone", "skype", "title", "pronouns", "start_date",
			"status_text", "status_emoji", "status_expiration",
			"tz", "tz_label", "tz_offset", "locale",
			"is_admin", "is_owner", "is_primary_owner", "is_restricted", "is_ultra_restricted",
			"is_bot", "is_app_user", "is_stranger", "is_invited_user", "is_email_confirmed", "has_2fa",
			"avatar_hash", "image_24", "image_32", "image_48", "image_72", "image_192", "image_512",
			"enterprise_id", "enterprise_name", "enterprise_user_id", "enterprise_is_admin", "enterprise_is_owner",
			"presence", "online", "auto_away", "manual_away", "connection_count", "last_activity",
			"always_active", "billing_active",
		}
		for _, field := range allFields {
			requestedFields[field] = true
		}
	} else if fieldsParam == "extended" {
		// Extended set of commonly used fields
		extendedFields := []string{
			"id", "team_id", "name", "real_name", "display_name", "email", "title",
			"status_text", "status_emoji", "is_admin", "is_owner", "is_bot", "is_restricted",
			"tz", "tz_label", "locale", "image_192",
			"enterprise_id", "enterprise_name",
		}
		for _, field := range extendedFields {
			requestedFields[field] = true
		}
	} else {
		// Parse custom field list
		for _, field := range strings.Split(fieldsParam, ",") {
			field = strings.TrimSpace(field)
			if field != "" {
				requestedFields[field] = true
			}
		}
	}

	// Fetch user info from API
	user, err := uh.apiProvider.Slack().GetUserInfoContext(ctx, userID)
	if err != nil {
		// If API fails, try to get from cache
		uh.logger.Warn("Failed to get user from API, trying cache", zap.String("user_id", userID), zap.Error(err))
		usersMap := uh.apiProvider.ProvideUsersMap()
		if cachedUser, ok := usersMap.Users[userID]; ok {
			user = &cachedUser
		} else {
			uh.logger.Error("Failed to get user info", zap.String("user_id", userID), zap.Error(err))
			return mcp.NewToolResultErrorFromErr("Failed to get user info", err), nil
		}
	}

	// Fetch presence if requested
	var presence *slack.UserPresence
	if requestedFields["presence"] || requestedFields["online"] || requestedFields["auto_away"] ||
		requestedFields["manual_away"] || requestedFields["connection_count"] || requestedFields["last_activity"] {
		presence, err = uh.apiProvider.Slack().GetUserPresenceContext(ctx, userID)
		if err != nil {
			uh.logger.Warn("Failed to get user presence", zap.String("user_id", userID), zap.Error(err))
			// Don't fail the whole request if presence fails
		}
	}

	// Build CSV header and row based on requested fields
	var headers []string
	var values []string

	// Helper function to add field if requested
	addField := func(fieldName, value string) {
		if requestedFields[fieldName] {
			headers = append(headers, fieldName)
			values = append(values, value)
		}
	}

	// Add basic fields
	addField("id", user.ID)
	addField("team_id", user.TeamID)
	addField("name", user.Name)
	addField("deleted", fmt.Sprintf("%t", user.Deleted))
	addField("color", user.Color)
	addField("updated", fmt.Sprintf("%d", user.Updated))

	// Add name fields
	addField("real_name", user.RealName)
	addField("display_name", user.Profile.DisplayName)
	addField("display_name_normalized", user.Profile.DisplayNameNormalized)
	addField("first_name", user.Profile.FirstName)
	addField("last_name", user.Profile.LastName)

	// Add contact fields
	addField("email", user.Profile.Email)
	addField("phone", user.Profile.Phone)
	addField("skype", user.Profile.Skype)
	addField("title", user.Profile.Title)
	// Note: pronouns and start_date not available in current slack-go version
	addField("pronouns", "")
	addField("start_date", "")

	// Add status fields
	addField("status_text", user.Profile.StatusText)
	addField("status_emoji", user.Profile.StatusEmoji)
	addField("status_expiration", fmt.Sprintf("%d", user.Profile.StatusExpiration))

	// Add timezone fields
	addField("tz", user.TZ)
	addField("tz_label", user.TZLabel)
	addField("tz_offset", fmt.Sprintf("%d", user.TZOffset))
	addField("locale", user.Locale)

	// Add permission fields
	addField("is_admin", fmt.Sprintf("%t", user.IsAdmin))
	addField("is_owner", fmt.Sprintf("%t", user.IsOwner))
	addField("is_primary_owner", fmt.Sprintf("%t", user.IsPrimaryOwner))
	addField("is_restricted", fmt.Sprintf("%t", user.IsRestricted))
	addField("is_ultra_restricted", fmt.Sprintf("%t", user.IsUltraRestricted))
	addField("is_bot", fmt.Sprintf("%t", user.IsBot))
	addField("is_app_user", fmt.Sprintf("%t", user.IsAppUser))
	addField("is_stranger", fmt.Sprintf("%t", user.IsStranger))
	addField("is_invited_user", fmt.Sprintf("%t", user.IsInvitedUser))

	// Add security fields
	// Note: IsEmailConfirmed not available in current slack-go version
	addField("is_email_confirmed", "")
	addField("has_2fa", fmt.Sprintf("%t", user.Has2FA))

	// Add profile images
	addField("avatar_hash", user.Profile.AvatarHash)
	addField("image_24", user.Profile.Image24)
	addField("image_32", user.Profile.Image32)
	addField("image_48", user.Profile.Image48)
	addField("image_72", user.Profile.Image72)
	addField("image_192", user.Profile.Image192)
	addField("image_512", user.Profile.Image512)

	// Add enterprise fields
	// EnterpriseUser is a struct, not a pointer, check if ID is set
	if user.Enterprise.ID != "" {
		addField("enterprise_id", user.Enterprise.EnterpriseID)
		addField("enterprise_name", user.Enterprise.EnterpriseName)
		addField("enterprise_user_id", user.Enterprise.ID)
		addField("enterprise_is_admin", fmt.Sprintf("%t", user.Enterprise.IsAdmin))
		addField("enterprise_is_owner", fmt.Sprintf("%t", user.Enterprise.IsOwner))
	} else {
		addField("enterprise_id", "")
		addField("enterprise_name", "")
		addField("enterprise_user_id", "")
		addField("enterprise_is_admin", "false")
		addField("enterprise_is_owner", "false")
	}

	// Add presence fields if fetched
	if presence != nil {
		addField("presence", presence.Presence)
		addField("online", fmt.Sprintf("%t", presence.Online))
		addField("auto_away", fmt.Sprintf("%t", presence.AutoAway))
		addField("manual_away", fmt.Sprintf("%t", presence.ManualAway))
		addField("connection_count", fmt.Sprintf("%d", presence.ConnectionCount))
		addField("last_activity", fmt.Sprintf("%d", presence.LastActivity))
	} else {
		// Add empty values if presence wasn't fetched
		addField("presence", "")
		addField("online", "")
		addField("auto_away", "")
		addField("manual_away", "")
		addField("connection_count", "")
		addField("last_activity", "")
	}

	// Bot-specific fields
	// Note: AlwaysActive not available in current slack-go version
	addField("always_active", "")

	// Build CSV output
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	// Write headers
	if err := writer.Write(headers); err != nil {
		uh.logger.Error("Failed to write CSV headers", zap.Error(err))
		return mcp.NewToolResultErrorFromErr("Failed to format user info", err), nil
	}

	// Write values
	if err := writer.Write(values); err != nil {
		uh.logger.Error("Failed to write CSV values", zap.Error(err))
		return mcp.NewToolResultErrorFromErr("Failed to format user info", err), nil
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		uh.logger.Error("CSV writer error", zap.Error(err))
		return mcp.NewToolResultErrorFromErr("Failed to format user info", err), nil
	}

	// Build result with metadata
	var result strings.Builder
	result.WriteString(fmt.Sprintf("# User: %s (%s)\n", user.RealName, user.ID))
	result.WriteString(fmt.Sprintf("# Fields returned: %d\n", len(headers)))
	result.WriteString(buf.String())

	uh.logger.Debug("Successfully retrieved user info",
		zap.String("user_id", user.ID),
		zap.Int("field_count", len(headers)))

	return mcp.NewToolResultText(result.String()), nil
}
