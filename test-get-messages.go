package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"

	"github.com/slack-go/slack"
)

func main() {
	// Fresh tokens
	xoxcToken := "xoxc-8653483748308-9037654190502-9453880141014-245ab8bf8bd1db47563cf69e0b0c5e2e5eab3364898ca25da61a8b4c85d0516b"
	xoxdToken := "xoxd-hhns3gaLx3nR7QaJ8mlWdmcQHeF4NFq680kdYqdse3Dw0VonngUqDycuV8V0x0Jd%2BqCw68pYVwe4qMflSSCqDs9PArILT2j5peppx8oEbtJ2mX3dnECNaRdOOCauXiUE5FAAyGHpWJdaybuIPEuX11kXVqfRD6Fmdn6KydRYF%2F6coBActzL9JI7tfNZpZGZAa0xTEg4IVOwrkj0RBPC0j0oz132P"

	// Create cookie jar
	jar, _ := cookiejar.New(nil)
	slackURL, _ := url.Parse("https://slack.com")
	jar.SetCookies(slackURL, []*http.Cookie{
		{Name: "d", Value: xoxdToken, Path: "/", Domain: ".slack.com"},
	})

	// Create HTTP client with cookie jar
	httpClient := &http.Client{Jar: jar}

	// Create Slack client
	api := slack.New(xoxcToken, slack.OptionHTTPClient(httpClient))

	// Test channel ID from logs: C090W7FMCG7 (#market-intelligence-alerts)
	channelID := "C090W7FMCG7"

	log.Printf("Testing get messages for channel: %s", channelID)

	// Try to get messages
	params := &slack.GetConversationHistoryParameters{
		ChannelID: channelID,
		Limit:     5,
	}

	ctx := context.Background()
	history, err := api.GetConversationHistoryContext(ctx, params)

	if err != nil {
		log.Printf("Failed to get history: %v", err)
		return
	}

	log.Printf("Success! Got %d messages", len(history.Messages))
	log.Printf("HasMore: %v", history.HasMore)

	// Print details of each message
	for i, msg := range history.Messages {
		log.Printf("\n--- Message %d ---", i+1)
		log.Printf("Timestamp: %s", msg.Timestamp)
		log.Printf("User: %s", msg.User)
		log.Printf("Type: %s", msg.Type)
		log.Printf("SubType: %s", msg.SubType)
		log.Printf("Text: %.100s...", msg.Text) // First 100 chars

		// Check if timestamp is valid format
		if msg.Timestamp == "" {
			log.Printf("WARNING: Empty timestamp!")
		} else if len(msg.Timestamp) < 10 {
			log.Printf("WARNING: Timestamp too short: %s", msg.Timestamp)
		}

		// Print raw JSON for first message to debug
		if i == 0 {
			msgJSON, _ := json.MarshalIndent(msg, "", "  ")
			log.Printf("Full first message (JSON):\n%s", string(msgJSON))
		}
	}
}
