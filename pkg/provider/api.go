package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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
const defaultCacheTTL = 1 * time.Hour
const defaultMinRefreshInterval = 30 * time.Second

var AllChanTypes = []string{"mpim", "im", "public_channel", "private_channel"}
var PrivateChanType = "private_channel"
var PubChanType = "public_channel"

var ErrUsersNotReady = errors.New(usersNotReadyMsg)
var ErrChannelsNotReady = errors.New(channelsNotReadyMsg)
var ErrEmojisNotReady = errors.New(emojisNotReadyMsg)
var ErrRefreshRateLimited = errors.New("refresh skipped due to rate limiting")

// getCacheDir returns the appropriate cache directory for slack-mcp-server
func getCacheDir() string {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		// Fallback to current directory if we can't get user cache dir
		return "."
	}

	dir := filepath.Join(cacheDir, "slack-mcp-server")
	if err := os.MkdirAll(dir, 0755); err != nil {
		// Fallback to current directory if we can't create cache dir
		return "."
	}
	return dir
}

// getCacheTTL returns the cache TTL from SLACK_MCP_CACHE_TTL env var or default (1 hour).
// Supports formats: "1h", "30m", "3600" (seconds), "0" (disable TTL, cache forever)
// Negative values are rejected and fall back to default.
func getCacheTTL() time.Duration {
	ttlStr := os.Getenv("SLACK_MCP_CACHE_TTL")
	if ttlStr == "" {
		return defaultCacheTTL
	}

	// Try parsing as duration first (e.g., "1h", "30m")
	if d, err := time.ParseDuration(ttlStr); err == nil {
		if d < 0 {
			return defaultCacheTTL // Reject negative TTL
		}
		return d
	}

	// Try parsing as seconds (e.g., "3600")
	if secs, err := strconv.ParseInt(ttlStr, 10, 64); err == nil {
		if secs < 0 {
			return defaultCacheTTL // Reject negative TTL
		}
		return time.Duration(secs) * time.Second
	}

	return defaultCacheTTL
}

// getMinRefreshInterval returns the minimum interval between forced refreshes from
// SLACK_MCP_MIN_REFRESH_INTERVAL env var or default (30s).
// Supports formats: "30s", "1m", "60" (seconds), "0" (disable rate limiting)
// Negative values are rejected and fall back to default.
func getMinRefreshInterval() time.Duration {
	intervalStr := os.Getenv("SLACK_MCP_MIN_REFRESH_INTERVAL")
	if intervalStr == "" {
		return defaultMinRefreshInterval
	}

	// Try parsing as duration first (e.g., "30s", "1m")
	if d, err := time.ParseDuration(intervalStr); err == nil {
		if d < 0 {
			return defaultMinRefreshInterval // Reject negative interval
		}
		return d
	}

	// Try parsing as seconds (e.g., "60")
	if secs, err := strconv.ParseInt(intervalStr, 10, 64); err == nil {
		if secs < 0 {
			return defaultMinRefreshInterval // Reject negative interval
		}
		return time.Duration(secs) * time.Second
	}

	return defaultMinRefreshInterval
}

// validateAuthAndGetTeamID performs auth validation on startup and returns the TeamID.
// This ensures tokens are valid before proceeding and enables cache namespacing
// to prevent cache contamination when using multiple Slack workspaces.
// Returns an error if authentication fails - the server should not start with invalid credentials.
func validateAuthAndGetTeamID(authProvider auth.Provider, logger *zap.Logger) (string, error) {
	xoxpToken := os.Getenv("SLACK_MCP_XOXP_TOKEN")
	xoxcToken := os.Getenv("SLACK_MCP_XOXC_TOKEN")
	xoxdToken := os.Getenv("SLACK_MCP_XOXD_TOKEN")
	if xoxpToken == "demo" || (xoxcToken == "demo" && xoxdToken == "demo") {
		return "demo", nil
	}

	httpClient := transport.ProvideHTTPClient(authProvider.Cookies(), logger)
	slackOpts := []slack.Option{slack.OptionHTTPClient(httpClient)}
	if os.Getenv("SLACK_MCP_GOVSLACK") == "true" {
		slackOpts = append(slackOpts, slack.OptionAPIURL("https://slack-gov.com/api/"))
	}
	slackClient := slack.New(authProvider.SlackToken(), slackOpts...)

	authResp, err := slackClient.AuthTest()
	if err != nil {
		return "", err
	}

	logger.Info("Authenticated to Slack",
		zap.String("team", authResp.Team),
		zap.String("team_id", authResp.TeamID),
		zap.String("user", authResp.User))

	return authResp.TeamID, nil
}

// getCachePathWithTeamID returns a cache file path prefixed with TeamID for workspace isolation.
// If TeamID is empty, returns the default filename without prefix.
func getCachePathWithTeamID(teamID, filename string) string {
	cacheDir := getCacheDir()
	if teamID != "" {
		return filepath.Join(cacheDir, teamID+"_"+filename)
	}
	return filepath.Join(cacheDir, filename)
}

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
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	NameNormalized string   `json:"name_normalized,omitempty"`
	Topic          string   `json:"topic"`
	Purpose        string   `json:"purpose"`
	MemberCount    int      `json:"memberCount"`
	Members        []string `json:"members,omitempty"`
	IsMpIM         bool     `json:"mpim"`
	IsIM           bool     `json:"im"`
	IsPrivate      bool     `json:"private"`

	// Additional fields from boot data
	Creator            string   `json:"creator,omitempty"`
	Created            int64    `json:"created,omitempty"`
	Updated            int64    `json:"updated,omitempty"`
	IsArchived         bool     `json:"is_archived,omitempty"`
	IsMember           bool     `json:"is_member,omitempty"`
	IsGeneral          bool     `json:"is_general,omitempty"`
	IsShared           bool     `json:"is_shared,omitempty"`
	IsExtShared        bool     `json:"is_ext_shared,omitempty"`
	IsOrgShared        bool     `json:"is_org_shared,omitempty"`
	IsChannel          bool     `json:"is_channel,omitempty"`
	IsGroup            bool     `json:"is_group,omitempty"`
	IsFrozen           bool     `json:"is_frozen,omitempty"`
	IsPendingExtShared bool     `json:"is_pending_ext_shared,omitempty"`
	IsOpen             bool     `json:"is_open,omitempty"`
	Unlinked           int64    `json:"unlinked,omitempty"`
	ContextTeamID      string   `json:"context_team_id,omitempty"`
	SharedTeamIDs      []string `json:"shared_team_ids,omitempty"`
	LastRead           string   `json:"last_read,omitempty"`
	Latest             string   `json:"latest,omitempty"`
	User               string   `json:"user,omitempty"` // User ID for IM channels
}

