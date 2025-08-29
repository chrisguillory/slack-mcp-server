package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/csv"
	"errors"
	"fmt"
	"net/url"
	"regexp"
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

var validFilterKeys = map[string]struct{}{
	"is":     {},
	"in":     {},
	"from":   {},
	"with":   {},
	"before": {},
	"after":  {},
	"on":     {},
	"during": {},
}

type SearchHandler struct {
	apiProvider *provider.ApiProvider
	logger      *zap.Logger
}

func NewSearchHandler(apiProvider *provider.ApiProvider, logger *zap.Logger) *SearchHandler {
	return &SearchHandler{
		apiProvider: apiProvider,
		logger:      logger,
	}
}

type searchParams struct {
	query string
	limit int
	page  int
	sort  string
}

// SearchMessage is like Message but without Cursor field (cursor is in metadata)
type SearchMessage struct {
	MsgID     string `json:"msgID"`
	UserID    string `json:"userID"`
	UserName  string `json:"userUser"`
	RealName  string `json:"realName"`
	Channel   string `json:"channelID"`
	ThreadTs  string `json:"ThreadTs"`
	Text      string `json:"text"`
	Time      string `json:"time"`
	Reactions string `json:"reactions,omitempty"`
	Permalink string `json:"permalink"`
}

func (sh *SearchHandler) SearchMessagesHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sh.logger.Debug("SearchMessagesHandler called", zap.Any("params", request.Params))

	params, err := sh.parseParamsToolSearch(request)
	if err != nil {
		sh.logger.Error("Failed to parse search params", zap.Error(err))
		return mcp.NewToolResultErrorFromErr("Failed to parse search parameters", err), nil
	}
	sh.logger.Debug("Search params parsed", zap.String("query", params.query), zap.Int("limit", params.limit), zap.Int("page", params.page), zap.String("sort", params.sort))

	// Configure sort parameters based on user choice
	var sortField, sortDir string
	if params.sort == "chronological" {
		sortField = "timestamp"
		sortDir = "asc" // Oldest first
	} else {
		sortField = slack.DEFAULT_SEARCH_SORT   // "score"
		sortDir = slack.DEFAULT_SEARCH_SORT_DIR // "desc"
	}

	searchParams := slack.SearchParameters{
		Sort:          sortField,
		SortDirection: sortDir,
		Highlight:     false,
		Count:         params.limit,
		Page:          params.page,
	}
	messagesRes, _, err := sh.apiProvider.Slack().SearchContext(ctx, params.query, searchParams)
	if err != nil {
		sh.logger.Error("Slack SearchContext failed", zap.Error(err))
		return mcp.NewToolResultErrorFromErr("Failed to search messages", err), nil
	}
	sh.logger.Debug("Search completed", zap.Int("matches", len(messagesRes.Matches)))

	messages := sh.convertMessagesFromSearch(messagesRes.Matches)

	// Determine if there's a next page
	var nextCursor string
	// Check if current page * items per page is less than total (meaning there are more items)
	if len(messages) > 0 && (messagesRes.Pagination.Page*messagesRes.Pagination.PerPage) < messagesRes.Pagination.TotalCount {
		nextCursor = base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("page:%d", messagesRes.Pagination.Page+1)))
	}

	// Parse fields parameter
	fields := request.GetString("fields", "msgID,userUser,realName,channelID,text,time")
	requestedFields := sh.parseFields(fields)

	// Build result with metadata at the beginning (similar to channels_list and users_list)
	csvBytes, err := sh.marshalSearchMessagesWithFields(messages, requestedFields)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("Failed to format search results", err), nil
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("# Total messages: %d\n", messagesRes.Pagination.TotalCount))
	result.WriteString(fmt.Sprintf("# Total pages: %d\n", messagesRes.Pagination.PageCount))
	result.WriteString(fmt.Sprintf("# Current page: %d\n", messagesRes.Pagination.Page))
	result.WriteString(fmt.Sprintf("# Items per page: %d\n", messagesRes.Pagination.PerPage))
	result.WriteString(fmt.Sprintf("# Returned in this page: %d\n", len(messages)))
	if messagesRes.Pagination.First > 0 && messagesRes.Pagination.Last > 0 {
		result.WriteString(fmt.Sprintf("# Item range: %d-%d\n", messagesRes.Pagination.First, messagesRes.Pagination.Last))
	}
	if nextCursor != "" {
		result.WriteString(fmt.Sprintf("# Next cursor: %s\n", nextCursor))
	} else {
		result.WriteString("# Next cursor: (none - last page)\n")
	}
	result.Write(csvBytes)

	return mcp.NewToolResultText(result.String()), nil
}

