package provider

import (
	"context"
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"strings"

	"github.com/korotovsky/slack-mcp-server/pkg/limiter"
	"github.com/korotovsky/slack-mcp-server/pkg/provider/edge"
	"github.com/korotovsky/slack-mcp-server/pkg/transport"
	"github.com/rusq/slackdump/v3/auth"
	"github.com/slack-go/slack"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

const usersNotReadyMsg = "users cache is not ready yet, sync process is still running... please wait"
const channelsNotReadyMsg = "channels cache is not ready yet, sync process is still running... please wait"
const emojisNotReadyMsg = "emojis cache is not ready yet, sync process is still running... please wait"
const defaultUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36"

var AllChanTypes = []string{"mpim", "im", "public_channel", "private_channel"}
var PrivateChanType = "private_channel"
var PubChanType = "public_channel"

var ErrUsersNotReady = errors.New(usersNotReadyMsg)
var ErrChannelsNotReady = errors.New(channelsNotReadyMsg)
var ErrEmojisNotReady = errors.New(emojisNotReadyMsg)

type UsersCache struct {
	Users    map[string]slack.User `json:"users"`
	UsersInv map[string]string     `json:"users_inv"`
}

type ChannelsCache struct {
	Channels    map[string]Channel `json:"channels"`
	ChannelsInv map[string]string  `json:"channels_inv"`
}

type EmojiCache struct {
	Emojis map[string]Emoji `json:"emojis"`
}

type Emoji struct {
	Name     string   `json:"name"`
	URL      string   `json:"url"`
	IsCustom bool     `json:"is_custom"`
	Aliases  []string `json:"aliases"`
	TeamID   string   `json:"team_id"`
	UserID   string   `json:"user_id"`
}

type Channel struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Topic       string `json:"topic"`
	Purpose     string `json:"purpose"`
	MemberCount int    `json:"memberCount"`
	IsMpIM      bool   `json:"mpim"`
	IsIM        bool   `json:"im"`
	IsPrivate   bool   `json:"private"`
}

type SlackAPI interface {
	// Standard slack-go API methods
	AuthTest() (*slack.AuthTestResponse, error)
	AuthTestContext(ctx context.Context) (*slack.AuthTestResponse, error)
	GetUsersContext(ctx context.Context, options ...slack.GetUsersOption) ([]slack.User, error)
	GetUsersInfo(users ...string) (*[]slack.User, error)
	PostMessageContext(ctx context.Context, channel string, options ...slack.MsgOption) (string, string, error)
	MarkConversationContext(ctx context.Context, channel, ts string) error

	// Useed to get messages
	GetConversationHistoryContext(ctx context.Context, params *slack.GetConversationHistoryParameters) (*slack.GetConversationHistoryResponse, error)
	GetConversationRepliesContext(ctx context.Context, params *slack.GetConversationRepliesParameters) (msgs []slack.Message, hasMore bool, nextCursor string, err error)
	SearchContext(ctx context.Context, query string, params slack.SearchParameters) (*slack.SearchMessages, *slack.SearchFiles, error)

	// Useed to get channels list from both Slack and Enterprise Grid versions
	GetConversationsContext(ctx context.Context, params *slack.GetConversationsParameters) ([]slack.Channel, string, error)

	// Edge API methods
	ClientUserBoot(ctx context.Context) (*edge.ClientUserBootResponse, error)

	// Reactions
	AddReactionContext(ctx context.Context, name string, item slack.ItemRef) error
	RemoveReactionContext(ctx context.Context, name string, item slack.ItemRef) error

	// Message management
	DeleteMessageContext(ctx context.Context, channel, messageTimestamp string) (string, string, error)
	UpdateMessageContext(ctx context.Context, channel, timestamp string, options ...slack.MsgOption) (string, string, string, error)

	// Channel members
	GetUsersInConversationContext(ctx context.Context, params *slack.GetUsersInConversationParameters) ([]string, string, error)
	GetConversationInfoContext(ctx context.Context, input *slack.GetConversationInfoInput) (*slack.Channel, error)

	// User information
	GetUserInfoContext(ctx context.Context, user string) (*slack.User, error)
	GetUserPresenceContext(ctx context.Context, user string) (*slack.UserPresence, error)
}