type SlackAPI interface {
	// Standard slack-go API methods
	AuthTest() (*slack.AuthTestResponse, error)
	AuthTestContext(ctx context.Context) (*slack.AuthTestResponse, error)
	GetUsersContext(ctx context.Context, options ...slack.GetUsersOption) ([]slack.User, error)
	GetUsersInfo(users ...string) (*[]slack.User, error)
	PostMessageContext(ctx context.Context, channel string, options ...slack.MsgOption) (string, string, error)
	MarkConversationContext(ctx context.Context, channel, ts string) error
	AddReactionContext(ctx context.Context, name string, item slack.ItemRef) error
	RemoveReactionContext(ctx context.Context, name string, item slack.ItemRef) error

	// Used to get messages
	GetConversationHistoryContext(ctx context.Context, params *slack.GetConversationHistoryParameters) (*slack.GetConversationHistoryResponse, error)
	GetConversationRepliesContext(ctx context.Context, params *slack.GetConversationRepliesParameters) (msgs []slack.Message, hasMore bool, nextCursor string, err error)
	SearchContext(ctx context.Context, query string, params slack.SearchParameters) (*slack.SearchMessages, *slack.SearchFiles, error)

	// Used to get file information
	GetFileInfoContext(ctx context.Context, fileID string, count, page int) (*slack.File, []slack.Comment, *slack.Paging, error)
	GetFileContext(ctx context.Context, downloadURL string, writer io.Writer) error

	// File upload (V2 uses a 3-step process: get pre-signed URL, upload bytes to storage, finalize.
	// slack-go wraps all 3 steps. V1 files.upload is deprecated since March 2025.)
	UploadFileV2Context(ctx context.Context, params slack.UploadFileV2Parameters) (*slack.FileSummary, error)

	// Make a file's public URL active (files.sharedPublicURL)
	ShareFilePublicURLContext(ctx context.Context, fileID string) (*slack.File, []slack.Comment, *slack.Paging, error)

	// Used to get channel info (for unread counts with xoxp tokens)
	GetConversationInfoContext(ctx context.Context, input *slack.GetConversationInfoInput) (*slack.Channel, error)

	// Used to get channels list from both Slack and Enterprise Grid versions
	GetConversationsContext(ctx context.Context, params *slack.GetConversationsParameters) ([]slack.Channel, string, error)

	// Used to list only channels the calling user is a member of (users.conversations).
	// For xoxp tokens this is more efficient than conversations.list because it excludes
	// non-member public channels and closed DMs that cannot have unreads.
	GetConversationsForUserContext(ctx context.Context, params *slack.GetConversationsForUserParameters) ([]slack.Channel, string, error)

	// Edge API methods
	ClientUserBoot(ctx context.Context) (*edge.ClientUserBootResponse, error)
	UsersSearch(ctx context.Context, query string, count int) ([]slack.User, error)
	ClientCounts(ctx context.Context) (edge.ClientCountsResponse, error)
	GetMutedChannels(ctx context.Context) (map[string]bool, error)

	// Message management
	DeleteMessageContext(ctx context.Context, channel, messageTimestamp string) (string, string, error)
	UpdateMessageContext(ctx context.Context, channel, timestamp string, options ...slack.MsgOption) (string, string, string, error)

	// Channel members
	GetUsersInConversationContext(ctx context.Context, params *slack.GetUsersInConversationParameters) ([]string, string, error)

	// User information
	GetUserInfoContext(ctx context.Context, user string) (*slack.User, error)
	GetUserPresenceContext(ctx context.Context, user string) (*slack.UserPresence, error)

	// Bot information
	GetBotInfoContext(ctx context.Context, parameters slack.GetBotInfoParameters) (*slack.Bot, error)

	// Channel management
	CreateConversationContext(ctx context.Context, channelName string, isPrivate bool) (*slack.Channel, error)
	ArchiveConversationContext(ctx context.Context, channelID string) error
	SetTopicOfConversationContext(ctx context.Context, channelID, topic string) (*slack.Channel, error)
	SetPurposeOfConversationContext(ctx context.Context, channelID, purpose string) (*slack.Channel, error)

	// User groups API methods
	GetUserGroupsContext(ctx context.Context, options ...slack.GetUserGroupsOption) ([]slack.UserGroup, error)
	GetUserGroupMembersContext(ctx context.Context, userGroup string, options ...slack.GetUserGroupMembersOption) ([]string, error)
	CreateUserGroupContext(ctx context.Context, userGroup slack.UserGroup, options ...slack.CreateUserGroupOption) (slack.UserGroup, error)
	UpdateUserGroupContext(ctx context.Context, userGroupID string, options ...slack.UpdateUserGroupsOption) (slack.UserGroup, error)
	UpdateUserGroupMembersContext(ctx context.Context, userGroup string, members string, options ...slack.UpdateUserGroupMembersOption) (slack.UserGroup, error)
}

type MCPSlackClient struct {
	slackClient *slack.Client
	botClient   *slack.Client // Bot client for xoxb token (posts as bot identity)
	edgeClient  *edge.Client

	authResponse *slack.AuthTestResponse
	authProvider auth.Provider

	isEnterprise   bool
	isOAuth        bool
	isBotToken     bool
	edgeFailed     bool     // set when edge API fails; subsequent calls skip straight to standard API
	teamEndpoint   string
	workspaceTeams []string // Team IDs (e.g. T08U80K08H4) for workspaces the user belongs to (from enterprise_user.teams)
}

type ApiProvider struct {
	transport string
	client    SlackAPI
	logger    *zap.Logger

	rateLimiter        *rate.Limiter
	cacheTTL           time.Duration
	minRefreshInterval time.Duration

	// Users cache: atomic pointer to immutable snapshot (no copy on read)
	usersSnapshot          atomic.Pointer[UsersCache]
	usersCachePath         string
	usersReady             bool
	lastForcedUsersRefresh time.Time
	usersMu                sync.RWMutex // protects usersReady, lastForcedUsersRefresh

	// Channels cache: atomic pointer to immutable snapshot (no copy on read)
	channelsSnapshot          atomic.Pointer[ChannelsCache]
	channelsCachePath         string
	channelsReady             bool
	lastForcedChannelsRefresh time.Time
	channelsMu                sync.RWMutex // protects channelsReady, lastForcedChannelsRefresh

	emojis      map[string]Emoji
	emojisCache string
	emojisReady bool

	// Bot resolution: bot_id -> user mapping
	botIDToUser map[string]slack.User // B091T8Q8ETT -> User{ID: "U091T8Q8Q8Z", Name: "linear"}
	appIDToUser map[string]slack.User // AEMQ3Q4F4 -> User{ID: "U091T8Q8Q8Z", Name: "linear"}
}

