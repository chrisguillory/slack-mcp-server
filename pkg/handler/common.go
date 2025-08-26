package handler

import (
	"github.com/gocarina/gocsv"
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

func getBotInfo(botID string) (userName, realName string, ok bool) {
	return botID, botID, true
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
