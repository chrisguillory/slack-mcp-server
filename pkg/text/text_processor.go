package text

import (
	"crypto/x509"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/slack-go/slack"
	"go.uber.org/zap"
	"golang.org/x/net/publicsuffix"
)

func AttachmentToText(att slack.Attachment) string {
	var parts []string

	if att.Title != "" {
		parts = append(parts, fmt.Sprintf("Title: %s", att.Title))
	}

	if att.AuthorName != "" {
		parts = append(parts, fmt.Sprintf("Author: %s", att.AuthorName))
	}

	if att.Pretext != "" {
		parts = append(parts, fmt.Sprintf("Pretext: %s", att.Pretext))
	}

	if att.Text != "" {
		parts = append(parts, fmt.Sprintf("Text: %s", att.Text))
	}

	if att.Footer != "" {
		ts, _ := TimestampToIsoRFC3339(string(att.Ts) + ".000000")

		parts = append(parts, fmt.Sprintf("Footer: %s @ %s", att.Footer, ts))
	}

	result := strings.Join(parts, "; ")

	result = strings.ReplaceAll(result, "\n", " ")
	result = strings.ReplaceAll(result, "\r", " ")
	result = strings.ReplaceAll(result, "\t", " ")
	result = strings.ReplaceAll(result, "(", "[")
	result = strings.ReplaceAll(result, ")", "]")
	result = strings.TrimSpace(result)

	return result
}

func AttachmentsTo2CSV(msgText string, attachments []slack.Attachment) string {
	if len(attachments) == 0 {
		return ""
	}

	var descriptions []string
	for _, att := range attachments {
		plainText := AttachmentToText(att)
		if plainText != "" {
			descriptions = append(descriptions, fmt.Sprintf("%s", plainText))
		}
	}

	prefix := ""
	if msgText != "" {
		prefix = ". "
	}

	return prefix + strings.Join(descriptions, ", ")
}

func IsUnfurlingEnabled(text string, opt string, logger *zap.Logger) bool {
	if opt == "" || opt == "no" || opt == "false" || opt == "0" {
		return false
	}

	if opt == "yes" || opt == "true" || opt == "1" {
		return true
	}

	allowed := make(map[string]struct{}, 0)
	for _, d := range strings.Split(opt, ",") {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" {
			continue
		}
		allowed[d] = struct{}{}
	}

	urlRe := regexp.MustCompile(`https?://[^\s]+`)
	urls := urlRe.FindAllString(text, -1)
	for _, rawURL := range urls {
		u, err := url.Parse(rawURL)
		if err != nil || u.Host == "" {
			continue
		}
		host := strings.ToLower(u.Host)
		if idx := strings.Index(host, ":"); idx != -1 {
			host = host[:idx]
		}
		host = strings.TrimPrefix(host, "www.")
		if _, ok := allowed[host]; !ok {
			if logger != nil {
				logger.Warn("Security: attempt to unfurl non-whitelisted host",
					zap.String("host", host),
					zap.String("allowed", opt),
				)
			}
			return false
		}
	}

	txtNoURLs := urlRe.ReplaceAllString(text, " ")

	domRe := regexp.MustCompile(`\b(?:[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?\.)+[A-Za-z]{2,}\b`)
	doms := domRe.FindAllString(txtNoURLs, -1)

	for _, d := range doms {
		d = strings.ToLower(d)

		if _, icann := publicsuffix.PublicSuffix(d); !icann {
			continue
		}

		if _, ok := allowed[d]; !ok {
			if logger != nil {
				logger.Warn("Security: attempt to unfurl non-whitelisted host",
					zap.String("host", d),
					zap.String("allowed", opt),
				)
			}
			return false
		}
	}

	return true
}

func Workspace(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	host := u.Hostname()
	parts := strings.Split(host, ".")
	if len(parts) < 3 {
		return "", fmt.Errorf("invalid Slack URL: %q", rawURL)
	}
	return parts[0], nil
}

