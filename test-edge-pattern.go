package main

import (
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
	// Fresh tokens
	xoxcToken := "xoxc-8653483748308-9037654190502-9438424531719-96058f11dd6e910ce3ecae9f9e63a5186b4d6b747b00627b93e163bda32030c2"
	xoxdToken := "xoxd-oPOW4kHLMGMuBZQipjXPR5%2BWNoGlFSRA4wkDAzS4mo7xfWLtsT54VPz%2B5UCDi7dDayoR1q4XxAWcadE%2F%2Fux5zoOf6uOtKPT7sPsEmU71CbEaRc227oqjwdFNyCUAf7IGowkUQDDawS%2FPYMtLNuigCDBYoiSolKiq2tjPb8%2BtNGQgg81EWKb22hI0ji8dOBK2WRdeGqTcjt9hWXxLWlWcGpRnwQYL"

	channelName := fmt.Sprintf("test-edge-%d", time.Now().Unix()%10000)

	// Method 1: URL-encoded form (like reactions.add)
	log.Println("=== Testing URL-encoded form (Edge client pattern) ===")

	// Build form data like reactions.add does
	formData := url.Values{}
	formData.Set("token", xoxcToken)
	formData.Set("name", channelName)
	formData.Set("validate_name", "true")
	formData.Set("is_private", "true")
	formData.Set("team_id", "T08U80K08H4")
	formData.Set("_x_reason", "conversations-view/createChannel")
	formData.Set("_x_mode", "online")
	formData.Set("_x_sonic", "true")
	formData.Set("_x_app_name", "client")

	// Create cookie jar
	jar, _ := cookiejar.New(nil)
	slackURL, _ := url.Parse("https://mainstayio.enterprise.slack.com")
	jar.SetCookies(slackURL, []*http.Cookie{
		{Name: "d", Value: xoxdToken, Path: "/", Domain: ".slack.com"},
	})

	client := &http.Client{Jar: jar}

	// Make request with URL-encoded body (no CSRF tokens in URL)
	reqURL := "https://mainstayio.enterprise.slack.com/api/conversations.create"

	req, _ := http.NewRequest("POST", reqURL, strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("Response: %s\n", string(body))

	// Method 2: Try with different _x_reason values
	log.Println("\n=== Testing with different _x_reason ===")

	reasons := []string{
		"changeReactionFromUserAction", // What reactions uses
		"conversations-view",
		"channel_browser",
		"shortcuts_menu",
	}

	for _, reason := range reasons {
		formData.Set("_x_reason", reason)
		formData.Set("name", fmt.Sprintf("%s-%s", channelName, strings.ReplaceAll(reason, "/", "-")))

		req, _ := http.NewRequest("POST", reqURL, strings.NewReader(formData.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36")

		resp, err := client.Do(req)
		if err != nil {
			log.Printf("Error with reason %s: %v", reason, err)
			continue
		}

		body, _ := io.ReadAll(resp.Body)
		log.Printf("Reason '%s': %s", reason, string(body))
		resp.Body.Close()
	}
}
