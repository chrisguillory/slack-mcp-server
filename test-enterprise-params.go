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
	// Tokens from your environment
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

	// Create Slack client
	api := slack.New(xoxcToken,
		slack.OptionHTTPClient(httpClient),
		slack.OptionDebug(false), // Turn off debug for cleaner output
	)

	// Get auth info
	auth, err := api.AuthTest()
	if err != nil {
		log.Printf("Auth test failed: %v", err)
		return
	}

	log.Printf("Authenticated as %s in team %s", auth.User, auth.Team)
	log.Printf("TeamID: %s", auth.TeamID)
	log.Printf("EnterpriseID: %s", auth.EnterpriseID)
	log.Printf("URL: %s", auth.URL)

	// Try different parameter combinations based on Perplexity's research
	channelName := fmt.Sprintf("test-params-%d", time.Now().Unix()%10000)

	// Test 1: With team_id (workspace ID, not organization ID)
	log.Println("\n=== Test 1: Using workspace TeamID ===")
	params1 := slack.CreateConversationParams{
		ChannelName: channelName + "-1",
		IsPrivate:   false,         // Try public channel first
		TeamID:      "T08U80K08H4", // Your workspace ID from curl
	}
	testCreate(api, params1)

	// Test 2: Using the Enterprise ID as TeamID
	log.Println("\n=== Test 2: Using EnterpriseID as TeamID ===")
	params2 := slack.CreateConversationParams{
		ChannelName: channelName + "-2",
		IsPrivate:   false,
		TeamID:      auth.EnterpriseID, // Try enterprise ID
	}
	testCreate(api, params2)

	// Test 3: Try with minimal params (no team_id)
	log.Println("\n=== Test 3: No TeamID specified ===")
	params3 := slack.CreateConversationParams{
		ChannelName: channelName + "-3",
		IsPrivate:   false,
	}
	testCreate(api, params3)

	// Test 4: Try admin endpoint (if available)
	log.Println("\n=== Test 4: Try admin.conversations.create endpoint ===")
	// Note: This likely won't work with browser tokens, but worth trying
	testAdminCreate(api, channelName+"-admin")
}

func testCreate(api *slack.Client, params slack.CreateConversationParams) {
	log.Printf("Attempting to create channel: %s (private=%v, team_id=%s)",
		params.ChannelName, params.IsPrivate, params.TeamID)

	ctx := context.Background()
	channel, err := api.CreateConversationContext(ctx, params)

	if err != nil {
		log.Printf("❌ Failed: %v", err)
		return
	}

	log.Printf("✅ Success! Channel ID: %s, Name: %s", channel.ID, channel.Name)
}

func testAdminCreate(api *slack.Client, channelName string) {
	log.Printf("Attempting admin.conversations.create for channel: %s", channelName)

	// The slack-go library might not have this method, but we can try
	// This is a placeholder - would need custom implementation
	log.Printf("⚠️  Admin endpoint not available in standard slack-go library")
}
