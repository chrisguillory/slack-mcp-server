package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
)

func main() {
	// Your tokens (fresh)
	xoxcToken := "xoxc-8653483748308-9037654190502-9480507636960-ef5ce9c5342c8921fba250e10ced301cd3b9516f88f4cd0da27fa175f1b83350"
	xoxdToken := "xoxd-G2y4Srg0y9x7mpZmMv1RSaxtbnXouUZc9UxHV9iqKFpjRt2enJ8IgP5DE8IjgG27P4KefzXkxM3y3hn1kbD88jqlzLaSR95IOVrpSAgWBu1hCF8L36Zo1WfUlB05tUZqGd5DR3ExHAxkEese4VG9NFneK27rgs9raOG5enRwzNkdejyluMRPjt2tJci5mjAUoDqCibDDlGM5mDf2HQnBY0OyI6kV"

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

	httpClient := &http.Client{
		Jar: jar,
	}

	// 1. Get list of channels to find context_team_id
	fmt.Println("=== Getting channel list to find context_team_id ===")

	formData := url.Values{
		"token":   {xoxcToken},
		"limit":   {"10"},
		"types":   {"public_channel,private_channel"},
		"team_id": {"T08U80K08H4"}, // Add the known team_id
	}

	req, _ := http.NewRequest("POST", "https://mainstayio.enterprise.slack.com/api/conversations.list", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(respBody, &result)

	if channels, ok := result["channels"].([]interface{}); ok && len(channels) > 0 {
		fmt.Printf("Found %d channels\n", len(channels))
		for i, ch := range channels {
			if channel, ok := ch.(map[string]interface{}); ok {
				fmt.Printf("\nChannel %d:\n", i+1)
				fmt.Printf("  Name: %v\n", channel["name"])
				fmt.Printf("  ID: %v\n", channel["id"])
				fmt.Printf("  context_team_id: %v\n", channel["context_team_id"])
				fmt.Printf("  shared_team_ids: %v\n", channel["shared_team_ids"])
				if i >= 2 {
					break // Just show first 3
				}
			}
		}
	} else {
		fmt.Printf("No channels found or error: %v\n", result["error"])
	}

	// 2. Try conversations.info to get more details
	fmt.Println("\n=== Getting specific channel info ===")

	// Use a known channel ID
	channelID := "C09H29FR41E" // #general or use one from above

	formData2 := url.Values{
		"token":   {xoxcToken},
		"channel": {channelID},
	}

	req2, _ := http.NewRequest("POST", "https://mainstayio.enterprise.slack.com/api/conversations.info", strings.NewReader(formData2.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp2, err := httpClient.Do(req2)
	if err != nil {
		log.Printf("Error: %v", err)
	} else {
		defer resp2.Body.Close()
		respBody2, _ := io.ReadAll(resp2.Body)
		var result2 map[string]interface{}
		json.Unmarshal(respBody2, &result2)

		if channel, ok := result2["channel"].(map[string]interface{}); ok {
			fmt.Printf("Channel details for %s:\n", channelID)
			fmt.Printf("  name: %v\n", channel["name"])
			fmt.Printf("  context_team_id: %v\n", channel["context_team_id"])
			fmt.Printf("  shared_team_ids: %v\n", channel["shared_team_ids"])

			// Extract the Team ID
			if contextTeamID, ok := channel["context_team_id"].(string); ok && contextTeamID != "" {
				fmt.Printf("\n✅ Found Team ID: %s\n", contextTeamID)
				fmt.Printf("   This is what should be used for conversations.create!\n")
			}
		}
	}
}
