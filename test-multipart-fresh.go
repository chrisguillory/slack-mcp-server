package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"time"
)

func main() {
	// Your tokens
	xoxcToken := "xoxc-8653483748308-9037654190502-9388340768309-9bc2a64319e3e1231afbf338b43a986de94b395ad2c5e99feac364d3bf629acb"
	xoxdToken := "xoxd-wfwZU80iiCnN02Xn7zkitbk4sLe9BLlBfHxt4v73P4tawqn2YkcLVQ8XCvqOznSbdNsH%2FrKJy8OhbKhudv3OIeDvjNXrs%2Fc33GETueDf4Xesl59OehnoX50iKXQUT7gjsuOT1z3oANGZYF%2F0jX1iqw25xMhQQo3pXcsCzPTe3YwagY%2BjrxRDhnDDjkVr8kmBgsE%2FKKOI8ikjxGsSD4dPKw55Fazf"
	teamID := "T08U80K08H4" // From your curl

	// IMPORTANT: You need to get fresh CSRF tokens from your browser!
	// Open Slack in browser, open DevTools Network tab, try to create a channel
	// Look for the conversations.create request and copy these values:

	// TODO: Replace these with FRESH values from your browser
	xID := "6b1e297c-1756862180.076"        // Get fresh _x_id from browser
	xCSID := "LhBhL26bLUA"                  // Get fresh _x_csid from browser
	slackRoute := "E08K7E7N092:E08K7E7N092" // Your Enterprise route

	channelName := fmt.Sprintf("test-multipart-%d", time.Now().Unix()%10000)

	// Build multipart body exactly like curl
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// IMPORTANT: Use a similar boundary format
	boundary := fmt.Sprintf("----WebKitFormBoundary%d", time.Now().UnixNano())
	writer.SetBoundary(boundary)

	// Add fields in exact order from curl
	writer.WriteField("token", xoxcToken)
	writer.WriteField("name", channelName)
	writer.WriteField("validate_name", "true")
	writer.WriteField("is_private", "true")
	writer.WriteField("team_id", teamID)
	writer.WriteField("_x_mode", "online")
	writer.WriteField("_x_sonic", "true")
	writer.WriteField("_x_app_name", "client")
	writer.Close()

	// Build URL with query params
	url := fmt.Sprintf(
		"https://mainstayio.enterprise.slack.com/api/conversations.create"+
			"?_x_id=%s&_x_csid=%s&slack_route=%s"+
			"&_x_version_ts=1756837958&_x_frontend_build_type=current"+
			"&_x_desktop_ia=4&_x_gantry=true&fp=39&_x_num_retries=0",
		xID, xCSID, slackRoute)

	req, err := http.NewRequest("POST", url, &body)
	if err != nil {
		log.Fatal(err)
	}

	// Set headers exactly like curl
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
	req.Header.Set("Cookie", fmt.Sprintf("d=%s", xoxdToken))
	req.Header.Set("Origin", "https://app.slack.com")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36")

	// Make request
	log.Printf("Creating channel: %s", channelName)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("Response: %s", string(respBody))
}
