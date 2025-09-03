package main

import (
	"context"
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

	httpClient := &http.Client{Jar: jar}
	api := slack.New(xoxcToken, slack.OptionHTTPClient(httpClient))

	// Get messages from a busy channel
	channelID := "C090W7FMCG7"

	params := &slack.GetConversationHistoryParameters{
		ChannelID: channelID,
		Limit:     50, // Get more messages to see variety
	}

	ctx := context.Background()
	history, err := api.GetConversationHistoryContext(ctx, params)

	if err != nil {
		log.Fatal(err)
	}

	// Count SubTypes
	subTypes := make(map[string]int)
	regularMessages := 0

	for _, msg := range history.Messages {
		if msg.SubType == "" {
			regularMessages++
		} else {
			subTypes[msg.SubType]++
		}
	}

	log.Printf("Total messages: %d", len(history.Messages))
	log.Printf("Regular messages (no SubType): %d", regularMessages)
	log.Println("\nSubType counts:")
	for subType, count := range subTypes {
		log.Printf("  %s: %d", subType, count)
	}

	// Show examples of each SubType
	log.Println("\nExamples of each SubType:")
	shown := make(map[string]bool)
	for _, msg := range history.Messages {
		if msg.SubType != "" && !shown[msg.SubType] {
			log.Printf("\n%s example:", msg.SubType)
			log.Printf("  User: %s", msg.User)
			log.Printf("  Text: %.100s", msg.Text)
			log.Printf("  BotID: %s", msg.BotID)
			shown[msg.SubType] = true
		}
	}
}