func TimestampToIsoRFC3339(slackTS string) (string, error) {
	parts := strings.Split(slackTS, ".")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid slack timestamp format: %s", slackTS)
	}

	seconds, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return "", fmt.Errorf("failed to parse seconds: %v", err)
	}

	microseconds, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", fmt.Errorf("failed to parse microseconds: %v", err)
	}

	t := time.Unix(seconds, microseconds*1000)

	return t.UTC().Format(time.RFC3339), nil
}

func ProcessText(s string) string {
	s = filterSpecialChars(s)

	return s
}

func HumanizeCertificates(certs []*x509.Certificate) string {
	var descriptions []string
	for _, cert := range certs {
		subjectCN := cert.Subject.CommonName
		issuerCN := cert.Issuer.CommonName
		expiry := cert.NotAfter.Format("2006-01-02")

		description := fmt.Sprintf("CN=%s (Issuer CN=%s, expires %s)", subjectCN, issuerCN, expiry)
		descriptions = append(descriptions, description)
	}
	return strings.Join(descriptions, ", ")
}

// BlocksToText extracts text content from Slack Block Kit blocks
func BlocksToText(blocks slack.Blocks) string {
	if len(blocks.BlockSet) == 0 {
		return ""
	}

	var textParts []string

	for _, block := range blocks.BlockSet {
		blockText := extractTextFromBlock(block)
		if blockText != "" {
			textParts = append(textParts, blockText)
		}
	}

	if len(textParts) == 0 {
		return ""
	}

	// Join with period and space to maintain readability
	return ". " + strings.Join(textParts, ". ")
}

// extractTextFromBlock extracts text from a single block based on its type
func extractTextFromBlock(block slack.Block) string {
	if block == nil {
		return ""
	}

	switch b := block.(type) {
	case *slack.SectionBlock:
		return extractFromSectionBlock(b)
	case *slack.ContextBlock:
		return extractFromContextBlock(b)
	case *slack.RichTextBlock:
		return extractFromRichTextBlock(b)
	case *slack.HeaderBlock:
		return extractFromHeaderBlock(b)
	// PlainTextInputBlock not available in this version of slack-go library
	case *slack.ActionBlock:
		// Action blocks contain interactive elements that might have text
		return extractFromActionBlock(b)
	}

	return ""
}

// extractFromSectionBlock extracts text from a section block
func extractFromSectionBlock(block *slack.SectionBlock) string {
	if block == nil {
		return ""
	}

	var parts []string

	// Extract main text
	if block.Text != nil {
		if text := extractFromTextBlockObject(block.Text); text != "" {
			parts = append(parts, text)
		}
	}

	// Extract fields (arranged in columns)
	for _, field := range block.Fields {
		if field != nil {
			if text := extractFromTextBlockObject(field); text != "" {
				parts = append(parts, text)
			}
		}
	}

	return strings.Join(parts, " ")
}

// extractFromContextBlock extracts text from a context block
func extractFromContextBlock(block *slack.ContextBlock) string {
	if block == nil {
		return ""
	}

	var parts []string

	for _, element := range block.ContextElements.Elements {
		switch elem := element.(type) {
		case *slack.TextBlockObject:
			if text := extractFromTextBlockObject(elem); text != "" {
				parts = append(parts, text)
			}
		case slack.TextBlockObject:
			if text := extractFromTextBlockObject(&elem); text != "" {
				parts = append(parts, text)
			}
			// Skip image elements as they don't contain text
		}
	}

	return strings.Join(parts, " ")
}

// extractFromRichTextBlock extracts text from a rich text block
func extractFromRichTextBlock(block *slack.RichTextBlock) string {
	if block == nil || len(block.Elements) == 0 {
		return ""
	}

	var parts []string

	for _, element := range block.Elements {
		elementText := extractFromRichTextElement(element)
		if elementText != "" {
			parts = append(parts, elementText)
		}
	}

	return strings.Join(parts, " ")
}

