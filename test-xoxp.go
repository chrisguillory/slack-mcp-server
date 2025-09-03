package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/slack-go/slack"
)

func main() {
	// Replace with your xoxp token when you get it
	token := "xoxp-YOUR-TOKEN-HERE"

	// Create Slack client - no special HTTP client needed for xoxp!
	api := slack.New(token, slack.OptionDebug(false))

	// Test auth
	auth, err := api.AuthTest()
	if err != nil {
		log.Fatalf("Auth failed: %v", err)
	}

	log.Printf("Authenticated as %s in team %s (%s)", auth.User, auth.Team, auth.TeamID)

	// Try creating a channel
	channelName := fmt.Sprintf("test-xoxp-%d", time.Now().Unix()%10000)

	params := slack.CreateConversationParams{
		ChannelName: channelName,
		IsPrivate:   true,
	}

	// For Enterprise Grid, include TeamID
	if auth.TeamID != "" {
		params.TeamID = auth.TeamID
	}

	log.Printf("Creating channel: %s", channelName)

	ctx := context.Background()
	channel, err := api.CreateConversationContext(ctx, params)

	if err != nil {
		log.Printf("Failed: %v", err)
	} else {
		log.Printf("SUCCESS! Created channel %s (ID: %s)", channel.Name, channel.ID)
	}
}