type MCPSlackClient struct {
	slackClient *slack.Client
	edgeClient  *edge.Client

	authResponse *slack.AuthTestResponse
	authProvider auth.Provider

	isEnterprise bool
	isOAuth      bool
	teamEndpoint string
}

type ApiProvider struct {
	transport string
	client    SlackAPI
	logger    *zap.Logger

	rateLimiter *rate.Limiter

	users      map[string]slack.User
	usersInv   map[string]string
	usersCache string
	usersReady bool

	channels      map[string]Channel
	channelsInv   map[string]string
	channelsCache string
	channelsReady bool

	emojis      map[string]Emoji
	emojisCache string
	emojisReady bool
}

func NewMCPSlackClient(authProvider auth.Provider, logger *zap.Logger) (*MCPSlackClient, error) {
	httpClient := transport.ProvideHTTPClient(authProvider.Cookies(), logger)

	slackClient := slack.New(authProvider.SlackToken(),
		slack.OptionHTTPClient(httpClient),
	)

	authResp, err := slackClient.AuthTest()
	if err != nil {
		return nil, err
	}

	authResponse := &slack.AuthTestResponse{
		URL:          authResp.URL,
		Team:         authResp.Team,
		User:         authResp.User,
		TeamID:       authResp.TeamID,
		UserID:       authResp.UserID,
		EnterpriseID: authResp.EnterpriseID,
		BotID:        authResp.BotID,
	}

	slackClient = slack.New(authProvider.SlackToken(),
		slack.OptionHTTPClient(httpClient),
		slack.OptionAPIURL(authResp.URL+"api/"),
	)

	edgeClient, err := edge.NewWithInfo(authResponse, authProvider,
		edge.OptionHTTPClient(httpClient),
	)
	if err != nil {
		return nil, err
	}

	isEnterprise := authResp.EnterpriseID != ""

	return &MCPSlackClient{
		slackClient:  slackClient,
		edgeClient:   edgeClient,
		authResponse: authResponse,
		authProvider: authProvider,
		isEnterprise: isEnterprise,
		isOAuth:      strings.HasPrefix(authProvider.SlackToken(), "xoxp-"),
		teamEndpoint: authResp.URL,
	}, nil
}

func (c *MCPSlackClient) AuthTest() (*slack.AuthTestResponse, error) {
	if os.Getenv("SLACK_MCP_XOXP_TOKEN") == "demo" || (os.Getenv("SLACK_MCP_XOXC_TOKEN") == "demo" && os.Getenv("SLACK_MCP_XOXD_TOKEN") == "demo") {
		return &slack.AuthTestResponse{
			URL:          "https://_.slack.com",
			Team:         "Demo Team",
			User:         "Username",
			TeamID:       "TEAM123456",
			UserID:       "U1234567890",
			EnterpriseID: "",
			BotID:        "",
		}, nil
	}

	if c.authResponse != nil {
		return c.authResponse, nil
	}

	return c.slackClient.AuthTest()
}

func (c *MCPSlackClient) AuthTestContext(ctx context.Context) (*slack.AuthTestResponse, error) {
	return c.slackClient.AuthTestContext(ctx)
}

func (c *MCPSlackClient) GetUsersContext(ctx context.Context, options ...slack.GetUsersOption) ([]slack.User, error) {
	return c.slackClient.GetUsersContext(ctx, options...)
}

func (c *MCPSlackClient) GetUsersInfo(users ...string) (*[]slack.User, error) {
	return c.slackClient.GetUsersInfo(users...)
}

func (c *MCPSlackClient) MarkConversationContext(ctx context.Context, channel, ts string) error {
	return c.slackClient.MarkConversationContext(ctx, channel, ts)
}

