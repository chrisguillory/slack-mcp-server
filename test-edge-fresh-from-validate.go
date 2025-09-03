package main

import (
	"bytes"
	"encoding/json"
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
	// Fresh tokens from your validateName curl
	xoxcToken := "xoxc-8653483748308-9037654190502-9453880141014-245ab8bf8bd1db47563cf69e0b0c5e2e5eab3364898ca25da61a8b4c85d0516b"
	xoxdToken := "xoxd-hhns3gaLx3nR7QaJ8mlWdmcQHeF4NFq680kdYqdse3Dw0VonngUqDycuV8V0x0Jd%2BqCw68pYVwe4qMflSSCqDs9PArILT2j5peppx8oEbtJ2mX3dnECNaRdOOCauXiUE5FAAyGHpWJdaybuIPEuX11kXVqfRD6Fmdn6KydRYF%2F6coBActzL9JI7tfNZpZGZAa0xTEg4IVOwrkj0RBPC0j0oz132P"
	teamID := "T08U80K08H4"

	// Fresh CSRF tokens from your validateName request
	xID := "e82e805f-1756905335.459"
	xCSID := "R-ia_gpEyrQ"

	channelName := fmt.Sprintf("test-validate-fresh-%d", time.Now().Unix()%10000)

	// First, let's test validateName to see if it works
	log.Printf("📋 Testing validateName first with channel: %s", channelName)
	if testValidateName(xoxcToken, xoxdToken, teamID, channelName, xID, xCSID) {
		log.Printf("✅ validateName succeeded! Name is available")
	} else {
		log.Printf("❌ validateName failed - name might be taken or invalid")
		return
	}

	// Now try to create the channel with the _x_reason field
	log.Printf("\n🚀 Now attempting to create channel: %s", channelName)

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

	// Build multipart body with _x_reason field
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	boundary := "----WebKitFormBoundaryALD2zjbhYWzpc5o0"
	writer.SetBoundary(boundary)

	// Add fields including the new _x_reason
	writer.WriteField("token", xoxcToken)
	writer.WriteField("name", channelName)
	writer.WriteField("validate_name", "true")
	writer.WriteField("is_private", "true")
	writer.WriteField("team_id", teamID)
	writer.WriteField("_x_reason", "create-channel") // NEW field from validateName
	writer.WriteField("_x_mode", "online")
	writer.WriteField("_x_sonic", "true")
	writer.WriteField("_x_app_name", "client")
	writer.Close()

	// Build URL with fresh CSRF tokens
	reqURL := fmt.Sprintf(
		"https://mainstayio.enterprise.slack.com/api/conversations.create"+
			"?_x_id=%s&_x_csid=%s&slack_route=%s:%s"+
			"&_x_version_ts=1756899154&_x_frontend_build_type=current"+
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

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("\n📦 Response: %s", string(respBody))

	// Parse JSON response
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err == nil {
		if ok, _ := result["ok"].(bool); ok {
			log.Printf("✅ SUCCESS! Channel created")
			if channel, exists := result["channel"].(map[string]interface{}); exists {
				log.Printf("   Channel ID: %v", channel["id"])
				log.Printf("   Channel Name: %v", channel["name"])
			}
		} else {
			log.Printf("❌ Failed with error: %v", result["error"])
		}
	}
}

func testValidateName(xoxcToken, xoxdToken, teamID, channelName, xID, xCSID string) bool {
	// Create cookie jar
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

	// Build multipart body for validateName
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	boundary := "----WebKitFormBoundarye2r501wHx3h3xl98"
	writer.SetBoundary(boundary)

	writer.WriteField("token", xoxcToken)
	writer.WriteField("name", channelName)
	writer.WriteField("team_id", teamID)
	writer.WriteField("_x_reason", "create-channel")
	writer.WriteField("_x_mode", "online")
	writer.WriteField("_x_sonic", "true")
	writer.WriteField("_x_app_name", "client")
	writer.Close()

	// Build validateName URL
	reqURL := fmt.Sprintf(
		"https://mainstayio.enterprise.slack.com/api/conversations.validateName"+
			"?_x_id=%s&_x_csid=%s&slack_route=%s:%s"+
			"&_x_version_ts=1756899154&_x_frontend_build_type=current"+
			"&_x_desktop_ia=4&_x_gantry=true&fp=39&_x_num_retries=0",
		xID, xCSID, teamID, teamID)

	req, err := http.NewRequest("POST", reqURL, &body)
	if err != nil {
		log.Printf("Error creating request: %v", err)
		return false
	}

	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
	req.Header.Set("Origin", "https://app.slack.com")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36")

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("Error making request: %v", err)
		return false
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("   validateName response: %s", string(respBody))

	// Check if successful
	return bytes.Contains(respBody, []byte(`"ok":true`))
}
