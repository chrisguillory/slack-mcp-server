package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/slack-go/slack"
)

func main() {
	// Get token from environment
	token := os.Getenv("SLACK_MCP_XOXC_TOKEN")
	if token == "" {
		token = "xoxc-8653483748308-9037654190502-9388340768309-9bc2a64319e3e1231afbf338b43a986de94b395ad2c5e99feac364d3bf629acb"
	}

	// Create Slack client with browser token
	api := slack.New(token, slack.OptionDebug(true))

	// Test auth first
	auth, err := api.AuthTest()
	if err != nil {
		log.Printf("Auth test failed: %v", err)
	} else {
		log.Printf("Authenticated as %s in team %s (%s)", auth.User, auth.Team, auth.TeamID)
	}

	// Try creating a channel using standard API
	channelName := "test-standard-api-" + fmt.Sprint(time.Now().Unix()%10000)

	params := slack.CreateConversationParams{
		ChannelName: channelName,
		IsPrivate:   true,
	}

	// For Enterprise Grid, add TeamID
	if auth != nil && auth.TeamID != "" {
		params.TeamID = auth.TeamID
		log.Printf("Using TeamID for Enterprise Grid: %s", auth.TeamID)
	}

	log.Printf("Attempting to create channel: %s", channelName)

	ctx := context.Background()
	channel, err := api.CreateConversationContext(ctx, params)

	if err != nil {
		log.Printf("Failed to create channel: %v", err)

		// Try to get more details about the error
		if slackErr, ok := err.(slack.SlackErrorResponse); ok {
			log.Printf("Slack error details: %+v", slackErr)
		}
	} else {
		log.Printf("Successfully created channel!")
		log.Printf("Channel ID: %s", channel.ID)
		log.Printf("Channel Name: %s", channel.Name)
	}
}