// extractFromRichTextElement extracts text from a rich text element
func extractFromRichTextElement(element slack.RichTextElement) string {
	if element == nil {
		return ""
	}

	switch elem := element.(type) {
	case *slack.RichTextSection:
		return extractFromRichTextSection(elem)
	case *slack.RichTextList:
		return extractFromRichTextList(elem)
	case *slack.RichTextPreformatted:
		return extractFromRichTextPreformatted(elem)
		// RichTextQuote is a type alias for RichTextSection in slack-go library
	}

	return ""
}

// extractFromRichTextSection extracts text from a rich text section
func extractFromRichTextSection(section *slack.RichTextSection) string {
	if section == nil || len(section.Elements) == 0 {
		return ""
	}

	var parts []string

	for _, element := range section.Elements {
		switch elem := element.(type) {
		case *slack.RichTextSectionTextElement:
			if elem.Text != "" {
				parts = append(parts, elem.Text)
			}
		case *slack.RichTextSectionLinkElement:
			// Include both URL and text for links
			if elem.Text != "" {
				parts = append(parts, elem.Text)
			} else if elem.URL != "" {
				parts = append(parts, elem.URL)
			}
		case *slack.RichTextSectionUserElement:
			// User mentions
			if elem.UserID != "" {
				parts = append(parts, fmt.Sprintf("@%s", elem.UserID))
			}
		case *slack.RichTextSectionChannelElement:
			// Channel references
			if elem.ChannelID != "" {
				parts = append(parts, fmt.Sprintf("#%s", elem.ChannelID))
			}
		case *slack.RichTextSectionBroadcastElement:
			// Broadcast mentions
			if elem.Range != "" {
				parts = append(parts, fmt.Sprintf("@%s", elem.Range))
			}
		}
	}

	return strings.Join(parts, " ")
}

// extractFromRichTextList extracts text from a rich text list
func extractFromRichTextList(list *slack.RichTextList) string {
	if list == nil || len(list.Elements) == 0 {
		return ""
	}

	var parts []string

	for i, element := range list.Elements {
		// Process each list item (which is typically a RichTextSection)
		elementText := extractFromRichTextElement(element)
		if elementText != "" {
			// Add list marker for readability
			if list.Style == "ordered" {
				parts = append(parts, fmt.Sprintf("%d. %s", i+1, elementText))
			} else {
				parts = append(parts, fmt.Sprintf("- %s", elementText))
			}
		}
	}

	return strings.Join(parts, " ")
}

// extractFromRichTextPreformatted extracts text from preformatted rich text
func extractFromRichTextPreformatted(preformatted *slack.RichTextPreformatted) string {
	if preformatted == nil || len(preformatted.Elements) == 0 {
		return ""
	}

	var parts []string

	for _, element := range preformatted.Elements {
		switch elem := element.(type) {
		case *slack.RichTextSectionTextElement:
			if elem.Text != "" {
				parts = append(parts, elem.Text)
			}
		}
	}

	return strings.Join(parts, " ")
}

// extractFromHeaderBlock extracts text from a header block
func extractFromHeaderBlock(block *slack.HeaderBlock) string {
	if block == nil || block.Text == nil {
		return ""
	}

	return extractFromTextBlockObject(block.Text)
}

// extractFromActionBlock extracts text from action block elements
func extractFromActionBlock(block *slack.ActionBlock) string {
	if block == nil || len(block.Elements.ElementSet) == 0 {
		return ""
	}

	var parts []string

	for _, element := range block.Elements.ElementSet {
		switch elem := element.(type) {
		case *slack.ButtonBlockElement:
			if elem.Text != nil {
				if text := extractFromTextBlockObject(elem.Text); text != "" {
					parts = append(parts, text)
				}
			}
		case *slack.SelectBlockElement:
			if elem.Placeholder != nil {
				if text := extractFromTextBlockObject(elem.Placeholder); text != "" {
					parts = append(parts, text)
				}
			}
			// Add more interactive element types as needed
		}
	}

	return strings.Join(parts, " ")
}