func NewMCPSlackClient(authProvider auth.Provider, logger *zap.Logger) (*MCPSlackClient, error) {
	httpClient := transport.ProvideHTTPClient(authProvider.Cookies(), logger)

	slackOpts := []slack.Option{slack.OptionHTTPClient(httpClient)}
	if os.Getenv("SLACK_MCP_GOVSLACK") == "true" {
		slackOpts = append(slackOpts, slack.OptionAPIURL("https://slack-gov.com/api/"))
	}
	slackClient := slack.New(authProvider.SlackToken(), slackOpts...)

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
	token := authProvider.SlackToken()

	// Token type detection
	// isOAuth: Official OAuth tokens (xoxp or xoxb) - uses Standard API
	// isBotToken: Bot token - determines feature availability (e.g., search)
	isOAuth := strings.HasPrefix(token, "xoxp-") || strings.HasPrefix(token, "xoxb-")
	isBotToken := strings.HasPrefix(token, "xoxb-")

	// If Enterprise Grid, fetch user's workspace Team IDs from enterprise_user.teams
	var workspaceTeams []string
	if isEnterprise {
		userInfo, err := slackClient.GetUserInfoContext(context.Background(), authResp.UserID)
		if err == nil && userInfo != nil {
			// Extract Team IDs from enterprise_user.teams field
			if userInfo.Enterprise.Teams != nil {
				workspaceTeams = userInfo.Enterprise.Teams
			}
		}
	}

	// Initialize bot client if SLACK_MCP_BOT_TOKEN is set
	var botClient *slack.Client
	botToken := os.Getenv("SLACK_MCP_BOT_TOKEN")
	if botToken != "" {
		// Create a separate client for bot operations
		// Bot tokens don't need cookies, just the token
		botClient = slack.New(botToken,
			slack.OptionHTTPClient(httpClient),
			slack.OptionAPIURL(authResp.URL+"api/"),
		)

		// Validate the bot token
		botAuthResp, err := botClient.AuthTest()
		if err != nil {
			logger.Warn("Bot token validation failed, bot posting will be disabled",
				zap.Error(err))
			botClient = nil
		} else {
			logger.Info("Bot client initialized successfully",
				zap.String("bot_user", botAuthResp.User),
				zap.String("bot_id", botAuthResp.BotID),
				zap.String("context", "console"))
		}
	}

	return &MCPSlackClient{
		slackClient:    slackClient,
		botClient:      botClient,
		edgeClient:     edgeClient,
		authResponse:   authResponse,
		authProvider:   authProvider,
		isEnterprise:   isEnterprise,
		isOAuth:        isOAuth,
		isBotToken:     isBotToken,
		teamEndpoint:   authResp.URL,
		workspaceTeams: workspaceTeams,
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
		}

		// Enterprise + non-OAuth: try edge API first (for DMs, MPIMs, etc.),
		// then supplement with standard API. The edge API may only return
		// partial results (e.g., DMs succeed but SearchChannels fails on
		// restricted teams), so we always merge both sources.
		//
		// The edge API returns all results in one shot (no pagination),
		// while the standard API paginates. We fully paginate the standard
		// API here and return a merged, deduplicated result set with an
		// empty cursor so the caller doesn't need to re-paginate.
		if !c.edgeFailed {
			edgeChannels, _, edgeErr := c.edgeClient.GetConversationsContext(ctx, nil)
			if edgeErr != nil {
				c.edgeFailed = true
				return c.slackClient.GetConversationsContext(ctx, params)
			}

			// Collect edge results into a map for deduplication.
			seen := make(map[string]struct{}, len(edgeChannels))
			var channels []slack.Channel
			for _, ec := range edgeChannels {
				if params != nil && params.ExcludeArchived && ec.IsArchived {
					continue
				}
				seen[ec.ID] = struct{}{}
				channels = append(channels, slack.Channel{
					IsGeneral: ec.IsGeneral,
					IsMember:  ec.IsMember,
					GroupConversation: slack.GroupConversation{
						Conversation: slack.Conversation{
							ID:                 ec.ID,
							IsIM:               ec.IsIM,
							IsMpIM:             ec.IsMpIM,
							IsPrivate:          ec.IsPrivate,
							Created:            slack.JSONTime(ec.Created),
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
						Creator:    ec.Creator,
						IsArchived: ec.IsArchived,
						Members:    ec.Members,
						Topic: slack.Topic{
							Value:   ec.Topic.Value,
							Creator: ec.Topic.Creator,
							LastSet: slack.JSONTime(ec.Topic.LastSet),
						},
						Purpose: slack.Purpose{
							Value:   ec.Purpose.Value,
							Creator: ec.Purpose.Creator,
							LastSet: slack.JSONTime(ec.Purpose.LastSet),
						},
					},
				})
			}

			// Supplement with ALL pages from the standard API to fill gaps
			// the edge API missed (e.g., public/private channels on
			// restricted teams where SearchChannels returns an error).
			stdParams := &slack.GetConversationsParameters{
				Limit:           999,
				ExcludeArchived: true,
			}
			if params != nil {
				stdParams.Types = params.Types
			}
			for {
				stdChannels, nextCur, stdErr := c.slackClient.GetConversationsContext(ctx, stdParams)
				if stdErr != nil {
					break // standard API failed; keep what edge gave us
				}
				for _, sc := range stdChannels {
					if _, ok := seen[sc.ID]; !ok {
						seen[sc.ID] = struct{}{}
						channels = append(channels, sc)
					}
				}
				if nextCur == "" {
					break
				}
				stdParams.Cursor = nextCur
			}

			return channels, "", nil
		}

		// Edge API previously failed -- use standard API directly.
		return c.slackClient.GetConversationsContext(ctx, params)
	}

	return c.slackClient.GetConversationsContext(ctx, params)
}