func (c *MCPSlackClient) GetConversationsContext(ctx context.Context, params *slack.GetConversationsParameters) ([]slack.Channel, string, error) {
	// Please see https://github.com/korotovsky/slack-mcp-server/issues/73
	// It seems that `conversations.list` works with `xoxp` tokens within Enterprise Grid setups
	// and if `xoxc`/`xoxd` defined we fallback to edge client.
	// In non Enterprise Grid setups we always use `conversations.list` api as it accepts both token types wtf.
	if c.isEnterprise {
		if c.isOAuth {
			return c.slackClient.GetConversationsContext(ctx, params)
		} else {
			edgeChannels, _, err := c.edgeClient.GetConversationsContext(ctx, nil)
			if err != nil {
				return nil, "", err
			}

			var channels []slack.Channel
			for _, ec := range edgeChannels {
				if params != nil && params.ExcludeArchived && ec.IsArchived {
					continue
				}

				channels = append(channels, slack.Channel{
					IsGeneral: ec.IsGeneral,
					GroupConversation: slack.GroupConversation{
						Conversation: slack.Conversation{
							ID:                 ec.ID,
							IsIM:               ec.IsIM,
							IsMpIM:             ec.IsMpIM,
							IsPrivate:          ec.IsPrivate,
							Created:            slack.JSONTime(ec.Created.Time().UnixMilli()),
							Unlinked:           ec.Unlinked,
							NameNormalized:     ec.NameNormalized,
							IsShared:           ec.IsShared,
							IsExtShared:        ec.IsExtShared,
							IsOrgShared:        ec.IsOrgShared,
							IsPendingExtShared: ec.IsPendingExtShared,
							NumMembers:         ec.NumMembers,
							User:               ec.User,
						},
						Name:       ec.Name,
						IsArchived: ec.IsArchived,
						Members:    ec.Members,
						Topic: slack.Topic{
							Value: ec.Topic.Value,
						},
						Purpose: slack.Purpose{
							Value: ec.Purpose.Value,
						},
					},
				})
			}

			return channels, "", nil
		}
	}

	return c.slackClient.GetConversationsContext(ctx, params)
}

func (c *MCPSlackClient) GetConversationHistoryContext(ctx context.Context, params *slack.GetConversationHistoryParameters) (*slack.GetConversationHistoryResponse, error) {
	return c.slackClient.GetConversationHistoryContext(ctx, params)
}

func (c *MCPSlackClient) GetConversationRepliesContext(ctx context.Context, params *slack.GetConversationRepliesParameters) (msgs []slack.Message, hasMore bool, nextCursor string, err error) {
	return c.slackClient.GetConversationRepliesContext(ctx, params)
}

func (c *MCPSlackClient) SearchContext(ctx context.Context, query string, params slack.SearchParameters) (*slack.SearchMessages, *slack.SearchFiles, error) {
	return c.slackClient.SearchContext(ctx, query, params)
}

func (c *MCPSlackClient) PostMessageContext(ctx context.Context, channelID string, options ...slack.MsgOption) (string, string, error) {
	return c.slackClient.PostMessageContext(ctx, channelID, options...)
}

func (c *MCPSlackClient) AddReactionContext(ctx context.Context, name string, item slack.ItemRef) error {
	// Route by token type
	if c.isOAuth {
		return c.slackClient.AddReactionContext(ctx, name, item)
	}
	return c.edgeClient.AddReactionContext(ctx, name, item)
}

func (c *MCPSlackClient) RemoveReactionContext(ctx context.Context, name string, item slack.ItemRef) error {
	// Route by token type
	if c.isOAuth {
		return c.slackClient.RemoveReactionContext(ctx, name, item)
	}
	return c.edgeClient.RemoveReactionContext(ctx, name, item)
}

func (c *MCPSlackClient) DeleteMessageContext(ctx context.Context, channel, messageTimestamp string) (string, string, error) {
	// chat.delete is only available via standard API
	return c.slackClient.DeleteMessageContext(ctx, channel, messageTimestamp)
}

func (c *MCPSlackClient) UpdateMessageContext(ctx context.Context, channel, timestamp string, options ...slack.MsgOption) (string, string, string, error) {
	// chat.update is only available via standard API
	return c.slackClient.UpdateMessageContext(ctx, channel, timestamp, options...)
}

func (c *MCPSlackClient) GetUsersInConversationContext(ctx context.Context, params *slack.GetUsersInConversationParameters) ([]string, string, error) {
	// In Enterprise Grid with browser tokens, we need to use the Edge client
	if c.isEnterprise && !c.isOAuth {
		// Use Edge client's UsersList method
		users, err := c.edgeClient.UsersList(ctx, params.ChannelID)
		if err != nil {
			return nil, "", err
		}

		// Extract user IDs from the User structs
		userIDs := make([]string, 0, len(users))
		for _, user := range users {
			userIDs = append(userIDs, user.ID)
		}

		// Handle pagination if needed - Edge client handles it internally
		// so we just return empty cursor for now
		return userIDs, "", nil
	}

	// For non-Enterprise or OAuth tokens, use standard API
	return c.slackClient.GetUsersInConversationContext(ctx, params)
}

