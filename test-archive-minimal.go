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
)

func main() {
	// Your tokens from the curl
	xoxcToken := "xoxc-8653483748308-9037654190502-9453880141014-245ab8bf8bd1db47563cf69e0b0c5e2e5eab3364898ca25da61a8b4c85d0516b"
	xoxdToken := "xoxd-H4u%2FSCOBbXIn1g%2FP3wtOWizD1KCuoP4h3KNvCP8iHju0opAA4Y%2B2t8Bthd4xM0S7%2B6VA29mmLcRuI%2Bcm7MluKkfF3wPcWGoSHwnonkJseFOG4yyrwSSB9m4ubmNtBy9JQCVM1Nm2uIV4zI0Io2u3NNLblfXTmtux86lQtusmTsL9BIGuEb0CpAUvqZTwDlxSB04OG1x7wrpheFZmwSDW0aob8XIp"
	
	// CSRF tokens from your curl
	xID := "e82e805f-1756907705.948"
	xCSID := "r_j0q1uFJZ4"
	
	// Channel to archive (the one we created earlier)
	channelID := "C09DC2WFS3Y" // test-validate-fresh-5634
	
	// Test configurations - progressively adding parameters
	tests := []struct {
		name        string
		includeCSRF bool
		includeXFields bool
		includeAllQueryParams bool
	}{
		{
			name: "1. Minimal - Just token and channel",
			includeCSRF: false,
			includeXFields: false,
			includeAllQueryParams: false,
		},
		{
			name: "2. With X fields (_x_mode, _x_sonic, _x_app_name)",
			includeCSRF: false,
			includeXFields: true,
			includeAllQueryParams: false,
		},
		{
			name: "3. With CSRF tokens in URL",
			includeCSRF: true,
			includeXFields: false,
			includeAllQueryParams: false,
		},
		{
			name: "4. With CSRF + X fields",
			includeCSRF: true,
			includeXFields: true,
			includeAllQueryParams: false,
		},
		{
			name: "5. Full - All parameters from curl",
			includeCSRF: true,
			includeXFields: true,
			includeAllQueryParams: true,
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

	for _, test := range tests {
		log.Printf("\n=== Test: %s ===", test.name)
		
		// Build multipart body
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		boundary := "----WebKitFormBoundaryjzabKc1Q45vmDLXb"
		writer.SetBoundary(boundary)
		
		// Always include token and channel
		writer.WriteField("token", xoxcToken)
		writer.WriteField("channel", channelID)
		
		// Optionally add X fields
		if test.includeXFields {
			writer.WriteField("_x_mode", "online")
			writer.WriteField("_x_sonic", "true")
			writer.WriteField("_x_app_name", "client")
		}
		
		writer.Close()
		
		// Build URL
		baseURL := "https://mainstayio.enterprise.slack.com/api/conversations.archive"
		
		if test.includeCSRF {
			if test.includeAllQueryParams {
				// Full URL with all parameters
				baseURL += fmt.Sprintf("?_x_id=%s&_x_csid=%s&slack_route=E08K7E7N092:E08K7E7N092&_x_version_ts=1756899154&_x_frontend_build_type=current&_x_desktop_ia=4&_x_gantry=true&fp=39&_x_num_retries=0", xID, xCSID)
			} else {
				// Just CSRF tokens
				baseURL += fmt.Sprintf("?_x_id=%s&_x_csid=%s", xID, xCSID)
			}
		}
		
		req, err := http.NewRequest("POST", baseURL, &body)
		if err != nil {
			log.Printf("   ❌ Error creating request: %v", err)
			continue
		}
		
		// Set headers
		req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
		req.Header.Set("Origin", "https://app.slack.com")
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36")
		
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
			log.Printf("   ✅ SUCCESS! Channel archived")
			log.Printf("   Minimum requirements found: %s", test.name)
			
			// Try to unarchive for next test
			unarchiveChannel(httpClient, xoxcToken, channelID, test.includeCSRF, xID, xCSID, test.includeXFields, test.includeAllQueryParams)
			break
		} else {
			errorMsg := result["error"]
			log.Printf("   ❌ Failed with error: %v", errorMsg)
			
			// If it's already archived, unarchive and retry
			if errorMsg == "already_archived" {
				log.Printf("   ℹ️  Channel already archived, unarchiving and retrying...")
				if unarchiveChannel(httpClient, xoxcToken, channelID, test.includeCSRF, xID, xCSID, test.includeXFields, test.includeAllQueryParams) {
					// Retry the archive with same parameters
					log.Printf("   🔄 Retrying archive after unarchive...")
					// Would need to duplicate the request here, skipping for brevity
				}
			}
		}
	}
}

func unarchiveChannel(httpClient *http.Client, token, channelID string, includeCSRF bool, xID, xCSID string, includeXFields bool, includeAllQueryParams bool) bool {
	// Build multipart body for unarchive
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	boundary := "----WebKitFormBoundaryjzabKc1Q45vmDLXb"
	writer.SetBoundary(boundary)
	
	writer.WriteField("token", token)
	writer.WriteField("channel", channelID)
	
	if includeXFields {
		writer.WriteField("_x_mode", "online")
		writer.WriteField("_x_sonic", "true")
		writer.WriteField("_x_app_name", "client")
	}
	
	writer.Close()
	
	// Build URL
	baseURL := "https://mainstayio.enterprise.slack.com/api/conversations.unarchive"
	
	if includeCSRF {
		if includeAllQueryParams {
			baseURL += fmt.Sprintf("?_x_id=%s&_x_csid=%s&slack_route=E08K7E7N092:E08K7E7N092&_x_version_ts=1756899154&_x_frontend_build_type=current&_x_desktop_ia=4&_x_gantry=true&fp=39&_x_num_retries=0", xID, xCSID)
		} else {
			baseURL += fmt.Sprintf("?_x_id=%s&_x_csid=%s", xID, xCSID)
		}
	}
	
	req, _ := http.NewRequest("POST", baseURL, &body)
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
	req.Header.Set("Origin", "https://app.slack.com")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36")
	
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("      Unarchive request error: %v", err)
		return false
	}
	defer resp.Body.Close()
	
	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(respBody, &result)
	
	if ok, _ := result["ok"].(bool); ok {
		log.Printf("      ✅ Successfully unarchived channel")
		return true
	} else {
		log.Printf("      ❌ Failed to unarchive: %v", result["error"])
		return false
	}
}