package edge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"runtime/trace"

	"github.com/rusq/slack"
)

// conversations.* API

// conversationsGenericInfoForm is the request to conversations.genericInfo
type conversationsGenericInfoForm struct {
	BaseRequest
	UpdatedChannels string `json:"updated_channels"` // i.e. {"C065H568ZAT":0}
	WebClientFields
}

type conversationsGenericInfoResponse struct {
	baseResponse
	Channels            []slack.Channel `json:"channels"`
	UnchangedChannelIDs []string        `json:"unchanged_channel_ids"`
}

func (cl *Client) ConversationsGenericInfo(ctx context.Context, channelID ...string) ([]slack.Channel, error) {
	ctx, task := trace.NewTask(ctx, "ConversationsGenericInfo")
	defer task.End()
	trace.Logf(ctx, "params", "channelID=%v", channelID)

	updChannel := make(map[string]int, len(channelID))
	for _, id := range channelID {
		updChannel[id] = 0
	}
	b, err := json.Marshal(updChannel)
	if err != nil {
		return nil, err
	}
	form := conversationsGenericInfoForm{
		BaseRequest: BaseRequest{
			Token: cl.token,
		},
		UpdatedChannels: string(b),
		WebClientFields: webclientReason("fallback:UnknownFetchManager"),
	}
	resp, err := cl.PostForm(ctx, "conversations.genericInfo", values(form, true))
	if err != nil {
		return nil, err
	}
	var r conversationsGenericInfoResponse
	if err := cl.ParseResponse(&r, resp); err != nil {
		return nil, err
	}
	return r.Channels, nil
}

type conversationsViewForm struct {
	BaseRequest
	CanonicalAvatars             bool   `json:"canonical_avatars"`
	NoUserProfile                bool   `json:"no_user_profile"`
	IgnoreReplies                bool   `json:"ignore_replies"`
	NoSelf                       bool   `json:"no_self"`
	IncludeFullUsers             bool   `json:"include_full_users"`
	IncludeUseCases              bool   `json:"include_use_cases"`
	IncludeStories               bool   `json:"include_stories"`
	NoMembers                    bool   `json:"no_members"`
	IncludeMutationTimestamps    bool   `json:"include_mutation_timestamps"`
	Count                        int    `json:"count"`
	Channel                      string `json:"channel"`
	IncludeFreeTeamExtraMessages bool   `json:"include_free_team_extra_messages"`
	WebClientFields
}

type ConversationsViewResponse struct {
	Users  []User            `json:"users"`
	IM     IM                `json:"im"`
	Emojis map[string]string `json:"emojis"`
	// we don't care about the rest of the response
}

func (cl *Client) ConversationsView(ctx context.Context, channelID string) (ConversationsViewResponse, error) {
	ctx, task := trace.NewTask(ctx, "ConversationsView")
	defer task.End()
	trace.Logf(ctx, "params", "channelID=%v", channelID)

	form := conversationsViewForm{
		BaseRequest: BaseRequest{
			Token: cl.token,
		},
		CanonicalAvatars:          true,
		NoUserProfile:             true,
		IgnoreReplies:             true,
		NoSelf:                    true,
		IncludeFullUsers:          false,
		IncludeUseCases:           false,
		IncludeStories:            false,
		NoMembers:                 true,
		IncludeMutationTimestamps: false,
		Count:                     50,
		Channel:                   channelID,
		WebClientFields:           webclientReason(""),
	}
	resp, err := cl.PostForm(ctx, "conversations.view", values(form, true))
	if err != nil {
		return ConversationsViewResponse{}, err
	}
	var r = struct {
		baseResponse
		ConversationsViewResponse
	}{}
	if err := cl.ParseResponse(&r, resp); err != nil {
		return ConversationsViewResponse{}, err
	}
	return r.ConversationsViewResponse, nil
}

// conversationsCreateForm is the request to conversations.create
type conversationsCreateForm struct {
	BaseRequest
	Name         string `json:"name"`
	ValidateName string `json:"validate_name"`
	IsPrivate    string `json:"is_private"`
	TeamID       string `json:"team_id"`
	WebClientFields
}

// ConversationsCreateForm is the request to conversations.create
type ConversationsCreateForm struct {
	BaseRequest
	Name         string `json:"name"`
	ValidateName string `json:"validate_name"`
	IsPrivate    string `json:"is_private"`
	TeamID       string `json:"team_id"`
	WebClientFields
}

// ConversationsCreateResponse matches the successful response structure
type ConversationsCreateResponse struct {
	baseResponse
	Channel slack.Channel `json:"channel"`
}

func (cl *Client) CreateConversation(ctx context.Context, channelName string, isPrivate bool, teamID string) (*slack.Channel, error) {
	ctx, task := trace.NewTask(ctx, "CreateConversation")
	defer task.End()
	trace.Logf(ctx, "params", "channelName=%v, isPrivate=%v, teamID=%v", channelName, isPrivate, teamID)

	// Convert boolean to string as expected by the API
	privateStr := "false"
	if isPrivate {
		privateStr = "true"
	}

	// Create multipart form data exactly like the working curl command
	var reqBody bytes.Buffer
	boundary := "----WebKitFormBoundaryALD2zjbhYWzpc5o0"
	writer := multipart.NewWriter(&reqBody)
	writer.SetBoundary(boundary)

	// Add fields in exact order from curl
	writer.WriteField("token", cl.token) // xoxc token
	writer.WriteField("name", channelName)
	writer.WriteField("validate_name", "true")
	writer.WriteField("is_private", privateStr)
	writer.WriteField("team_id", teamID)
	writer.WriteField("_x_mode", "online")
	writer.WriteField("_x_sonic", "true")
	writer.WriteField("_x_app_name", "client")
	writer.Close()

	// Build URL using webapiURL method and add query parameters
	baseURL := cl.webapiURL("conversations.create")
	url := fmt.Sprintf("%s?_x_id=6b1e297c-1756862180.076&_x_csid=LhBhL26bLUA&slack_route=%s%%3A%s&_x_version_ts=1756837958&_x_frontend_build_type=current&_x_desktop_ia=4&_x_gantry=true&fp=39&_x_num_retries=0",
		baseURL, teamID, teamID)

	req, err := http.NewRequestWithContext(ctx, "POST", url, &reqBody)
	if err != nil {
		return nil, err
	}

	// Set exact headers from curl
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Origin", "https://app.slack.com")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36")

	// Note: The Edge client should handle cookies automatically through its HTTP client
	// The xoxd cookie should be included by the underlying auth provider

	resp, err := cl.Raw().Do(req)
	if err != nil {
		return nil, err
	}

	var r ConversationsCreateResponse

	if err := cl.ParseResponse(&r, resp); err != nil {
		return nil, err
	}

	trace.Logf(ctx, "response", "ok=%v, error=%v, channel_id=%v, channel_name=%v", r.Ok, r.Error, r.Channel.ID, r.Channel.Name)

	// Check if the API call failed
	if !r.Ok {
		return nil, fmt.Errorf("slack API error: %s", r.Error)
	}

	return &r.Channel, nil
}