func (c *MCPSlackClient) GetConversationInfoContext(ctx context.Context, input *slack.GetConversationInfoInput) (*slack.Channel, error) {
	// conversations.info is only available via standard API
	return c.slackClient.GetConversationInfoContext(ctx, input)
}

func (c *MCPSlackClient) GetUserInfoContext(ctx context.Context, user string) (*slack.User, error) {
	// In Enterprise Grid with browser tokens, we might need to use cached data
	// For now, try the standard API first
	return c.slackClient.GetUserInfoContext(ctx, user)
}

func (c *MCPSlackClient) GetUserPresenceContext(ctx context.Context, user string) (*slack.UserPresence, error) {
	// users.getPresence is only available via standard API
	return c.slackClient.GetUserPresenceContext(ctx, user)
}

func (c *MCPSlackClient) ClientUserBoot(ctx context.Context) (*edge.ClientUserBootResponse, error) {
	return c.edgeClient.ClientUserBoot(ctx)
}

func (c *MCPSlackClient) IsEnterprise() bool {
	return c.isEnterprise
}

func (c *MCPSlackClient) AuthResponse() *slack.AuthTestResponse {
	return c.authResponse
}

func (c *MCPSlackClient) Raw() struct {
	Slack *slack.Client
	Edge  *edge.Client
} {
	return struct {
		Slack *slack.Client
		Edge  *edge.Client
	}{
		Slack: c.slackClient,
		Edge:  c.edgeClient,
	}
}

func New(transport string, logger *zap.Logger) *ApiProvider {
	var (
		authProvider auth.ValueAuth
		err          error
	)

	// Check for XOXP token first (User OAuth)
	xoxpToken := os.Getenv("SLACK_MCP_XOXP_TOKEN")
	if xoxpToken != "" {
		authProvider, err = auth.NewValueAuth(xoxpToken, "")
		if err != nil {
			logger.Fatal("Failed to create auth provider with XOXP token", zap.Error(err))
		}

		return newWithXOXP(transport, authProvider, logger)
	}

	// Fall back to XOXC/XOXD tokens (session-based)
	xoxcToken := os.Getenv("SLACK_MCP_XOXC_TOKEN")
	xoxdToken := os.Getenv("SLACK_MCP_XOXD_TOKEN")

	if xoxcToken == "" || xoxdToken == "" {
		logger.Fatal("Authentication required: Either SLACK_MCP_XOXP_TOKEN (User OAuth) or both SLACK_MCP_XOXC_TOKEN and SLACK_MCP_XOXD_TOKEN (session-based) environment variables must be provided")
	}

	authProvider, err = auth.NewValueAuth(xoxcToken, xoxdToken)
	if err != nil {
		logger.Fatal("Failed to create auth provider with XOXC/XOXD tokens", zap.Error(err))
	}

	return newWithXOXC(transport, authProvider, logger)
}

func newWithXOXP(transport string, authProvider auth.ValueAuth, logger *zap.Logger) *ApiProvider {
	var (
		client *MCPSlackClient
		err    error
	)

	usersCache := os.Getenv("SLACK_MCP_USERS_CACHE")
	if usersCache == "" {
		usersCache = ".users_cache.json"
	}

	channelsCache := os.Getenv("SLACK_MCP_CHANNELS_CACHE")
	if channelsCache == "" {
		channelsCache = ".channels_cache.json"
	}

	emojisCache := os.Getenv("SLACK_MCP_EMOJIS_CACHE")
	if emojisCache == "" {
		emojisCache = ".emojis_cache.json"
	}

	if os.Getenv("SLACK_MCP_XOXP_TOKEN") == "demo" || (os.Getenv("SLACK_MCP_XOXC_TOKEN") == "demo" && os.Getenv("SLACK_MCP_XOXD_TOKEN") == "demo") {
		logger.Info("Demo credentials are set, skip.")
	} else {
		client, err = NewMCPSlackClient(authProvider, logger)
		if err != nil {
			logger.Fatal("Failed to create MCP Slack client", zap.Error(err))
		}
	}

	return &ApiProvider{
		transport: transport,
		client:    client,
		logger:    logger,

		rateLimiter: limiter.Tier2.Limiter(),

		users:      make(map[string]slack.User),
		usersInv:   map[string]string{},
		usersCache: usersCache,

		channels:      make(map[string]Channel),
		channelsInv:   map[string]string{},
		channelsCache: channelsCache,

		emojis:      make(map[string]Emoji),
		emojisCache: emojisCache,
	}
}