// extractFromTextBlockObject extracts text from a text block object
func extractFromTextBlockObject(textObj *slack.TextBlockObject) string {
	if textObj == nil || textObj.Text == "" {
		return ""
	}

	// Process the text based on type (plain_text or mrkdwn)
	text := textObj.Text

	// If it's markdown, we might want to process it
	if textObj.Type == "mrkdwn" {
		// Process markdown formatting if needed
		// For now, we'll keep the markdown as-is since it contains valuable formatting
		return text
	}

	return text
}

func filterSpecialChars(text string) string {
	replaceWithCommaCheck := func(match []string, isLast bool) string {
		var url, linkText string

		if len(match) == 3 && strings.Contains(match[0], "|") {
			url = match[1]
			linkText = match[2]
		} else if len(match) == 3 {
			linkText = match[1]
			url = match[2]
		}

		replacement := url + " - " + linkText

		if !isLast {
			replacement += ","
		}

		return replacement
	}

	// Helper function to check if this is the last link/element
	isLastInText := func(original string, currentText string) bool {
		linkPos := strings.LastIndex(currentText, original)
		if linkPos == -1 {
			return false
		}
		afterLink := strings.TrimSpace(currentText[linkPos+len(original):])
		return afterLink == ""
	}

	// Handle Slack-style links: <URL|Description>
	slackLinkRegex := regexp.MustCompile(`<(https?://[^>|]+)\|([^>]+)>`)
	slackMatches := slackLinkRegex.FindAllStringSubmatch(text, -1)
	for _, match := range slackMatches {
		original := match[0]
		isLast := isLastInText(original, text)
		replacement := replaceWithCommaCheck(match, isLast)
		text = strings.Replace(text, original, replacement, 1)
	}

	// Handle markdown links: [Description](URL)
	markdownLinkRegex := regexp.MustCompile(`\[([^\]]+)\]\((https?://[^)]+)\)`)
	markdownMatches := markdownLinkRegex.FindAllStringSubmatch(text, -1)
	for _, match := range markdownMatches {
		original := match[0]
		isLast := isLastInText(original, text)
		replacement := replaceWithCommaCheck(match, isLast)
		text = strings.Replace(text, original, replacement, 1)
	}

	htmlLinkRegex := regexp.MustCompile(`<a\s+href=["']([^"']+)["'][^>]*>([^<]+)</a>`)
	htmlMatches := htmlLinkRegex.FindAllStringSubmatch(text, -1)
	for _, match := range htmlMatches {
		original := match[0]
		isLast := isLastInText(original, text)
		url := match[1]
		linkText := match[2]
		replacement := url + " - " + linkText
		if !isLast {
			replacement += ","
		}
		text = strings.Replace(text, original, replacement, 1)
	}

	urlRegex := regexp.MustCompile(`https?://[^\s<>"{}|\\^` + "`" + `\[\]]+`)
	urls := urlRegex.FindAllString(text, -1)

	protected := text
	for i, url := range urls {
		placeholder := "___URL_PLACEHOLDER_" + string(rune(48+i)) + "___"
		protected = strings.Replace(protected, url, placeholder, 1)
	}

	cleanRegex := regexp.MustCompile(`[^0-9\p{L}\p{M}\s\.\,\-_:/\?=&%]`)
	cleaned := cleanRegex.ReplaceAllString(protected, "")

	// Restore the URLs
	for i, url := range urls {
		placeholder := "___URL_PLACEHOLDER_" + string(rune(48+i)) + "___"
		cleaned = strings.Replace(cleaned, placeholder, url, 1)
	}

	spaceRegex := regexp.MustCompile(`\s+`)
	cleaned = spaceRegex.ReplaceAllString(cleaned, " ")

	return strings.TrimSpace(cleaned)
}
