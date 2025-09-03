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
	"time"
)

func main() {
	// Your tokens
	xoxcToken := "xoxc-8653483748308-9037654190502-9453880141014-245ab8bf8bd1db47563cf69e0b0c5e2e5eab3364898ca25da61a8b4c85d0516b"
	xoxdToken := "xoxd-H4u%2FSCOBbXIn1g%2FP3wtOWizD1KCuoP4h3KNvCP8iHju0opAA4Y%2B2t8Bthd4xM0S7%2B6VA29mmLcRuI%2Bcm7MluKkfF3wPcWGoSHwnonkJseFOG4yyrwSSB9m4ubmNtBy9JQCVM1Nm2uIV4zI0Io2u3NNLblfXTmtux86lQtusmTsL9BIGuEb0CpAUvqZTwDlxSB04OG1x7wrpheFZmwSDW0aob8XIp"
	teamID := "T08U80K08H4"

	// CSRF tokens
	xID := "e82e805f-1756907705.948"
	xCSID := "r_j0q1uFJZ4"

	channelName := fmt.Sprintf("test-simple-%d", time.Now().Unix()%10000)

	// Test configurations
	tests := []struct {
		name          string
		includeCSRF   bool
		includeTeamID bool
	}{
		{
			name:          "1. Minimal - Just token + name (like slack-go/slack)",
			includeCSRF:   false,
			includeTeamID: false,
		},
		{
			name:          "2. Add team_id for Enterprise Grid",
			includeCSRF:   false,
			includeTeamID: true,
		},
		{
			name:          "3. Add CSRF tokens",
			includeCSRF:   true,
			includeTeamID: false,
		},
		{
			name:          "4. Add both team_id and CSRF",
			includeCSRF:   true,
			includeTeamID: true,
		},
	}

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

	for i, test := range tests {
		testChannelName := fmt.Sprintf("%s-%d", channelName, i)
		log.Printf("\n=== Test: %s ===", test.name)
		log.Printf("   Channel name: %s", testChannelName)

		// Build form data (URL-encoded) - like slack-go/slack
		formData := url.Values{
			"token":      {xoxcToken},
			"name":       {testChannelName},
			"is_private": {"true"},
		}

		if test.includeTeamID {
			formData.Set("team_id", teamID)
		}

		// Build URL
		baseURL := "https://mainstayio.enterprise.slack.com/api/conversations.create"

		if test.includeCSRF {
			baseURL += fmt.Sprintf("?_x_id=%s&_x_csid=%s", xID, xCSID)
		}

		// Create request with URL-encoded form data
		req, err := http.NewRequest("POST", baseURL, strings.NewReader(formData.Encode()))
		if err != nil {
			log.Printf("   ❌ Error creating request: %v", err)
			continue
		}

		// Set headers - minimal
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		// Make request
		resp, err := httpClient.Do(req)
		if err != nil {
			log.Printf("   ❌ Request error: %v", err)
			continue
		}
		defer resp.Body.Close()

		// Read response
		respBody, _ := io.ReadAll(resp.Body)

		// Parse response
		var result map[string]interface{}
		if err := json.Unmarshal(respBody, &result); err != nil {
			log.Printf("   ❌ Parse error: %v", err)
			log.Printf("   Raw response: %s", string(respBody))
			continue
		}

		// Check result
		if ok, _ := result["ok"].(bool); ok {
			log.Printf("   ✅ SUCCESS! Channel created")
			if channel, exists := result["channel"].(map[string]interface{}); exists {
				log.Printf("   Channel ID: %v", channel["id"])
			}
			log.Printf("   👉 Works with: %s", test.name)
		} else {
			errorMsg := result["error"]
			log.Printf("   ❌ Failed with error: %v", errorMsg)
		}
	}

	log.Printf("\n📊 Summary: Testing complete")
}
