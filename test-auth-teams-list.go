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
	// Fresh tokens
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

	fmt.Println("=== Testing auth.teams.list ===")
	fmt.Println("This should return all workspaces (Team IDs) within the Enterprise organization")

	// Try auth.teams.list
	formData := url.Values{
		"token": {xoxcToken},
	}

	// Try both standard and Enterprise URLs
	urls := []string{
		"https://slack.com/api/auth.teams.list",
		"https://mainstayio.enterprise.slack.com/api/auth.teams.list",
	}

	for _, apiURL := range urls {
		fmt.Printf("\n📍 Trying: %s\n", apiURL)

		req, err := http.NewRequest("POST", apiURL, strings.NewReader(formData.Encode()))
		if err != nil {
			log.Printf("Error creating request: %v", err)
			continue
		}

		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := httpClient.Do(req)
		if err != nil {
			log.Printf("Error making request: %v", err)
			continue
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)

		// Parse response
		var result map[string]interface{}
		if err := json.Unmarshal(respBody, &result); err != nil {
			log.Printf("Parse error: %v", err)
			fmt.Printf("Raw response: %s\n", string(respBody))
			continue
		}

		// Pretty print
		pretty, _ := json.MarshalIndent(result, "", "  ")
		fmt.Printf("Response:\n%s\n", string(pretty))

		// Check if successful and extract teams
		if ok, _ := result["ok"].(bool); ok {
			fmt.Println("\n✅ SUCCESS! Found workspaces:")
			if teams, exists := result["teams"].([]interface{}); exists {
				for _, team := range teams {
					if t, ok := team.(map[string]interface{}); ok {
						fmt.Printf("  Team ID: %v, Name: %v\n", t["id"], t["name"])
					}
				}
			}
		} else {
			fmt.Printf("❌ Error: %v\n", result["error"])
		}
	}
}