func newWithXOXC(transport string, authProvider auth.ValueAuth, logger *zap.Logger) *ApiProvider {
	var (
		client *MCPSlackClient
		err    error
	)

	usersCache := os.Getenv("SLACK_MCP_USERS_CACHE")
	if usersCache == "" {
		usersCache = ".users_cache.json"
	}

	channelsCache := os.Getenv("SLACK_MCP_CHANNELS_CACHE")
	if channelsCache == "" {
		channelsCache = ".channels_cache_v2.json"
	}

	emojisCache := os.Getenv("SLACK_MCP_EMOJIS_CACHE")
	if emojisCache == "" {
		emojisCache = ".emojis_cache.json"
	}

	if os.Getenv("SLACK_MCP_XOXP_TOKEN") == "demo" || (os.Getenv("SLACK_MCP_XOXC_TOKEN") == "demo" && os.Getenv("SLACK_MCP_XOXD_TOKEN") == "demo") {
		logger.Info("Demo credentials are set, skip.")
	} else {
		client, err = NewMCPSlackClient(authProvider, logger)
		if err != nil {
			logger.Fatal("Failed to create MCP Slack client", zap.Error(err))
		}
	}

	return &ApiProvider{
		transport: transport,
		client:    client,
		logger:    logger,

		rateLimiter: limiter.Tier2.Limiter(),

		users:      make(map[string]slack.User),
		usersInv:   map[string]string{},
		usersCache: usersCache,

		channels:      make(map[string]Channel),
		channelsInv:   map[string]string{},
		channelsCache: channelsCache,

		emojis:      make(map[string]Emoji),
		emojisCache: emojisCache,
	}
}

func (ap *ApiProvider) RefreshUsers(ctx context.Context) error {
	var (
		list         []slack.User
		usersCounter = 0
		optionLimit  = slack.GetUsersOptionLimit(1000)
	)

	if data, err := ioutil.ReadFile(ap.usersCache); err == nil {
		var cachedUsers []slack.User
		if err := json.Unmarshal(data, &cachedUsers); err != nil {
			ap.logger.Warn("Failed to unmarshal users cache, will refetch",
				zap.String("cache_file", ap.usersCache),
				zap.Error(err))
		} else {
			for _, u := range cachedUsers {
				ap.users[u.ID] = u
				ap.usersInv[u.Name] = u.ID
			}
			ap.logger.Info("Loaded users from cache",
				zap.Int("count", len(cachedUsers)),
				zap.String("cache_file", ap.usersCache))
			ap.usersReady = true
			return nil
		}
	}

	users, err := ap.client.GetUsersContext(ctx,
		optionLimit,
	)
	if err != nil {
		ap.logger.Error("Failed to fetch users", zap.Error(err))
		return err
	} else {
		list = append(list, users...)
	}

	for _, user := range users {
		ap.users[user.ID] = user
		ap.usersInv[user.Name] = user.ID
		usersCounter++
	}

	users, err = ap.GetSlackConnect(ctx)
	if err != nil {
		ap.logger.Error("Failed to fetch users from Slack Connect", zap.Error(err))
		return err
	} else {
		list = append(list, users...)
	}

	for _, user := range users {
		ap.users[user.ID] = user
		ap.usersInv[user.Name] = user.ID
		usersCounter++
	}

	if data, err := json.MarshalIndent(list, "", "  "); err != nil {
		ap.logger.Error("Failed to marshal users for cache", zap.Error(err))
	} else {
		if err := ioutil.WriteFile(ap.usersCache, data, 0644); err != nil {
			ap.logger.Error("Failed to write cache file",
				zap.String("cache_file", ap.usersCache),
				zap.Error(err))
		} else {
			ap.logger.Info("Wrote users to cache",
				zap.Int("count", usersCounter),
				zap.String("cache_file", ap.usersCache))
		}
	}

	ap.usersReady = true

	return nil
}

