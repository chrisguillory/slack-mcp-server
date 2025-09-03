package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"time"
)

func main() {
	// Your fresh tokens
	xoxcToken := "xoxc-8653483748308-9037654190502-9438424531719-96058f11dd6e910ce3ecae9f9e63a5186b4d6b747b00627b93e163bda32030c2"
	xoxdToken := "xoxd-oPOW4kHLMGMuBZQipjXPR5%2BWNoGlFSRA4wkDAzS4mo7xfWLtsT54VPz%2B5UCDi7dDayoR1q4XxAWcadE%2F%2Fux5zoOf6uOtKPT7sPsEmU71CbEaRc227oqjwdFNyCUAf7IGowkUQDDawS%2FPYMtLNuigCDBYoiSolKiq2tjPb8%2BtNGQgg81EWKb22hI0ji8dOBK2WRdeGqTcjt9hWXxLWlWcGpRnwQYL"
	teamID := "T08U80K08H4"

	// IMPORTANT: Get FRESH CSRF tokens from browser!
	// Instructions:
	// 1. Open Slack in browser (app.slack.com)
	// 2. Open DevTools Network tab
	// 3. Try to create a channel manually
	// 4. Look for conversations.create request
	// 5. Copy these values from the Request URL:

	// TODO: Replace with FRESH values from browser
	xID := "REPLACE_WITH_FRESH_X_ID"     // e.g., "6b1e297c-1756862180.076"
	xCSID := "REPLACE_WITH_FRESH_X_CSID" // e.g., "LhBhL26bLUA"

	if xID == "REPLACE_WITH_FRESH_X_ID" {
		log.Fatal("⚠️  STOP! You need to get fresh CSRF tokens from your browser first!")
	}

	channelName := fmt.Sprintf("test-edge-csrf-%d", time.Now().Unix()%10000)

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

	// Build multipart body
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	boundary := "----WebKitFormBoundaryALD2zjbhYWzpc5o0"
	writer.SetBoundary(boundary)

	// Add fields in exact order
	writer.WriteField("token", xoxcToken)
	writer.WriteField("name", channelName)
	writer.WriteField("validate_name", "true")
	writer.WriteField("is_private", "true")
	writer.WriteField("team_id", teamID)
	writer.WriteField("_x_mode", "online")
	writer.WriteField("_x_sonic", "true")
	writer.WriteField("_x_app_name", "client")
	writer.Close()

	// Build URL with FRESH CSRF tokens
	reqURL := fmt.Sprintf(
		"https://mainstayio.enterprise.slack.com/api/conversations.create"+
			"?_x_id=%s&_x_csid=%s&slack_route=%s:%s"+
			"&_x_version_ts=1756837958&_x_frontend_build_type=current"+
			"&_x_desktop_ia=4&_x_gantry=true&fp=39&_x_num_retries=0",
		xID, xCSID, teamID, teamID)

	req, err := http.NewRequest("POST", reqURL, &body)
	if err != nil {
		log.Fatal(err)
	}

	// Set headers
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
	req.Header.Set("Origin", "https://app.slack.com")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36")

	// Make request
	log.Printf("🚀 Attempting to create channel: %s", channelName)
	log.Printf("   Using fresh CSRF tokens: _x_id=%s, _x_csid=%s", xID, xCSID)

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("\n📦 Response: %s", string(respBody))

	// Check if successful
	if bytes.Contains(respBody, []byte(`"ok":true`)) {
		log.Printf("✅ SUCCESS! Channel created")
	} else {
		log.Printf("❌ Failed. Response: %s", string(respBody))
	}
}
