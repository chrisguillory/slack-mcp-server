package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"time"

	"github.com/slack-go/slack"
)

func main() {
	// Get tokens
	xoxcToken := "xoxc-8653483748308-9037654190502-9388340768309-9bc2a64319e3e1231afbf338b43a986de94b395ad2c5e99feac364d3bf629acb"
	xoxdToken := "xoxd-wfwZU80iiCnN02Xn7zkitbk4sLe9BLlBfHxt4v73P4tawqn2YkcLVQ8XCvqOznSbdNsH%2FrKJy8OhbKhudv3OIeDvjNXrs%2Fc33GETueDf4Xesl59OehnoX50iKXQUT7gjsuOT1z3oANGZYF%2F0jX1iqw25xMhQQo3pXcsCzPTe3YwagY%2BjrxRDhnDDjkVr8kmBgsE%2FKKOI8ikjxGsSD4dPKw55Fazf"

	// Create cookie jar
	jar, _ := cookiejar.New(nil)

	// Add xoxd cookie for Slack domains
	slackURL, _ := url.Parse("https://mainstayio.enterprise.slack.com")
	cookies := []*http.Cookie{
		{
			Name:   "d",
			Value:  xoxdToken,
			Path:   "/",
			Domain: ".slack.com",
		},
	}
	jar.SetCookies(slackURL, cookies)

	// Create HTTP client with cookie jar
	httpClient := &http.Client{
		Jar: jar,
	}

	// Create Slack client with xoxc token AND the cookie jar
	api := slack.New(xoxcToken,
		slack.OptionHTTPClient(httpClient),
		slack.OptionDebug(true),
		slack.OptionAPIURL("https://mainstayio.enterprise.slack.com/api/"),
	)

	// Test auth
	auth, err := api.AuthTest()
	if err != nil {
		log.Printf("Auth test failed: %v", err)
		// Continue anyway to see if channel creation works
	} else {
		log.Printf("Authenticated as %s in team %s (%s)", auth.User, auth.Team, auth.TeamID)
	}

	// Try creating a channel
	channelName := "test-cookies-" + fmt.Sprint(time.Now().Unix()%10000)

	params := slack.CreateConversationParams{
		ChannelName: channelName,
		IsPrivate:   true,
	}

	// Add TeamID if we have it
	if auth != nil && auth.TeamID != "" {
		params.TeamID = auth.TeamID
	} else {
		// Hardcode TeamID from your Enterprise Grid
		params.TeamID = "T08U80K08H4" // From your curl command
	}

	log.Printf("Attempting to create channel: %s with TeamID: %s", channelName, params.TeamID)

	ctx := context.Background()
	channel, err := api.CreateConversationContext(ctx, params)

	if err != nil {
		log.Printf("Failed to create channel: %v", err)

		// Try to get more details
		if slackErr, ok := err.(slack.SlackErrorResponse); ok {
			log.Printf("Slack error response: %+v", slackErr)
		}
	} else {
		log.Printf("Successfully created channel!")
		log.Printf("Channel ID: %s", channel.ID)
		log.Printf("Channel Name: %s", channel.Name)
	}
}
