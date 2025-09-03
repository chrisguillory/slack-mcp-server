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
	// Fresh tokens from your .envrc-personal
	xoxcToken := "xoxc-8653483748308-9037654190502-9438424531719-96058f11dd6e910ce3ecae9f9e63a5186b4d6b747b00627b93e163bda32030c2"
	xoxdToken := "xoxd-oPOW4kHLMGMuBZQipjXPR5%2BWNoGlFSRA4wkDAzS4mo7xfWLtsT54VPz%2B5UCDi7dDayoR1q4XxAWcadE%2F%2Fux5zoOf6uOtKPT7sPsEmU71CbEaRc227oqjwdFNyCUAf7IGowkUQDDawS%2FPYMtLNuigCDBYoiSolKiq2tjPb8%2BtNGQgg81EWKb22hI0ji8dOBK2WRdeGqTcjt9hWXxLWlWcGpRnwQYL"

	// Create cookie jar with xoxd
	jar, _ := cookiejar.New(nil)
	slackURL, _ := url.Parse("https://slack.com")
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

	// Create Slack client with fresh tokens
	api := slack.New(xoxcToken,
		slack.OptionHTTPClient(httpClient),
		slack.OptionDebug(false),
	)

	// Test auth first
	auth, err := api.AuthTest()
	if err != nil {
		log.Printf("Auth test failed: %v", err)
		log.Printf("Continuing anyway to test channel creation...")
	} else {
		log.Printf("✅ AUTH SUCCESS!")
		log.Printf("   User: %s", auth.User)
		log.Printf("   Team: %s", auth.Team)
		log.Printf("   TeamID: %s", auth.TeamID)
		log.Printf("   URL: %s", auth.URL)
		if auth.EnterpriseID != "" {
			log.Printf("   EnterpriseID: %s", auth.EnterpriseID)
		}
	}

	// Try creating a channel with fresh tokens
	channelName := fmt.Sprintf("test-fresh-%d", time.Now().Unix()%10000)

	log.Printf("\n🔨 Attempting to create channel: %s", channelName)

	// Test 1: Private channel with TeamID
	params1 := slack.CreateConversationParams{
		ChannelName: channelName + "-private",
		IsPrivate:   true,
	}
	if auth != nil && auth.TeamID != "" {
		params1.TeamID = auth.TeamID
	} else {
		params1.TeamID = "T08U80K08H4" // Fallback to known TeamID
	}

	ctx := context.Background()
	channel, err := api.CreateConversationContext(ctx, params1)

	if err != nil {
		log.Printf("❌ Private channel failed: %v", err)
	} else {
		log.Printf("✅ SUCCESS! Created private channel: %s (ID: %s)", channel.Name, channel.ID)
	}

	// Test 2: Public channel
	params2 := slack.CreateConversationParams{
		ChannelName: channelName + "-public",
		IsPrivate:   false,
		TeamID:      params1.TeamID,
	}

	channel2, err := api.CreateConversationContext(ctx, params2)

	if err != nil {
		log.Printf("❌ Public channel failed: %v", err)
	} else {
		log.Printf("✅ SUCCESS! Created public channel: %s (ID: %s)", channel2.Name, channel2.ID)
	}
}