func (sh *SearchHandler) convertMessagesFromSearch(slackMessages []slack.SearchMessage) []SearchMessage {
	usersMap := sh.apiProvider.ProvideUsersMap()
	var messages []SearchMessage
	warn := false

	for _, msg := range slackMessages {
		userName, realName, ok := getUserInfo(msg.User, usersMap.Users)

		if !ok && msg.User == "" && msg.Username != "" {
			userName, realName, ok = getBotInfo(msg.Username)
		} else if !ok {
			warn = true
		}

		threadTs, _ := extractThreadTS(msg.Permalink)

		timestamp, err := text.TimestampToIsoRFC3339(msg.Timestamp)
		if err != nil {
			sh.logger.Error("Failed to convert timestamp to RFC3339", zap.Error(err))
			continue
		}

		msgText := msg.Text + text.AttachmentsTo2CSV(msg.Text, msg.Attachments)

		messages = append(messages, SearchMessage{
			MsgID:     msg.Timestamp,
			UserID:    msg.User,
			UserName:  userName,
			RealName:  realName,
			Text:      text.ProcessText(msgText),
			Channel:   fmt.Sprintf("#%s", msg.Channel.Name),
			ThreadTs:  threadTs,
			Time:      timestamp,
			Reactions: "",
			Permalink: msg.Permalink,
		})
	}

	if ready, err := sh.apiProvider.IsReady(); !ready {
		if warn && errors.Is(err, provider.ErrUsersNotReady) {
			sh.logger.Warn(
				"Slack users sync not ready; you may see raw UIDs instead of names and lose some functionality.",
				zap.Error(err),
			)
		}
	}
	return messages
}

func (sh *SearchHandler) parseParamsToolSearch(req mcp.CallToolRequest) (*searchParams, error) {
	rawQuery := strings.TrimSpace(req.GetString("search_query", ""))
	freeText, filters := splitQuery(rawQuery)

	if req.GetBool("filter_threads_only", false) {
		addFilter(filters, "is", "thread")
	}
	if chName := req.GetString("filter_in_channel", ""); chName != "" {
		f, err := sh.paramFormatChannel(chName)
		if err != nil {
			sh.logger.Error("Invalid channel filter", zap.String("filter", chName), zap.Error(err))
			return nil, err
		}
		addFilter(filters, "in", f)
	} else if im := req.GetString("filter_in_im_or_mpim", ""); im != "" {
		f, err := sh.paramFormatUser(im)
		if err != nil {
			sh.logger.Error("Invalid IM/MPIM filter", zap.String("filter", im), zap.Error(err))
			return nil, err
		}
		addFilter(filters, "in", f)
	}
	if with := req.GetString("filter_users_with", ""); with != "" {
		f, err := sh.paramFormatUser(with)
		if err != nil {
			sh.logger.Error("Invalid with-user filter", zap.String("filter", with), zap.Error(err))
			return nil, err
		}
		addFilter(filters, "with", f)
	}
	if from := req.GetString("filter_users_from", ""); from != "" {
		f, err := sh.paramFormatUser(from)
		if err != nil {
			sh.logger.Error("Invalid from-user filter", zap.String("filter", from), zap.Error(err))
			return nil, err
		}
		addFilter(filters, "from", f)
	}

	dateMap, err := buildDateFilters(
		req.GetString("filter_date_before", ""),
		req.GetString("filter_date_after", ""),
		req.GetString("filter_date_on", ""),
		req.GetString("filter_date_during", ""),
	)
	if err != nil {
		sh.logger.Error("Invalid date filters", zap.Error(err))
		return nil, err
	}
	for key, val := range dateMap {
		addFilter(filters, key, val)
	}

	finalQuery := buildQuery(freeText, filters)
	limit := req.GetInt("limit", 100)
	cursor := req.GetString("cursor", "")
	sort := req.GetString("sort", "relevance")

	var (
		page          int
		decodedCursor []byte
	)
	if cursor != "" {
		decodedCursor, err = base64.StdEncoding.DecodeString(cursor)
		if err != nil {
			sh.logger.Error("Invalid cursor decoding", zap.String("cursor", cursor), zap.Error(err))
			return nil, fmt.Errorf("invalid cursor: %v", err)
		}
		parts := strings.Split(string(decodedCursor), ":")
		if len(parts) != 2 {
			sh.logger.Error("Invalid cursor format", zap.String("cursor", cursor))
			return nil, fmt.Errorf("invalid cursor: %v", cursor)
		}
		page, err = strconv.Atoi(parts[1])
		if err != nil || page < 1 {
			sh.logger.Error("Invalid cursor page", zap.String("cursor", cursor), zap.Error(err))
			return nil, fmt.Errorf("invalid cursor page: %v", err)
		}
	} else {
		page = 1
	}

	sh.logger.Debug("Search parameters built",
		zap.String("query", finalQuery),
		zap.Int("limit", limit),
		zap.Int("page", page),
		zap.String("sort", sort),
	)
	return &searchParams{
		query: finalQuery,
		limit: limit,
		page:  page,
		sort:  sort,
	}, nil
}

