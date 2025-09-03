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

	// First, get the current user ID from auth.test
	fmt.Println("=== Getting current user ID from auth.test ===")
	formData := url.Values{
		"token": {xoxcToken},
	}

	req, _ := http.NewRequest("POST", "https://slack.com/api/auth.test", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var authResult map[string]interface{}
	json.Unmarshal(respBody, &authResult)

	userID, _ := authResult["user_id"].(string)
	fmt.Printf("Current user ID: %s\n", userID)

	// Now get the user info with enterprise_user details
	fmt.Println("\n=== Getting user info with enterprise_user.teams ===")

	formData2 := url.Values{
		"token": {xoxcToken},
		"user":  {userID},
	}

	// Try both endpoints
	urls := []string{
		"https://slack.com/api/users.info",
		"https://mainstayio.enterprise.slack.com/api/users.info",
	}

	for _, apiURL := range urls {
		fmt.Printf("\n📍 Trying: %s\n", apiURL)

		req2, _ := http.NewRequest("POST", apiURL, strings.NewReader(formData2.Encode()))
		req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp2, err := httpClient.Do(req2)
		if err != nil {
			log.Printf("Error: %v", err)
			continue
		}
		defer resp2.Body.Close()

		respBody2, _ := io.ReadAll(resp2.Body)
		var result map[string]interface{}
		json.Unmarshal(respBody2, &result)

		if ok, _ := result["ok"].(bool); ok {
			if user, exists := result["user"].(map[string]interface{}); exists {
				fmt.Printf("User name: %v\n", user["name"])

				// Check for enterprise_user field
				if enterpriseUser, exists := user["enterprise_user"].(map[string]interface{}); exists {
					fmt.Printf("\n✅ Found enterprise_user:\n")
					fmt.Printf("  Enterprise ID: %v\n", enterpriseUser["enterprise_id"])
					fmt.Printf("  Enterprise Name: %v\n", enterpriseUser["enterprise_name"])
					fmt.Printf("  Is Admin: %v\n", enterpriseUser["is_admin"])
					fmt.Printf("  Is Owner: %v\n", enterpriseUser["is_owner"])

					// The crucial part - teams array
					if teams, exists := enterpriseUser["teams"].([]interface{}); exists {
						fmt.Printf("  Teams: %v\n", teams)
						if len(teams) > 0 {
							fmt.Printf("\n🎯 FOUND TEAM ID: %v\n", teams[0])
							fmt.Printf("   This is what we need for conversations.create!\n")
						}
					} else {
						fmt.Printf("  Teams field not found or empty\n")
					}
				} else {
					fmt.Printf("No enterprise_user field found\n")
				}
			}
		} else {
			fmt.Printf("Error: %v\n", result["error"])
		}
	}
}
