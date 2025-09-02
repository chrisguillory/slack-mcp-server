package handler

import (
	"context"

	"github.com/gocarina/gocsv"
	"github.com/korotovsky/slack-mcp-server/pkg/provider"
	"github.com/mark3labs/mcp-go/mcp"
	"go.uber.org/zap"
)

type CurrentUser struct {
	UserID       string `csv:"user_id"`
	UserName     string `csv:"user_name"`
	TeamID       string `csv:"team_id"`
	TeamName     string `csv:"team_name"`
	WorkspaceURL string `csv:"workspace_url"`
	EnterpriseID string `csv:"enterprise_id,omitempty"`
}

type AuthHandler struct {
	apiProvider *provider.ApiProvider
	logger      *zap.Logger
}

func NewAuthHandler(apiProvider *provider.ApiProvider, logger *zap.Logger) *AuthHandler {
	return &AuthHandler{
		apiProvider: apiProvider,
		logger:      logger,
	}
}

// GetCurrentUserHandler returns information about the authenticated user
func (ah *AuthHandler) GetCurrentUserHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ah.logger.Debug("GetCurrentUserHandler called")

	// Call auth.test to get current user information
	authResponse, err := ah.apiProvider.Slack().AuthTestContext(ctx)
	if err != nil {
		ah.logger.Error("AuthTestContext failed", zap.Error(err))
		return mcp.NewToolResultErrorFromErr("Failed to get current user information", err), nil
	}

	ah.logger.Debug("Auth test successful",
		zap.String("user", authResponse.User),
		zap.String("user_id", authResponse.UserID),
		zap.String("team", authResponse.Team),
		zap.String("team_id", authResponse.TeamID),
	)

	// Create user info structure
	currentUser := CurrentUser{
		UserID:       authResponse.UserID,
		UserName:     authResponse.User,
		TeamID:       authResponse.TeamID,
		TeamName:     authResponse.Team,
		WorkspaceURL: authResponse.URL,
	}

	// Include enterprise ID if present (for Enterprise Grid)
	if authResponse.EnterpriseID != "" {
		currentUser.EnterpriseID = authResponse.EnterpriseID
	}

	// Format as CSV
	csvBytes, err := gocsv.MarshalBytes([]*CurrentUser{&currentUser})
	if err != nil {
		ah.logger.Error("Failed to marshal user info to CSV", zap.Error(err))
		return mcp.NewToolResultErrorFromErr("Failed to format user information as CSV", err), nil
	}

	ah.logger.Debug("Returning current user info",
		zap.String("user_id", currentUser.UserID),
		zap.String("team_id", currentUser.TeamID),
	)

	return mcp.NewToolResultText(string(csvBytes)), nil
}