func (ap *ApiProvider) RefreshEmojis(ctx context.Context) error {
	// Try loading from cache first
	if data, err := ioutil.ReadFile(ap.emojisCache); err == nil {
		var cachedEmojis []Emoji
		if err := json.Unmarshal(data, &cachedEmojis); err != nil {
			ap.logger.Warn("Failed to unmarshal emojis cache, will refetch",
				zap.String("cache_file", ap.emojisCache),
				zap.Error(err))
		} else {
			for _, e := range cachedEmojis {
				ap.emojis[e.Name] = e
			}
			ap.logger.Info("Loaded emojis from cache",
				zap.Int("count", len(cachedEmojis)),
				zap.String("cache_file", ap.emojisCache))
			ap.emojisReady = true
			return nil
		}
	}

	// Handle demo mode or nil client
	if ap.client == nil {
		ap.logger.Info("Client is nil (demo mode), using default emojis only")
		// Just add the common unicode emojis and mark as ready
		ap.addCommonUnicodeEmojis()
		ap.emojisReady = true
		return nil
	}

	// Fetch emojis from Slack API
	// Note: Since we can't access Raw() method on SlackAPI interface,
	// we'll use a type assertion to access the MCPSlackClient
	mcpClient, ok := ap.client.(*MCPSlackClient)
	if !ok {
		ap.logger.Error("Failed to cast client to MCPSlackClient")
		return errors.New("failed to access emoji API")
	}

	emojis, err := mcpClient.Raw().Slack.GetEmojiContext(ctx)
	if err != nil {
		ap.logger.Error("Failed to fetch emojis", zap.Error(err))
		return err
	}

	// Convert to our internal format
	var emojiList []Emoji
	for name, url := range emojis {
		// Check if it's an alias (starts with "alias:")
		isAlias := strings.HasPrefix(url, "alias:")
		if !isAlias {
			emoji := Emoji{
				Name:     name,
				URL:      url,
				IsCustom: true, // All returned emojis from API are custom
				Aliases:  []string{},
				TeamID:   mcpClient.AuthResponse().TeamID,
				UserID:   "", // We don't have user info from this API
			}
			ap.emojis[name] = emoji
			emojiList = append(emojiList, emoji)
		}
	}

	// Also add standard Unicode emojis
	ap.addCommonUnicodeEmojis()

	// Collect all emojis for saving to cache
	for _, emoji := range ap.emojis {
		if emoji.IsCustom {
			continue // Already added custom emojis to emojiList
		}
		emojiList = append(emojiList, emoji)
	}

	// Save to cache
	if data, err := json.MarshalIndent(emojiList, "", "  "); err != nil {
		ap.logger.Error("Failed to marshal emojis for cache", zap.Error(err))
	} else {
		if err := ioutil.WriteFile(ap.emojisCache, data, 0644); err != nil {
			ap.logger.Error("Failed to write cache file",
				zap.String("cache_file", ap.emojisCache),
				zap.Error(err))
		} else {
			ap.logger.Info("Wrote emojis to cache",
				zap.Int("count", len(emojiList)),
				zap.String("cache_file", ap.emojisCache))
		}
	}

	ap.emojisReady = true
	return nil
}

func (ap *ApiProvider) addCommonUnicodeEmojis() {
	// Standard Unicode emojis (a subset of common ones)
	// These are not returned by the API but are always available
	commonUnicodeEmojis := map[string]string{
		"thumbsup":         "👍",
		"thumbsdown":       "👎",
		"heart":            "❤️",
		"smile":            "😊",
		"laughing":         "😂",
		"cry":              "😢",
		"angry":            "😠",
		"clap":             "👏",
		"fire":             "🔥",
		"eyes":             "👀",
		"rocket":           "🚀",
		"100":              "💯",
		"pray":             "🙏",
		"tada":             "🎉",
		"white_check_mark": "✅",
		"x":                "❌",
		"warning":          "⚠️",
		"question":         "❓",
		"exclamation":      "❗",
		"heavy_plus_sign":  "+1",
		"heavy_minus_sign": "-1",
	}

	for name, unicode := range commonUnicodeEmojis {
		if _, exists := ap.emojis[name]; !exists {
			emoji := Emoji{
				Name:     name,
				URL:      unicode, // Store the unicode character as URL for unicode emojis
				IsCustom: false,
				Aliases:  []string{},
				TeamID:   "",
				UserID:   "",
			}
			ap.emojis[name] = emoji
		}
	}
}

