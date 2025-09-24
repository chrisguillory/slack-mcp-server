package handler

import (
	"bytes"
	"encoding/csv"
	"strings"

	"github.com/gocarina/gocsv"
	"github.com/korotovsky/slack-mcp-server/pkg/provider"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/slack-go/slack"
)

// Common helper functions used across handlers

func getUserInfo(userID string, usersMap map[string]slack.User) (userName, realName string, ok bool) {
	if u, ok := usersMap[userID]; ok {
		return u.Name, u.RealName, true
	}
	return userID, userID, false
}

func getBotInfo(botID string, apiProvider *provider.ApiProvider) (slack.User, bool) {
	return apiProvider.ResolveBotIDToUser(botID)
}

func marshalMessagesToCSV(messages []Message) (*mcp.CallToolResult, error) {
	csvBytes, err := gocsv.MarshalBytes(&messages)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("Failed to format messages as CSV", err), nil
	}
	return mcp.NewToolResultText(string(csvBytes)), nil
}

func marshalMessagesToCSVBytes(messages []Message) ([]byte, error) {
	return gocsv.MarshalBytes(&messages)
}

// parseMessageFields parses the fields parameter for message-returning tools
func parseMessageFields(fields string) map[string]bool {
	requestedFields := make(map[string]bool)

	if fields == "" {
		// Return default fields
		fields = "msgID,userUser,realName,text,time"
	}

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
		requestedFields["cursor"] = true
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

// marshalMessagesWithFields marshals messages to CSV with only requested fields
func marshalMessagesWithFields(messages []Message, fields map[string]bool, includeCursor bool) ([]byte, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	// Define field order and headers
	possibleFields := []struct {
		key    string
		header string
	}{
		{"msgID", "MsgID"},
		{"userID", "UserID"},
		{"userUser", "UserUser"},
		{"realName", "RealName"},
		{"channelID", "ChannelID"},
		{"threadTs", "ThreadTs"},
		{"text", "Text"},
		{"time", "Time"},
		{"reactions", "Reactions"},
		{"cursor", "Cursor"},
	}

	// Build headers and track field order
	var headers []string
	var fieldOrder []string
	for _, field := range possibleFields {
		// Skip cursor field for replies (when includeCursor is false)
		if field.key == "cursor" && !includeCursor {
			continue
		}
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
			case "cursor":
				row = append(row, msg.Cursor)
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