func (sh *SearchHandler) paramFormatUser(raw string) (string, error) {
	users := sh.apiProvider.ProvideUsersMap()
	raw = strings.TrimSpace(raw)

	// Handle DM channel IDs (format: D...)
	if strings.HasPrefix(raw, "D") {
		// For DM channels, we need to extract the user from the channel
		// The search API expects @username format for DMs
		cms := sh.apiProvider.ProvideChannelsMaps()
		if dm, ok := cms.Channels[raw]; ok {
			// For DMs, use the channel name (which is typically the username)
			return fmt.Sprintf("@%s", dm.Name), nil
		}
		return "", fmt.Errorf("DM channel %q not found or not accessible. Please verify the channel ID or try using @username instead", raw)
	}

	// Handle user IDs (format: U...)
	if strings.HasPrefix(raw, "U") {
		u, ok := users.Users[raw]
		if !ok {
			return "", fmt.Errorf("user ID %q not found. Please verify the user exists and is accessible", raw)
		}
		return fmt.Sprintf("<@%s>", u.ID), nil
	}

	// Strip @ prefix if present
	if strings.HasPrefix(raw, "<@") {
		raw = raw[2:]
	}
	if strings.HasPrefix(raw, "@") {
		raw = raw[1:]
	}

	// Look up by username
	uid, ok := users.UsersInv[raw]
	if !ok {
		// Provide helpful error message with suggestions
		return "", fmt.Errorf("username %q not found. Try using the full username, @username format, or the user/channel ID directly", raw)
	}
	return fmt.Sprintf("<@%s>", uid), nil
}

func (sh *SearchHandler) paramFormatChannel(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	cms := sh.apiProvider.ProvideChannelsMaps()

	// Handle #channel format
	if strings.HasPrefix(raw, "#") {
		if id, ok := cms.ChannelsInv[raw]; ok {
			return "#" + cms.Channels[id].Name, nil
		}
		return "", fmt.Errorf("channel %q not found. Please verify the channel name and that you have access to it", raw)
	}

	// Handle channel ID format (C...)
	if strings.HasPrefix(raw, "C") {
		if chn, ok := cms.Channels[raw]; ok {
			return "#" + chn.Name, nil
		}
		return "", fmt.Errorf("channel ID %q not found or not accessible. Please verify the channel ID is correct", raw)
	}

	// Invalid format - provide helpful message
	return "", fmt.Errorf("invalid channel format %q. Use #channel-name or channel ID (starting with C)", raw)
}

func extractThreadTS(rawurl string) (string, error) {
	u, err := url.Parse(rawurl)
	if err != nil {
		return "", err
	}
	return u.Query().Get("thread_ts"), nil
}