func (ap *ApiProvider) RefreshChannels(ctx context.Context) error {
	if data, err := ioutil.ReadFile(ap.channelsCache); err == nil {
		var cachedChannels []Channel
		if err := json.Unmarshal(data, &cachedChannels); err != nil {
			ap.logger.Warn("Failed to unmarshal channels cache, will refetch",
				zap.String("cache_file", ap.channelsCache),
				zap.Error(err))
		} else {
			for _, c := range cachedChannels {
				ap.channels[c.ID] = c
				ap.channelsInv[c.Name] = c.ID
			}
			ap.logger.Info("Loaded channels from cache",
				zap.Int("count", len(cachedChannels)),
				zap.String("cache_file", ap.channelsCache))
			ap.channelsReady = true
			return nil
		}
	}

	channels := ap.GetChannels(ctx, AllChanTypes)

	if data, err := json.MarshalIndent(channels, "", "  "); err != nil {
		ap.logger.Error("Failed to marshal channels for cache", zap.Error(err))
	} else {
		if err := ioutil.WriteFile(ap.channelsCache, data, 0644); err != nil {
			ap.logger.Error("Failed to write cache file",
				zap.String("cache_file", ap.channelsCache),
				zap.Error(err))
		} else {
			ap.logger.Info("Wrote channels to cache",
				zap.Int("count", len(channels)),
				zap.String("cache_file", ap.channelsCache))
		}
	}

	ap.channelsReady = true

	return nil
}

func (ap *ApiProvider) GetSlackConnect(ctx context.Context) ([]slack.User, error) {
	boot, err := ap.client.ClientUserBoot(ctx)
	if err != nil {
		ap.logger.Error("Failed to fetch client user boot", zap.Error(err))
		return nil, err
	}

	var collectedIDs []string
	for _, im := range boot.IMs {
		if !im.IsShared && !im.IsExtShared {
			continue
		}

		_, ok := ap.users[im.User]
		if !ok {
			collectedIDs = append(collectedIDs, im.User)
		}
	}

	res := make([]slack.User, 0, len(collectedIDs))
	if len(collectedIDs) > 0 {
		usersInfo, err := ap.client.GetUsersInfo(strings.Join(collectedIDs, ","))
		if err != nil {
			ap.logger.Error("Failed to fetch users info for shared IMs", zap.Error(err))
			return nil, err
		}

		for _, u := range *usersInfo {
			res = append(res, u)
		}
	}

	return res, nil
}

func (ap *ApiProvider) GetChannels(ctx context.Context, channelTypes []string) []Channel {
	if len(channelTypes) == 0 {
		channelTypes = AllChanTypes
	}

	params := &slack.GetConversationsParameters{
		Types:           AllChanTypes,
		Limit:           1000,
		ExcludeArchived: true,
	}

	var (
		channels []slack.Channel
		chans    []Channel

		nextcur string
		err     error
	)

	for {
		if err := ap.rateLimiter.Wait(ctx); err != nil {
			ap.logger.Error("Rate limiter wait failed", zap.Error(err))
			return nil
		}

		channels, nextcur, err = ap.client.GetConversationsContext(ctx, params)
		if err != nil {
			ap.logger.Error("Failed to fetch channels", zap.Error(err))
			break
		}

		chans = make([]Channel, 0, len(channels))
		for _, channel := range channels {
			ch := mapChannel(
				channel.ID,
				channel.Name,
				channel.NameNormalized,
				channel.Topic.Value,
				channel.Purpose.Value,
				channel.User,
				channel.Members,
				channel.NumMembers,
				channel.IsIM,
				channel.IsMpIM,
				channel.IsPrivate,
				ap.ProvideUsersMap().Users,
			)
			chans = append(chans, ch)
		}

		for _, ch := range chans {
			ap.channels[ch.ID] = ch
			ap.channelsInv[ch.Name] = ch.ID
		}

		if nextcur == "" {
			break
		}

		params.Cursor = nextcur
	}

	var res []Channel
	for _, t := range channelTypes {
		for _, channel := range ap.channels {
			if (t == "public_channel" && !channel.IsPrivate && !channel.IsIM && !channel.IsMpIM) ||
				(t == "private_channel" && channel.IsPrivate && !channel.IsIM && !channel.IsMpIM) ||
				(t == "im" && channel.IsIM) ||
				(t == "mpim" && channel.IsMpIM) {
				res = append(res, channel)
			}
		}
	}

	return res
}

