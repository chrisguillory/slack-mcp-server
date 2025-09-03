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
	// Your tokens
	xoxcToken := "xoxc-8653483748308-9037654190502-9453880141014-245ab8bf8bd1db47563cf69e0b0c5e2e5eab3364898ca25da61a8b4c85d0516b"
	xoxdToken := "xoxd-H4u%2FSCOBbXIn1g%2FP3wtOWizD1KCuoP4h3KNvCP8iHju0opAA4Y%2B2t8Bthd4xM0S7%2B6VA29mmLcRuI%2Bcm7MluKkfF3wPcWGoSHwnonkJseFOG4yyrwSSB9m4ubmNtBy9JQCVM1Nm2uIV4zI0Io2u3NNLblfXTmtux86lQtusmTsL9BIGuEb0CpAUvqZTwDlxSB04OG1x7wrpheFZmwSDW0aob8XIp"

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

	// Test auth.test endpoint
	formData := url.Values{
		"token": {xoxcToken},
	}

	req, err := http.NewRequest("POST", "https://slack.com/api/auth.test", strings.NewReader(formData.Encode()))
	if err != nil {
		log.Fatal(err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	// Parse and pretty print
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		log.Fatal(err)
	}

	pretty, _ := json.MarshalIndent(result, "", "  ")
	fmt.Printf("auth.test response:\n%s\n", string(pretty))

	// Check specific fields
	fmt.Printf("\n📍 Key fields:\n")
	fmt.Printf("   team: %v\n", result["team"])
	fmt.Printf("   team_id: %v\n", result["team_id"])
	fmt.Printf("   url: %v\n", result["url"])
	fmt.Printf("   enterprise_id: %v\n", result["enterprise_id"])

	// Also try with Enterprise URL
	fmt.Printf("\n=== Testing with Enterprise URL ===\n")
	req2, _ := http.NewRequest("POST", "https://mainstayio.enterprise.slack.com/api/auth.test", strings.NewReader(formData.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp2, err := httpClient.Do(req2)
	if err != nil {
		log.Printf("Error: %v", err)
	} else {
		defer resp2.Body.Close()
		respBody2, _ := io.ReadAll(resp2.Body)
		var result2 map[string]interface{}
		json.Unmarshal(respBody2, &result2)
		pretty2, _ := json.MarshalIndent(result2, "", "  ")
		fmt.Printf("Enterprise auth.test response:\n%s\n", string(pretty2))
	}
}