func parseFlexibleDate(dateStr string) (time.Time, string, error) {
	dateStr = strings.TrimSpace(dateStr)
	standardFormats := []string{
		"2006-01-02",      // YYYY-MM-DD
		"2006/01/02",      // YYYY/MM/DD
		"01-02-2006",      // MM-DD-YYYY
		"01/02/2006",      // MM/DD/YYYY
		"02-01-2006",      // DD-MM-YYYY
		"02/01/2006",      // DD/MM/YYYY
		"Jan 2, 2006",     // Jan 2, 2006
		"January 2, 2006", // January 2, 2006
		"2 Jan 2006",      // 2 Jan 2006
		"2 January 2006",  // 2 January 2006
	}
	for _, fmtStr := range standardFormats {
		if t, err := time.Parse(fmtStr, dateStr); err == nil {
			return t, t.Format("2006-01-02"), nil
		}
	}

	monthMap := map[string]int{
		"january": 1, "jan": 1,
		"february": 2, "feb": 2,
		"march": 3, "mar": 3,
		"april": 4, "apr": 4,
		"may":  5,
		"june": 6, "jun": 6,
		"july": 7, "jul": 7,
		"august": 8, "aug": 8,
		"september": 9, "sep": 9, "sept": 9,
		"october": 10, "oct": 10,
		"november": 11, "nov": 11,
		"december": 12, "dec": 12,
	}

	// Month-Year patterns
	monthYear := regexp.MustCompile(`^(\d{4})\s+([A-Za-z]+)$|^([A-Za-z]+)\s+(\d{4})$`)
	if m := monthYear.FindStringSubmatch(dateStr); m != nil {
		var year int
		var monStr string
		if m[1] != "" && m[2] != "" {
			year, _ = strconv.Atoi(m[1])
			monStr = strings.ToLower(m[2])
		} else {
			year, _ = strconv.Atoi(m[4])
			monStr = strings.ToLower(m[3])
		}
		if mon, ok := monthMap[monStr]; ok {
			t := time.Date(year, time.Month(mon), 1, 0, 0, 0, 0, time.UTC)
			return t, t.Format("2006-01-02"), nil
		}
	}

	// Day-Month-Year and Month-Day-Year patterns
	dmy1 := regexp.MustCompile(`^(\d{1,2})[-\s]+([A-Za-z]+)[-\s]+(\d{4})$`)
	if m := dmy1.FindStringSubmatch(dateStr); m != nil {
		day, _ := strconv.Atoi(m[1])
		year, _ := strconv.Atoi(m[3])
		monStr := strings.ToLower(m[2])
		if mon, ok := monthMap[monStr]; ok {
			t := time.Date(year, time.Month(mon), day, 0, 0, 0, 0, time.UTC)
			if t.Day() == day {
				return t, t.Format("2006-01-02"), nil
			}
		}
	}
	mdy := regexp.MustCompile(`^([A-Za-z]+)[-\s]+(\d{1,2})[-\s]+(\d{4})$`)
	if m := mdy.FindStringSubmatch(dateStr); m != nil {
		monStr := strings.ToLower(m[1])
		day, _ := strconv.Atoi(m[2])
		year, _ := strconv.Atoi(m[3])
		if mon, ok := monthMap[monStr]; ok {
			t := time.Date(year, time.Month(mon), day, 0, 0, 0, 0, time.UTC)
			if t.Day() == day {
				return t, t.Format("2006-01-02"), nil
			}
		}
	}
	ymd := regexp.MustCompile(`^(\d{4})[-\s]+([A-Za-z]+)[-\s]+(\d{1,2})$`)
	if m := ymd.FindStringSubmatch(dateStr); m != nil {
		year, _ := strconv.Atoi(m[1])
		monStr := strings.ToLower(m[2])
		day, _ := strconv.Atoi(m[3])
		if mon, ok := monthMap[monStr]; ok {
			t := time.Date(year, time.Month(mon), day, 0, 0, 0, 0, time.UTC)
			if t.Day() == day {
				return t, t.Format("2006-01-02"), nil
			}
		}
	}

	lower := strings.ToLower(dateStr)
	now := time.Now().UTC()
	switch lower {
	case "today":
		t := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		return t, t.Format("2006-01-02"), nil
	case "yesterday":
		t := now.AddDate(0, 0, -1)
		t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		return t, t.Format("2006-01-02"), nil
	case "tomorrow":
		t := now.AddDate(0, 0, 1)
		t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		return t, t.Format("2006-01-02"), nil
	}

	daysAgo := regexp.MustCompile(`^(\d+)\s+days?\s+ago$`)
	if m := daysAgo.FindStringSubmatch(lower); m != nil {
		days, _ := strconv.Atoi(m[1])
		t := now.AddDate(0, 0, -days)
		t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		return t, t.Format("2006-01-02"), nil
	}

	return time.Time{}, "", fmt.Errorf("unable to parse date: %s", dateStr)
}