func (ap *ApiProvider) ProvideUsersMap() *UsersCache {
	return &UsersCache{
		Users:    ap.users,
		UsersInv: ap.usersInv,
	}
}

func (ap *ApiProvider) ProvideChannelsMaps() *ChannelsCache {
	return &ChannelsCache{
		Channels:    ap.channels,
		ChannelsInv: ap.channelsInv,
	}
}

func (ap *ApiProvider) ProvideEmojiMap() *EmojiCache {
	return &EmojiCache{
		Emojis: ap.emojis,
	}
}

func (ap *ApiProvider) IsReady() (bool, error) {
	if !ap.usersReady {
		return false, ErrUsersNotReady
	}
	if !ap.channelsReady {
		return false, ErrChannelsNotReady
	}
	// Note: We don't check emojisReady here because emojis are optional
	// and shouldn't block other operations. The emoji handler will check
	// this separately if needed.
	return true, nil
}

func (ap *ApiProvider) IsEmojisReady() (bool, error) {
	if !ap.emojisReady {
		return false, ErrEmojisNotReady
	}
	return true, nil
}

func (ap *ApiProvider) ServerTransport() string {
	return ap.transport
}

func (ap *ApiProvider) Slack() SlackAPI {
	return ap.client
}

func mapChannel(
	id, name, nameNormalized, topic, purpose, user string,
	members []string,
	numMembers int,
	isIM, isMpIM, isPrivate bool,
	usersMap map[string]slack.User,
) Channel {
	channelName := name
	finalPurpose := purpose
	finalTopic := topic
	finalMemberCount := numMembers

	if isIM {
		finalMemberCount = 2
		if u, ok := usersMap[user]; ok {
			channelName = "@" + u.Name
			// Use RealName, fallback to Profile.RealName if empty
			displayName := u.RealName
			if displayName == "" {
				displayName = u.Profile.RealName
			}
			// Add (deactivated) suffix for deleted users
			if u.Deleted && displayName != "" {
				displayName += " (deactivated)"
			}
			finalPurpose = "DM with " + displayName
		} else {
			channelName = "@" + user
			finalPurpose = "DM with " + user
		}
		finalTopic = ""
	} else if isMpIM {
		if len(members) > 0 {
			finalMemberCount = len(members)
			var userNames []string
			for _, uid := range members {
				if u, ok := usersMap[uid]; ok {
					// Use RealName, fallback to Profile.RealName if empty
					displayName := u.RealName
					if displayName == "" {
						displayName = u.Profile.RealName
					}
					// Add (deactivated) suffix for deleted users
					if u.Deleted && displayName != "" {
						displayName += " (deactivated)"
					}
					userNames = append(userNames, displayName)
				} else {
					userNames = append(userNames, uid)
				}
			}
			channelName = "@" + nameNormalized
			finalPurpose = "Group DM with " + strings.Join(userNames, ", ")
			finalTopic = ""
		}
	} else {
		// Use nameNormalized if available, otherwise fall back to name
		displayName := nameNormalized
		if displayName == "" {
			displayName = name
		}
		channelName = "#" + displayName
	}

	return Channel{
		ID:          id,
		Name:        channelName,
		Topic:       finalTopic,
		Purpose:     finalPurpose,
		MemberCount: finalMemberCount,
		IsIM:        isIM,
		IsMpIM:      isMpIM,
		IsPrivate:   isPrivate,
	}
}