func (c *MCPSlackClient) GetConversationsForUserContext(ctx context.Context, params *slack.GetConversationsForUserParameters) ([]slack.Channel, string, error) {
	return c.slackClient.GetConversationsForUserContext(ctx, params)
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

func (c *MCPSlackClient) GetFileInfoContext(ctx context.Context, fileID string, count, page int) (*slack.File, []slack.Comment, *slack.Paging, error) {
	return c.slackClient.GetFileInfoContext(ctx, fileID, count, page)
}

func (c *MCPSlackClient) GetFileContext(ctx context.Context, downloadURL string, writer io.Writer) error {
	return c.slackClient.GetFileContext(ctx, downloadURL, writer)
}

func (c *MCPSlackClient) UploadFileV2Context(ctx context.Context, params slack.UploadFileV2Parameters) (*slack.FileSummary, error) {
	return c.slackClient.UploadFileV2Context(ctx, params)
}

func (c *MCPSlackClient) ShareFilePublicURLContext(ctx context.Context, fileID string) (*slack.File, []slack.Comment, *slack.Paging, error) {
	return c.slackClient.ShareFilePublicURLContext(ctx, fileID)
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

func (c *MCPSlackClient) GetBotInfoContext(ctx context.Context, parameters slack.GetBotInfoParameters) (*slack.Bot, error) {
	// bots.info is available via standard API
	return c.slackClient.GetBotInfoContext(ctx, parameters)
}

// Channel management methods

func (c *MCPSlackClient) CreateConversationContext(ctx context.Context, channelName string, isPrivate bool) (*slack.Channel, error) {
	// This is the original method signature for compatibility
	// It uses the first workspace Team ID as default
	return c.CreateConversationInWorkspaceContext(ctx, channelName, isPrivate, "")
}

func (c *MCPSlackClient) CreateConversationInWorkspaceContext(ctx context.Context, channelName string, isPrivate bool, workspaceID string) (*slack.Channel, error) {
	// Use standard API for all cases - it works with browser tokens too!
	// The key is including the correct team_id for Enterprise Grid
	params := slack.CreateConversationParams{
		ChannelName: channelName,
		IsPrivate:   isPrivate,
	}

	// For Enterprise Grid, use the workspace Team ID (not Enterprise ID!)
	if c.isEnterprise && len(c.workspaceTeams) > 0 {
		// If specific workspace requested, validate it exists
		if workspaceID != "" {
			found := false
			for _, teamID := range c.workspaceTeams {
				if teamID == workspaceID {
					params.TeamID = workspaceID
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("workspace %s not found in user's workspaces: %v", workspaceID, c.workspaceTeams)
			}
		} else {
			// If no workspace specified and user has multiple workspaces, error
			if len(c.workspaceTeams) > 1 {
				return nil, fmt.Errorf("multiple workspaces available (%v), please specify which one to use", c.workspaceTeams)
			}
			// Use the only workspace available
			params.TeamID = c.workspaceTeams[0]
		}
	}

	channel, err := c.slackClient.CreateConversationContext(ctx, params)
	if err != nil {
		return nil, err
	}
	return channel, nil
}

func (c *MCPSlackClient) ArchiveConversationContext(ctx context.Context, channelID string) error {
	// ArchiveConversation archives a channel
	return c.slackClient.ArchiveConversationContext(ctx, channelID)
}

func (c *MCPSlackClient) SetTopicOfConversationContext(ctx context.Context, channelID, topic string) (*slack.Channel, error) {
	// SetTopicOfConversation sets the topic of a channel
	return c.slackClient.SetTopicOfConversationContext(ctx, channelID, topic)
}

func (c *MCPSlackClient) SetPurposeOfConversationContext(ctx context.Context, channelID, purpose string) (*slack.Channel, error) {
	// SetPurposeOfConversation sets the purpose (description) of a channel
	return c.slackClient.SetPurposeOfConversationContext(ctx, channelID, purpose)
}

func (c *MCPSlackClient) ClientUserBoot(ctx context.Context) (*edge.ClientUserBootResponse, error) {
	return c.edgeClient.ClientUserBoot(ctx)
}

func (c *MCPSlackClient) UsersSearch(ctx context.Context, query string, count int) ([]slack.User, error) {
	return c.edgeClient.UsersSearch(ctx, query, count)
}

func (c *MCPSlackClient) ClientCounts(ctx context.Context) (edge.ClientCountsResponse, error) {
	return c.edgeClient.ClientCounts(ctx)
}

func (c *MCPSlackClient) GetMutedChannels(ctx context.Context) (map[string]bool, error) {
	return c.edgeClient.GetMutedChannels(ctx)
}

func (c *MCPSlackClient) GetUserGroupsContext(ctx context.Context, options ...slack.GetUserGroupsOption) ([]slack.UserGroup, error) {
	return c.slackClient.GetUserGroupsContext(ctx, options...)
}

func (c *MCPSlackClient) GetUserGroupMembersContext(ctx context.Context, userGroup string, options ...slack.GetUserGroupMembersOption) ([]string, error) {
	return c.slackClient.GetUserGroupMembersContext(ctx, userGroup, options...)
}

func (c *MCPSlackClient) CreateUserGroupContext(ctx context.Context, userGroup slack.UserGroup, options ...slack.CreateUserGroupOption) (slack.UserGroup, error) {
	return c.slackClient.CreateUserGroupContext(ctx, userGroup, options...)
}

func (c *MCPSlackClient) UpdateUserGroupContext(ctx context.Context, userGroupID string, options ...slack.UpdateUserGroupsOption) (slack.UserGroup, error) {
	return c.slackClient.UpdateUserGroupContext(ctx, userGroupID, options...)
}

func (c *MCPSlackClient) UpdateUserGroupMembersContext(ctx context.Context, userGroup string, members string, options ...slack.UpdateUserGroupMembersOption) (slack.UserGroup, error) {
	return c.slackClient.UpdateUserGroupMembersContext(ctx, userGroup, members, options...)
}

func (c *MCPSlackClient) IsEnterprise() bool {
	return c.isEnterprise
}

func (c *MCPSlackClient) AuthResponse() *slack.AuthTestResponse {
	return c.authResponse
}

func (c *MCPSlackClient) GetWorkspaceTeams() []string {
	return c.workspaceTeams
}

func (c *MCPSlackClient) IsBotToken() bool {
	return c.isBotToken
}

func (c *MCPSlackClient) IsOAuth() bool {
	return c.isOAuth
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

// BotClient returns the bot Slack client, or nil if not configured
func (c *MCPSlackClient) BotClient() *slack.Client {
	return c.botClient
}

// HasBotClient returns true if a bot token was configured
func (c *MCPSlackClient) HasBotClient() bool {
	return c.botClient != nil
}

// PostMessageAsBotContext posts a message using the bot token
func (c *MCPSlackClient) PostMessageAsBotContext(ctx context.Context, channelID string, options ...slack.MsgOption) (string, string, error) {
	if c.botClient == nil {
		return "", "", errors.New("bot token not configured - set SLACK_MCP_BOT_TOKEN environment variable")
	}
	return c.botClient.PostMessageContext(ctx, channelID, options...)
}

func New(transport string, logger *zap.Logger) *ApiProvider {
	var (
		authProvider auth.ValueAuth
		err          error
	)

	// Read all environment variables
	xoxpToken := os.Getenv("SLACK_MCP_XOXP_TOKEN")
	xoxbToken := os.Getenv("SLACK_MCP_XOXB_TOKEN")
	xoxcToken := os.Getenv("SLACK_MCP_XOXC_TOKEN")
	xoxdToken := os.Getenv("SLACK_MCP_XOXD_TOKEN")

	// Warn if both user and bot tokens are set
	if xoxpToken != "" && xoxbToken != "" {
		logger.Warn(
			"Both SLACK_MCP_XOXP_TOKEN and SLACK_MCP_XOXB_TOKEN are set. "+
				"Using User token (xoxp) for full features. "+
				"Bot token will be ignored.",
			zap.String("context", "console"),
		)
	}

	// Priority 1: XOXP token (User OAuth)
	if xoxpToken != "" {
		authProvider, err = auth.NewValueAuth(xoxpToken, "")
		if err != nil {
			logger.Fatal("Failed to create auth provider with XOXP token", zap.Error(err))
		}

		return newWithXOXP(transport, authProvider, logger)
	}

	// Priority 2: XOXB token (Bot)
	if xoxbToken != "" {
		authProvider, err = auth.NewValueAuth(xoxbToken, "")
		if err != nil {
			logger.Fatal("Failed to create auth provider with XOXB token", zap.Error(err))
		}

		logger.Info("Using Bot token authentication",
			zap.String("context", "console"),
			zap.String("token_type", "xoxb"),
		)

		return newWithXOXB(transport, authProvider, logger)
	}

	// Priority 3: XOXC/XOXD tokens (session-based)
	if xoxcToken == "" || xoxdToken == "" {
		logger.Fatal("Authentication required: Either SLACK_MCP_XOXP_TOKEN, SLACK_MCP_XOXB_TOKEN, or both SLACK_MCP_XOXC_TOKEN and SLACK_MCP_XOXD_TOKEN must be provided")
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

	teamID, err := validateAuthAndGetTeamID(authProvider, logger)
	if err != nil {
		logger.Fatal("Authentication failed - check your Slack tokens", zap.Error(err))
	}

	usersCache := os.Getenv("SLACK_MCP_USERS_CACHE")
	if usersCache == "" {
		usersCache = getCachePathWithTeamID(teamID, "users_cache.json")
	}

	channelsCache := os.Getenv("SLACK_MCP_CHANNELS_CACHE")
	if channelsCache == "" {
		channelsCache = getCachePathWithTeamID(teamID, "channels_cache_v2.json")
	}

	emojisCache := os.Getenv("SLACK_MCP_EMOJIS_CACHE")
	if emojisCache == "" {
		emojisCache = getCachePathWithTeamID(teamID, "emojis_cache.json")
	}

	if os.Getenv("SLACK_MCP_XOXP_TOKEN") == "demo" || (os.Getenv("SLACK_MCP_XOXC_TOKEN") == "demo" && os.Getenv("SLACK_MCP_XOXD_TOKEN") == "demo") {
		logger.Info("Demo credentials are set, skip.")
	} else {
		client, err = NewMCPSlackClient(authProvider, logger)
		if err != nil {
			logger.Fatal("Failed to create MCP Slack client", zap.Error(err))
		}
	}

	ap := &ApiProvider{
		transport: transport,
		client:    client,
		logger:    logger,

		rateLimiter:        limiter.Tier2.Limiter(),
		cacheTTL:           getCacheTTL(),
		minRefreshInterval: getMinRefreshInterval(),

		usersCachePath:    usersCache,
		channelsCachePath: channelsCache,

		emojis:      make(map[string]Emoji),
		emojisCache: emojisCache,

		botIDToUser: make(map[string]slack.User),
		appIDToUser: make(map[string]slack.User),
	}
	// Initialize with empty snapshots
	ap.usersSnapshot.Store(&UsersCache{
		Users:    make(map[string]slack.User),
		UsersInv: make(map[string]string),
	})
	ap.channelsSnapshot.Store(&ChannelsCache{
		Channels:    make(map[string]Channel),
		ChannelsInv: make(map[string]string),
	})
	return ap
}

func newWithXOXB(transport string, authProvider auth.ValueAuth, logger *zap.Logger) *ApiProvider {
	// Bot tokens do not support demo mode, but otherwise share the same
	// initialization logic as user OAuth tokens.
	return newWithXOXP(transport, authProvider, logger)
}

func newWithXOXC(transport string, authProvider auth.ValueAuth, logger *zap.Logger) *ApiProvider {
	var (
		client *MCPSlackClient
		err    error
	)

	teamID, err := validateAuthAndGetTeamID(authProvider, logger)
	if err != nil {
		logger.Fatal("Authentication failed - check your Slack tokens", zap.Error(err))
	}

	usersCache := os.Getenv("SLACK_MCP_USERS_CACHE")
	if usersCache == "" {
		usersCache = getCachePathWithTeamID(teamID, "users_cache.json")
	}

	channelsCache := os.Getenv("SLACK_MCP_CHANNELS_CACHE")
	if channelsCache == "" {
		channelsCache = getCachePathWithTeamID(teamID, "channels_cache_v2.json")
	}

	emojisCache := os.Getenv("SLACK_MCP_EMOJIS_CACHE")
	if emojisCache == "" {
		emojisCache = getCachePathWithTeamID(teamID, "emojis_cache.json")
	}

	if os.Getenv("SLACK_MCP_XOXP_TOKEN") == "demo" || (os.Getenv("SLACK_MCP_XOXC_TOKEN") == "demo" && os.Getenv("SLACK_MCP_XOXD_TOKEN") == "demo") {
		logger.Info("Demo credentials are set, skip.")
	} else {
		client, err = NewMCPSlackClient(authProvider, logger)
		if err != nil {
			logger.Fatal("Failed to create MCP Slack client", zap.Error(err))
		}
	}

	ap := &ApiProvider{
		transport: transport,
		client:    client,
		logger:    logger,

		rateLimiter:        limiter.Tier2.Limiter(),
		cacheTTL:           getCacheTTL(),
		minRefreshInterval: getMinRefreshInterval(),

		usersCachePath:    usersCache,
		channelsCachePath: channelsCache,

		emojis:      make(map[string]Emoji),
		emojisCache: emojisCache,

		botIDToUser: make(map[string]slack.User),
		appIDToUser: make(map[string]slack.User),
	}
	// Initialize with empty snapshots
	ap.usersSnapshot.Store(&UsersCache{
		Users:    make(map[string]slack.User),
		UsersInv: make(map[string]string),
	})
	ap.channelsSnapshot.Store(&ChannelsCache{
		Channels:    make(map[string]Channel),
		ChannelsInv: make(map[string]string),
	})
	return ap
}

func (ap *ApiProvider) RefreshUsers(ctx context.Context) error {
	return ap.refreshUsersInternal(ctx, false)
}

// ForceRefreshUsers bypasses the cache and fetches fresh user data from Slack API.
// Rate limited by SLACK_MCP_MIN_REFRESH_INTERVAL (default 30s) to prevent API abuse.
// Returns ErrRefreshRateLimited if refresh is skipped due to rate limiting.
func (ap *ApiProvider) ForceRefreshUsers(ctx context.Context) error {
	if ap.minRefreshInterval > 0 {
		// Use single lock scope for check-and-update to prevent TOCTOU race
		ap.usersMu.Lock()
		sinceLast := time.Since(ap.lastForcedUsersRefresh)
		if sinceLast < ap.minRefreshInterval {
			ap.usersMu.Unlock()
			ap.logger.Debug("Skipping forced users refresh, within rate limit",
				zap.Duration("since_last", sinceLast),
				zap.Duration("min_interval", ap.minRefreshInterval))
			return ErrRefreshRateLimited
		}
		// Update timestamp before refresh to prevent concurrent forced refreshes
		ap.lastForcedUsersRefresh = time.Now()
		ap.usersMu.Unlock()
	}

	ap.logger.Info("Force refreshing users cache")
	return ap.refreshUsersInternal(ctx, true)
}

func (ap *ApiProvider) refreshUsersInternal(ctx context.Context, force bool) error {
	ap.usersMu.Lock()
	defer ap.usersMu.Unlock()

	var (
		list        []slack.User
		optionLimit = slack.GetUsersOptionLimit(1000)
	)

	// Check if we should use cache (not forced, cache exists, and within TTL)
	if !force {
		if data, err := os.ReadFile(ap.usersCachePath); err == nil {
			var cachedUsers []slack.User
			if err := json.Unmarshal(data, &cachedUsers); err != nil {
				ap.logger.Warn("Failed to unmarshal users cache, will refetch",
					zap.String("cache_file", ap.usersCachePath),
					zap.Error(err))
			} else if len(cachedUsers) == 0 {
				ap.logger.Warn("Users cache is empty or null, will refetch",
					zap.String("cache_file", ap.usersCachePath))
			} else {
				// Check cache TTL using file modification time
				cacheValid := true
				if ap.cacheTTL > 0 {
					if fileInfo, err := os.Stat(ap.usersCachePath); err == nil {
						cacheAge := time.Since(fileInfo.ModTime())
						if cacheAge > ap.cacheTTL {
							ap.logger.Info("Users cache expired, will refetch",
								zap.Duration("cache_age", cacheAge),
								zap.Duration("ttl", ap.cacheTTL),
								zap.String("cache_file", ap.usersCachePath))
							cacheValid = false
						}
					}
				}

				if cacheValid {
					// Build new snapshot from cache
					newSnapshot := &UsersCache{
						Users:    make(map[string]slack.User, len(cachedUsers)),
						UsersInv: make(map[string]string, len(cachedUsers)),
					}
					for _, u := range cachedUsers {
						newSnapshot.Users[u.ID] = u
						newSnapshot.UsersInv[u.Name] = u.ID
					}
					ap.usersSnapshot.Store(newSnapshot)
					ap.logger.Info("Loaded users from cache",
						zap.Int("count", len(cachedUsers)),
						zap.String("cache_file", ap.usersCachePath))
					ap.usersReady = true
					return nil
				}
			}
		}
	}

	// Fetch fresh data from Slack API
	users, err := ap.client.GetUsersContext(ctx,
		optionLimit,
	)
	if err != nil {
		ap.logger.Error("Failed to fetch users", zap.Error(err))
		return err
	}
	list = append(list, users...)

	// Build new snapshot
	newSnapshot := &UsersCache{
		Users:    make(map[string]slack.User),
		UsersInv: make(map[string]string),
	}
	for _, user := range users {
		newSnapshot.Users[user.ID] = user
		newSnapshot.UsersInv[user.Name] = user.ID
	}
	// Store intermediate snapshot so GetSlackConnect can read current users
	ap.usersSnapshot.Store(newSnapshot)

	connectUsers, err := ap.GetSlackConnect(ctx)
	if err != nil {
		ap.logger.Error("Failed to fetch users from Slack Connect", zap.Error(err))
		return err
	}
	list = append(list, connectUsers...)

	// Add Slack Connect users to a new snapshot (since maps are shared)
	if len(connectUsers) > 0 {
		finalSnapshot := &UsersCache{
			Users:    make(map[string]slack.User, len(newSnapshot.Users)+len(connectUsers)),
			UsersInv: make(map[string]string, len(newSnapshot.UsersInv)+len(connectUsers)),
		}
		for k, v := range newSnapshot.Users {
			finalSnapshot.Users[k] = v
		}
		for k, v := range newSnapshot.UsersInv {
			finalSnapshot.UsersInv[k] = v
		}
		for _, user := range connectUsers {
			finalSnapshot.Users[user.ID] = user
			finalSnapshot.UsersInv[user.Name] = user.ID
		}
		ap.usersSnapshot.Store(finalSnapshot)
	}

	if data, err := json.MarshalIndent(list, "", "  "); err != nil {
		ap.logger.Error("Failed to marshal users for cache", zap.Error(err))
	} else {
		if err := os.WriteFile(ap.usersCachePath, data, 0644); err != nil {
			ap.logger.Error("Failed to write cache file",
				zap.String("cache_file", ap.usersCachePath),
				zap.Error(err))
		} else {
			ap.logger.Info("Wrote users to cache",
				zap.Int("count", len(list)),
				zap.String("cache_file", ap.usersCachePath))
		}
	}

	// No need to build app_id mapping at startup - we'll do it on-demand in ResolveBotIDToUser

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
		"thumbsup":         "\xf0\x9f\x91\x8d",
		"thumbsdown":       "\xf0\x9f\x91\x8e",
		"heart":            "\xe2\x9d\xa4\xef\xb8\x8f",
		"smile":            "\xf0\x9f\x98\x8a",
		"laughing":         "\xf0\x9f\x98\x82",
		"cry":              "\xf0\x9f\x98\xa2",
		"angry":            "\xf0\x9f\x98\xa0",
		"clap":             "\xf0\x9f\x91\x8f",
		"fire":             "\xf0\x9f\x94\xa5",
		"eyes":             "\xf0\x9f\x91\x80",
		"rocket":           "\xf0\x9f\x9a\x80",
		"100":              "\xf0\x9f\x92\xaf",
		"pray":             "\xf0\x9f\x99\x8f",
		"tada":             "\xf0\x9f\x8e\x89",
		"white_check_mark": "\xe2\x9c\x85",
		"x":                "\xe2\x9d\x8c",
		"warning":          "\xe2\x9a\xa0\xef\xb8\x8f",
		"question":         "\xe2\x9d\x93",
		"exclamation":      "\xe2\x9d\x97",
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
	return ap.refreshChannelsInternal(ctx, false)
}

// ForceRefreshChannels bypasses the cache and fetches fresh channel data from Slack API.
// Use this when a channel lookup fails to attempt recovery with fresh data.
// Rate limited by SLACK_MCP_MIN_REFRESH_INTERVAL (default 30s) to prevent API abuse.
// Returns ErrRefreshRateLimited if refresh is skipped due to rate limiting.
func (ap *ApiProvider) ForceRefreshChannels(ctx context.Context) error {
	if ap.minRefreshInterval > 0 {
		// Use single lock scope for check-and-update to prevent TOCTOU race
		ap.channelsMu.Lock()
		sinceLast := time.Since(ap.lastForcedChannelsRefresh)
		if sinceLast < ap.minRefreshInterval {
			ap.channelsMu.Unlock()
			ap.logger.Debug("Skipping forced channels refresh, within rate limit",
				zap.Duration("since_last", sinceLast),
				zap.Duration("min_interval", ap.minRefreshInterval))
			return ErrRefreshRateLimited
		}
		// Update timestamp before refresh to prevent concurrent forced refreshes
		ap.lastForcedChannelsRefresh = time.Now()
		ap.channelsMu.Unlock()
	}

	ap.logger.Info("Force refreshing channels cache")
	return ap.refreshChannelsInternal(ctx, true)
}

func (ap *ApiProvider) refreshChannelsInternal(ctx context.Context, force bool) error {
	ap.channelsMu.Lock()
	defer ap.channelsMu.Unlock()

	// Check if we should use cache (not forced, cache exists, and within TTL)
	if !force {
		if data, err := os.ReadFile(ap.channelsCachePath); err == nil {
			var cachedChannels []Channel
			if err := json.Unmarshal(data, &cachedChannels); err != nil {
				ap.logger.Warn("Failed to unmarshal channels cache, will refetch",
					zap.String("cache_file", ap.channelsCachePath),
					zap.Error(err))
			} else if len(cachedChannels) == 0 {
				ap.logger.Warn("Channels cache is empty or null, will refetch",
					zap.String("cache_file", ap.channelsCachePath))
			} else {
				// Check cache TTL using file modification time
				cacheValid := true
				if ap.cacheTTL > 0 {
					if fileInfo, err := os.Stat(ap.channelsCachePath); err == nil {
						cacheAge := time.Since(fileInfo.ModTime())
						if cacheAge > ap.cacheTTL {
							ap.logger.Info("Channels cache expired, will refetch",
								zap.Duration("cache_age", cacheAge),
								zap.Duration("ttl", ap.cacheTTL),
								zap.String("cache_file", ap.channelsCachePath))
							cacheValid = false
						}
					}
				}

				if cacheValid {
					// Re-map channels with current users cache to ensure DM names are populated
					usersMap := ap.ProvideUsersMap().Users
					newSnapshot := &ChannelsCache{
						Channels:    make(map[string]Channel, len(cachedChannels)),
						ChannelsInv: make(map[string]string, len(cachedChannels)),
					}
					for _, c := range cachedChannels {
						// For IM channels, re-generate the name and purpose using current users cache
						if c.IsIM {
							// Re-map the channel to get updated user name if available
							remappedChannel := mapChannel(
								c.ID, "", "", c.Topic, c.Purpose,
								c.User, c.Members, c.MemberCount,
								c.IsIM, c.IsMpIM, c.IsPrivate, c.IsExtShared,
								usersMap,
							)
							newSnapshot.Channels[c.ID] = remappedChannel
							newSnapshot.ChannelsInv[remappedChannel.Name] = c.ID
						} else {
							newSnapshot.Channels[c.ID] = c
							newSnapshot.ChannelsInv[c.Name] = c.ID
						}
					}
					ap.channelsSnapshot.Store(newSnapshot)
					ap.logger.Info("Loaded channels from cache and re-mapped DM names",
						zap.Int("count", len(cachedChannels)),
						zap.String("cache_file", ap.channelsCachePath))
					ap.channelsReady = true
					return nil
				}
			}
		}
	}

	// Fetch fresh data from Slack API
	channels := ap.GetChannels(ctx, AllChanTypes)

	if len(channels) == 0 {
		ap.logger.Warn("No channels fetched from Slack API, not writing empty cache",
			zap.String("cache_file", ap.channelsCachePath))
	} else if data, err := json.MarshalIndent(channels, "", "  "); err != nil {
		ap.logger.Error("Failed to marshal channels for cache", zap.Error(err))
	} else {
		if err := os.WriteFile(ap.channelsCachePath, data, 0644); err != nil {
			ap.logger.Error("Failed to write cache file",
				zap.String("cache_file", ap.channelsCachePath),
				zap.Error(err))
		} else {
			ap.logger.Info("Wrote channels to cache",
				zap.Int("count", len(channels)),
				zap.String("cache_file", ap.channelsCachePath))
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

	usersSnapshot := ap.usersSnapshot.Load()
	var collectedIDs []string
	for _, im := range boot.IMs {
		if !im.IsShared && !im.IsExtShared {
			continue
		}

		_, ok := usersSnapshot.Users[im.User]
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

func (ap *ApiProvider) GetChannelsType(ctx context.Context, channelType string) []Channel {
	return ap.getChannelsMultiType(ctx, []string{channelType})
}

func (ap *ApiProvider) getChannelsMultiType(ctx context.Context, channelTypes []string) []Channel {
	params := &slack.GetConversationsParameters{
		Types:           channelTypes,
		Limit:           999,
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
		ap.logger.Debug("Fetched channels",
			zap.Strings("channelTypes", channelTypes),
			zap.Int("count", len(channels)),
		)
		if err != nil {
			ap.logger.Error("Failed to fetch channels", zap.Error(err))
			break
		}

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
				channel.IsExtShared,
				ap.ProvideUsersMap().Users,
			)
			chans = append(chans, ch)
		}

		if nextcur == "" {
			break
		}

		params.Cursor = nextcur
	}
	return chans
}

func (ap *ApiProvider) GetChannels(ctx context.Context, channelTypes []string) []Channel {
	if len(channelTypes) == 0 {
		channelTypes = AllChanTypes
	}

	// Fetch all channel types in a single paginated call. The standard
	// conversations.list API supports multiple types per request, and the edge
	// API (Enterprise Grid + non-OAuth) returns all types regardless. This
	// avoids making 4 separate API round-trips (one per type).
	chans := ap.getChannelsMultiType(ctx, AllChanTypes)

	// Build new snapshot with all fetched channels
	newSnapshot := &ChannelsCache{
		Channels:    make(map[string]Channel, len(chans)),
		ChannelsInv: make(map[string]string, len(chans)),
	}
	for _, ch := range chans {
		newSnapshot.Channels[ch.ID] = ch
		newSnapshot.ChannelsInv[ch.Name] = ch.ID
	}
	ap.channelsSnapshot.Store(newSnapshot)

	// Filter by requested channel types
	var res []Channel
	for _, t := range channelTypes {
		for _, channel := range newSnapshot.Channels {
			if t == "public_channel" && !channel.IsPrivate && !channel.IsIM && !channel.IsMpIM {
				res = append(res, channel)
			}
			if t == "private_channel" && channel.IsPrivate && !channel.IsIM && !channel.IsMpIM {
				res = append(res, channel)
			}
			if t == "im" && channel.IsIM {
				res = append(res, channel)
			}
			if t == "mpim" && channel.IsMpIM {
				res = append(res, channel)
			}
		}
	}

	return res
}

func (ap *ApiProvider) ProvideUsersMap() *UsersCache {
	// Atomic load - no lock needed, snapshot is immutable
	return ap.usersSnapshot.Load()
}

func (ap *ApiProvider) ProvideChannelsMaps() *ChannelsCache {
	// Atomic load - no lock needed, snapshot is immutable
	return ap.channelsSnapshot.Load()
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

func (ap *ApiProvider) IsBotToken() bool {
	client, ok := ap.client.(*MCPSlackClient)
	return ok && client != nil && client.IsBotToken()
}

func (ap *ApiProvider) IsOAuth() bool {
	client, ok := ap.client.(*MCPSlackClient)
	return ok && client != nil && client.IsOAuth()
}

// SlackBot returns the bot client for bot-identity posting
// Returns nil if bot token is not configured
func (ap *ApiProvider) SlackBot() *slack.Client {
	if mcp, ok := ap.client.(*MCPSlackClient); ok {
		return mcp.BotClient()
	}
	return nil
}

// HasSlackBot returns true if bot posting is available
func (ap *ApiProvider) HasSlackBot() bool {
	if mcp, ok := ap.client.(*MCPSlackClient); ok {
		return mcp.HasBotClient()
	}
	return false
}

// SearchUsers searches for users by name, email, or display name.
// For OAuth tokens (xoxp/xoxb), it searches the local users cache using regex matching.
// For browser tokens (xoxc/xoxd), it uses the edge API's UsersSearch method.
func (ap *ApiProvider) SearchUsers(ctx context.Context, query string, limit int) ([]slack.User, error) {
	if ap.IsOAuth() {
		return ap.searchUsersInCache(query, limit)
	}

	return ap.client.UsersSearch(ctx, query, limit)
}

// searchUsersInCache performs a case-insensitive regex search on cached users.
// Matches against username, real name, display name, and email.
func (ap *ApiProvider) searchUsersInCache(query string, limit int) ([]slack.User, error) {
	if !ap.usersReady {
		return nil, ErrUsersNotReady
	}

	pattern, err := regexp.Compile("(?i)" + regexp.QuoteMeta(query))
	if err != nil {
		return nil, err
	}

	usersCache := ap.usersSnapshot.Load()
	var results []slack.User
	for _, user := range usersCache.Users {
		if user.Deleted {
			continue
		}

		if pattern.MatchString(user.Name) ||
			pattern.MatchString(user.RealName) ||
			pattern.MatchString(user.Profile.DisplayName) ||
			pattern.MatchString(user.Profile.Email) {
			results = append(results, user)

			if len(results) >= limit {
				break
			}
		}
	}

	return results, nil
}

func mapChannel(
	id, name, nameNormalized, topic, purpose, user string,
	members []string,
	numMembers int,
	isIM, isMpIM, isPrivate, isExtShared bool,
	usersMap map[string]slack.User,
) Channel {
	channelName := name
	finalPurpose := purpose
	finalTopic := topic
	finalMemberCount := numMembers

	var userID string
	if isIM {
		finalMemberCount = 2
		userID = user // Store the user ID for later re-mapping

		// If user field is empty but we have members, try to extract from members
		if userID == "" && len(members) > 0 {
			// For IM channels, members should contain the other user's ID
			// Try each member to find a valid user in the users map
			for _, memberID := range members {
				if _, ok := usersMap[memberID]; ok {
					userID = memberID
					break
				}
			}
		}

		if u, ok := usersMap[userID]; ok {
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
		} else if userID != "" {
			channelName = "@" + userID
			finalPurpose = "DM with " + userID
		} else {
			channelName = "@"
			finalPurpose = "DM with "
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
		ID:             id,
		Name:           channelName,
		NameNormalized: nameNormalized,
		Topic:          finalTopic,
		Purpose:        finalPurpose,
		MemberCount:    finalMemberCount,
		Members:        members,
		IsIM:           isIM,
		IsMpIM:         isMpIM,
		IsPrivate:      isPrivate,
		IsExtShared:    isExtShared,
		User:           userID,
	}
}

// ResolveBotIDToUser resolves a bot ID to a user, using cache or API
func (ap *ApiProvider) ResolveBotIDToUser(botID string) (slack.User, bool) {
	// Check cache first
	if user, ok := ap.botIDToUser[botID]; ok {
		ap.logger.Debug("Bot ID resolved from cache",
			zap.String("bot_id", botID),
			zap.String("user_name", user.Name))
		return user, true
	}

	// Try to fetch bot info via API
	ctx := context.Background()
	botInfo, err := ap.client.GetBotInfoContext(ctx, slack.GetBotInfoParameters{
		Bot: botID,
	})
	if err != nil {
		ap.logger.Debug("Failed to fetch bot info",
			zap.String("bot_id", botID),
			zap.Error(err))
		return slack.User{}, false
	}

	// Look up user by app_id - first check cache, then search users list

	// First check if we've already cached this app_id mapping
	if user, ok := ap.appIDToUser[botInfo.AppID]; ok {
		// Cache the bot->user mapping for next time
		ap.botIDToUser[botID] = user
		ap.logger.Debug("Bot ID resolved via cached app_id",
			zap.String("bot_id", botID),
			zap.String("app_id", botInfo.AppID),
			zap.String("user_id", user.ID),
			zap.String("user_name", user.Name))
		return user, true
	}

	// Not in cache, search through users list for matching app_id
	usersSnapshot := ap.usersSnapshot.Load()
	for _, user := range usersSnapshot.Users {
		if user.IsBot && user.Profile.ApiAppID == botInfo.AppID {
			// Found it! Cache both mappings
			ap.appIDToUser[botInfo.AppID] = user
			ap.botIDToUser[botID] = user
			ap.logger.Debug("Bot ID resolved by searching users",
				zap.String("bot_id", botID),
				zap.String("app_id", botInfo.AppID),
				zap.String("user_id", user.ID),
				zap.String("user_name", user.Name))
			return user, true
		}
	}

	ap.logger.Debug("App ID not found in users list",
		zap.String("bot_id", botID),
		zap.String("app_id", botInfo.AppID),
		zap.String("bot_name", botInfo.Name))

	// No user found, but we have bot info - create a pseudo-user
	pseudoUser := slack.User{
		ID:       botID,
		Name:     strings.ToLower(botInfo.Name),
		RealName: botInfo.Name,
		IsBot:    true,
	}

	// Cache it
	ap.botIDToUser[botID] = pseudoUser
	ap.logger.Debug("Created pseudo-user for bot",
		zap.String("bot_id", botID),
		zap.String("bot_name", botInfo.Name))

	return pseudoUser, true
}