func buildDateFilters(before, after, on, during string) (map[string]string, error) {
	out := make(map[string]string)
	if on != "" {
		if during != "" || before != "" || after != "" {
			return nil, fmt.Errorf("'on' cannot be combined with other date filters")
		}
		_, normalized, err := parseFlexibleDate(on)
		if err != nil {
			return nil, fmt.Errorf("invalid 'on' date: %v", err)
		}
		out["on"] = normalized
		return out, nil
	}
	if during != "" {
		if before != "" || after != "" {
			return nil, fmt.Errorf("'during' cannot be combined with 'before' or 'after'")
		}
		_, normalized, err := parseFlexibleDate(during)
		if err != nil {
			return nil, fmt.Errorf("invalid 'during' date: %v", err)
		}
		out["during"] = normalized
		return out, nil
	}
	if after != "" {
		_, normalized, err := parseFlexibleDate(after)
		if err != nil {
			return nil, fmt.Errorf("invalid 'after' date: %v", err)
		}
		out["after"] = normalized
	}
	if before != "" {
		_, normalized, err := parseFlexibleDate(before)
		if err != nil {
			return nil, fmt.Errorf("invalid 'before' date: %v", err)
		}
		out["before"] = normalized
	}
	if after != "" && before != "" {
		a, _, _ := parseFlexibleDate(after)
		b, _, _ := parseFlexibleDate(before)
		if a.After(b) {
			return nil, fmt.Errorf("'after' date is after 'before' date")
		}
	}
	return out, nil
}

func isFilterKey(key string) bool {
	_, ok := validFilterKeys[strings.ToLower(key)]
	return ok
}

func splitQuery(q string) (freeText []string, filters map[string][]string) {
	filters = make(map[string][]string)
	for _, tok := range strings.Fields(q) {
		parts := strings.SplitN(tok, ":", 2)
		if len(parts) == 2 && isFilterKey(parts[0]) {
			key := strings.ToLower(parts[0])
			filters[key] = append(filters[key], parts[1])
		} else {
			freeText = append(freeText, tok)
		}
	}
	return
}

func addFilter(filters map[string][]string, key, val string) {
	for _, existing := range filters[key] {
		if existing == val {
			return
		}
	}
	filters[key] = append(filters[key], val)
}

func buildQuery(freeText []string, filters map[string][]string) string {
	var out []string
	out = append(out, freeText...)
	for _, key := range []string{"is", "in", "from", "with", "before", "after", "on", "during"} {
		for _, val := range filters[key] {
			out = append(out, fmt.Sprintf("%s:%s", key, val))
		}
	}
	return strings.Join(out, " ")
}

func (sh *SearchHandler) parseFields(fields string) map[string]bool {
	requestedFields := make(map[string]bool)

	if fields == "all" {
		// Include all available fields
		requestedFields["msgID"] = true
		requestedFields["userID"] = true
		requestedFields["userUser"] = true
		requestedFields["realName"] = true
		requestedFields["channelID"] = true
		requestedFields["threadTs"] = true
		requestedFields["text"] = true
		requestedFields["time"] = true
		requestedFields["reactions"] = true
		requestedFields["permalink"] = true
	} else {
		// Parse comma-separated fields
		for _, field := range strings.Split(fields, ",") {
			field = strings.TrimSpace(field)
			if field != "" {
				requestedFields[field] = true
			}
		}
	}

	return requestedFields
}

func (sh *SearchHandler) marshalSearchMessagesWithFields(messages []SearchMessage, fields map[string]bool) ([]byte, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	// Define field order and headers
	possibleFields := []struct {
		key    string
		header string
	}{
		{"msgID", "MsgID"},
		{"userID", "UserID"},
		{"userUser", "UserName"},
		{"realName", "RealName"},
		{"channelID", "Channel"},
		{"threadTs", "ThreadTs"},
		{"text", "Text"},
		{"time", "Time"},
		{"reactions", "Reactions"},
		{"permalink", "Permalink"},
	}

	// Build headers based on requested fields
	var headers []string
	var fieldOrder []string
	for _, field := range possibleFields {
		if fields[field.key] {
			headers = append(headers, field.header)
			fieldOrder = append(fieldOrder, field.key)
		}
	}

	// Write headers
	if err := writer.Write(headers); err != nil {
		return nil, err
	}

	// Write data rows
	for _, msg := range messages {
		var row []string
		for _, field := range fieldOrder {
			switch field {
			case "msgID":
				row = append(row, msg.MsgID)
			case "userID":
				row = append(row, msg.UserID)
			case "userUser":
				row = append(row, msg.UserName)
			case "realName":
				row = append(row, msg.RealName)
			case "channelID":
				row = append(row, msg.Channel)
			case "threadTs":
				row = append(row, msg.ThreadTs)
			case "text":
				row = append(row, msg.Text)
			case "time":
				row = append(row, msg.Time)
			case "reactions":
				row = append(row, msg.Reactions)
			case "permalink":
				row = append(row, msg.Permalink)
			}
		}
		if err := writer.Write(row); err != nil {
			return nil, err
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func marshalSearchMessagesToCSVBytes(messages []SearchMessage) ([]byte, error) {
	return gocsv.MarshalBytes(&messages)
}
