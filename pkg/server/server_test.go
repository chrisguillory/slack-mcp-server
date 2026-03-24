package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestShouldAddTool_ReadOnly_EmptyEnabledTools(t *testing.T) {
	t.Run("all read-only tools registered with empty enabledTools", func(t *testing.T) {
		readOnlyTools := []string{
			ToolGetChannelMessages,
			ToolGetThreadMessages,
			ToolSearchMessages,
			ToolListChannels,
			ToolListUsers,
		}
		for _, tool := range readOnlyTools {
			result := shouldAddTool(tool, []string{}, "")
			assert.True(t, result, "tool %s should be registered when enabledTools is empty", tool)
		}
	})

	t.Run("all read-only tools registered with nil enabledTools", func(t *testing.T) {
		result := shouldAddTool(ToolGetChannelMessages, nil, "")
		assert.True(t, result, "tool should be registered when enabledTools is nil")
	})

	t.Run("unknown tools also registered with empty enabledTools", func(t *testing.T) {
		result := shouldAddTool("future_new_tool", []string{}, "")
		assert.True(t, result, "unknown tools should be registered when enabledTools is empty")
	})
}

func TestShouldAddTool_ReadOnly_ExplicitEnabledTools(t *testing.T) {
	tests := []struct {
		name         string
		toolName     string
		enabledTools []string
		expected     bool
	}{
		{
			name:         "tool in enabledTools list is registered",
			toolName:     ToolGetChannelMessages,
			enabledTools: []string{ToolGetChannelMessages, ToolListChannels},
			expected:     true,
		},
		{
			name:         "tool not in enabledTools list is not registered",
			toolName:     ToolPostMessage,
			enabledTools: []string{ToolGetChannelMessages, ToolListChannels},
			expected:     false,
		},
		{
			name:         "read-only tool blocked when not in explicit list",
			toolName:     ToolGetChannelMessages,
			enabledTools: []string{ToolListChannels},
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldAddTool(tt.toolName, tt.enabledTools, "")
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestShouldAddTool_SingleToolEnabled(t *testing.T) {
	enabledTools := []string{ToolListChannels}

	for _, tool := range ValidToolNames {
		result := shouldAddTool(tool, enabledTools, "")
		if tool == ToolListChannels {
			assert.True(t, result, "list_channels should be registered")
		} else {
			assert.False(t, result, "%s should NOT be registered when only list_channels is enabled", tool)
		}
	}
}

func TestValidToolNames(t *testing.T) {
	t.Run("ValidToolNames contains all expected tools", func(t *testing.T) {
		expectedTools := map[string]bool{
			ToolGetCurrentUser:        true,
			ToolGetChannelMessages:    true,
			ToolGetThreadMessages:     true,
			ToolPostMessage:           true,
			ToolPostMessageAsBot:      true,
			ToolAddReaction:           true,
			ToolRemoveReaction:        true,
			ToolDeleteMessage:         true,
			ToolUpdateMessage:         true,
			ToolUpdateMessageAsBot:    true,
			ToolDeleteMessageAsBot:    true,
			ToolSearchMessages:        true,
			ToolListChannels:          true,
			ToolListChannelMembers:    true,
			ToolListUsers:             true,
			ToolGetUserInfo:           true,
			ToolGetOrgOverview:        true,
			ToolCreateChannel:         true,
			ToolArchiveChannel:        true,
			ToolListEmojis:            true,
			ToolDownloadFile:          true,
			ToolGetFileInfo:           true,
			ToolUploadFile:            true,
			ToolMakeFilePublic:        true,
			ToolGetSlackTemplates:     true,
			ToolConversationsUnreads:  true,
			ToolConversationsMark:     true,
			ToolUsergroupsList:        true,
			ToolUsergroupsMe:          true,
			ToolUsergroupsCreate:      true,
			ToolUsergroupsUpdate:      true,
			ToolUsergroupsUsersUpdate: true,
		}

		assert.Equal(t, len(expectedTools), len(ValidToolNames), "ValidToolNames should have %d tools", len(expectedTools))

		for _, tool := range ValidToolNames {
			assert.True(t, expectedTools[tool], "unexpected tool in ValidToolNames: %s", tool)
		}
	})

	t.Run("constants match their string values", func(t *testing.T) {
		assert.Equal(t, "get_current_user", ToolGetCurrentUser)
		assert.Equal(t, "get_channel_messages", ToolGetChannelMessages)
		assert.Equal(t, "get_thread_messages", ToolGetThreadMessages)
		assert.Equal(t, "post_message", ToolPostMessage)
		assert.Equal(t, "post_message_as_bot", ToolPostMessageAsBot)
		assert.Equal(t, "add_reaction", ToolAddReaction)
		assert.Equal(t, "remove_reaction", ToolRemoveReaction)
		assert.Equal(t, "delete_message", ToolDeleteMessage)
		assert.Equal(t, "update_message", ToolUpdateMessage)
		assert.Equal(t, "update_message_as_bot", ToolUpdateMessageAsBot)
		assert.Equal(t, "delete_message_as_bot", ToolDeleteMessageAsBot)
		assert.Equal(t, "search_messages", ToolSearchMessages)
		assert.Equal(t, "list_channels", ToolListChannels)
		assert.Equal(t, "list_channel_members", ToolListChannelMembers)
		assert.Equal(t, "list_users", ToolListUsers)
		assert.Equal(t, "get_user_info", ToolGetUserInfo)
		assert.Equal(t, "get_org_overview", ToolGetOrgOverview)
		assert.Equal(t, "create_channel", ToolCreateChannel)
		assert.Equal(t, "archive_channel", ToolArchiveChannel)
		assert.Equal(t, "list_emojis", ToolListEmojis)
		assert.Equal(t, "download_file", ToolDownloadFile)
		assert.Equal(t, "get_file_info", ToolGetFileInfo)
		assert.Equal(t, "upload_file", ToolUploadFile)
		assert.Equal(t, "make_file_public", ToolMakeFilePublic)
		assert.Equal(t, "get_slack_templates", ToolGetSlackTemplates)
		assert.Equal(t, "conversations_unreads", ToolConversationsUnreads)
		assert.Equal(t, "conversations_mark", ToolConversationsMark)
		assert.Equal(t, "usergroups_list", ToolUsergroupsList)
		assert.Equal(t, "usergroups_me", ToolUsergroupsMe)
		assert.Equal(t, "usergroups_create", ToolUsergroupsCreate)
		assert.Equal(t, "usergroups_update", ToolUsergroupsUpdate)
		assert.Equal(t, "usergroups_users_update", ToolUsergroupsUsersUpdate)
	})
}

func TestValidateEnabledTools(t *testing.T) {
	t.Run("empty list is valid", func(t *testing.T) {
		err := ValidateEnabledTools([]string{})
		assert.NoError(t, err)
	})

	t.Run("nil list is valid", func(t *testing.T) {
		err := ValidateEnabledTools(nil)
		assert.NoError(t, err)
	})

	t.Run("all valid tool names pass", func(t *testing.T) {
		err := ValidateEnabledTools(ValidToolNames)
		assert.NoError(t, err)
	})

	t.Run("single valid tool passes", func(t *testing.T) {
		err := ValidateEnabledTools([]string{ToolListChannels})
		assert.NoError(t, err)
	})

	t.Run("single invalid tool fails", func(t *testing.T) {
		err := ValidateEnabledTools([]string{"invalid_tool"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid_tool")
		assert.Contains(t, err.Error(), "Valid tools are:")
	})

	t.Run("multiple invalid tools listed in error", func(t *testing.T) {
		err := ValidateEnabledTools([]string{"foo", "bar"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "foo")
		assert.Contains(t, err.Error(), "bar")
	})

	t.Run("mix of valid and invalid tools fails", func(t *testing.T) {
		err := ValidateEnabledTools([]string{ToolListChannels, "invalid_tool", ToolAddReaction})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid tool name(s): invalid_tool.")
	})

	t.Run("typo in tool name fails", func(t *testing.T) {
		err := ValidateEnabledTools([]string{"channel_list"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "channel_list")
	})
}

// Helper to set/unset env vars for tests
func setEnv(key, value string) func() {
	old := os.Getenv(key)
	os.Setenv(key, value)
	return func() {
		if old == "" {
			os.Unsetenv(key)
		} else {
			os.Setenv(key, old)
		}
	}
}

func TestShouldAddTool_WriteTool_AddMessage(t *testing.T) {
	t.Run("empty enabledTools and empty env var - not registered", func(t *testing.T) {
		cleanup := setEnv("SLACK_MCP_ADD_MESSAGE_TOOL", "")
		defer cleanup()

		result := shouldAddTool(ToolPostMessage, []string{}, "SLACK_MCP_ADD_MESSAGE_TOOL")
		assert.False(t, result, "write tool should NOT be registered when both enabledTools is empty and env var is not set")
	})

	t.Run("empty enabledTools and env var set to true - registered", func(t *testing.T) {
		cleanup := setEnv("SLACK_MCP_ADD_MESSAGE_TOOL", "true")
		defer cleanup()

		result := shouldAddTool(ToolPostMessage, []string{}, "SLACK_MCP_ADD_MESSAGE_TOOL")
		assert.True(t, result, "write tool should be registered when enabledTools is empty but env var is set")
	})

	t.Run("empty enabledTools and env var set to channel list - registered", func(t *testing.T) {
		cleanup := setEnv("SLACK_MCP_ADD_MESSAGE_TOOL", "C123,C456")
		defer cleanup()

		result := shouldAddTool(ToolPostMessage, []string{}, "SLACK_MCP_ADD_MESSAGE_TOOL")
		assert.True(t, result, "write tool should be registered when enabledTools is empty but env var has channel list")
	})

	t.Run("explicit enabledTools includes tool and empty env var - registered", func(t *testing.T) {
		cleanup := setEnv("SLACK_MCP_ADD_MESSAGE_TOOL", "")
		defer cleanup()

		result := shouldAddTool(ToolPostMessage, []string{ToolPostMessage}, "SLACK_MCP_ADD_MESSAGE_TOOL")
		assert.True(t, result, "write tool should be registered when explicitly in enabledTools even without env var")
	})

	t.Run("explicit enabledTools excludes tool - not registered even with env var", func(t *testing.T) {
		cleanup := setEnv("SLACK_MCP_ADD_MESSAGE_TOOL", "true")
		defer cleanup()

		result := shouldAddTool(ToolPostMessage, []string{ToolGetChannelMessages}, "SLACK_MCP_ADD_MESSAGE_TOOL")
		assert.False(t, result, "write tool should NOT be registered when not in explicit enabledTools list")
	})
}

func TestShouldAddTool_WriteTool_Reactions(t *testing.T) {
	t.Run("empty enabledTools and no env var - not registered", func(t *testing.T) {
		cleanup := setEnv("SLACK_MCP_REACTION_TOOL", "")
		defer cleanup()

		result := shouldAddTool(ToolAddReaction, []string{}, "SLACK_MCP_REACTION_TOOL")
		assert.False(t, result, "add_reaction should NOT be registered when env var is not set")

		result = shouldAddTool(ToolRemoveReaction, []string{}, "SLACK_MCP_REACTION_TOOL")
		assert.False(t, result, "remove_reaction should NOT be registered when env var is not set")
	})

	t.Run("empty enabledTools and env var set - registered", func(t *testing.T) {
		cleanup := setEnv("SLACK_MCP_REACTION_TOOL", "true")
		defer cleanup()

		result := shouldAddTool(ToolAddReaction, []string{}, "SLACK_MCP_REACTION_TOOL")
		assert.True(t, result, "add_reaction should be registered when env var is set")

		result = shouldAddTool(ToolRemoveReaction, []string{}, "SLACK_MCP_REACTION_TOOL")
		assert.True(t, result, "remove_reaction should be registered when env var is set")
	})

	t.Run("explicit enabledTools includes tool - registered without env var", func(t *testing.T) {
		cleanup := setEnv("SLACK_MCP_REACTION_TOOL", "")
		defer cleanup()

		result := shouldAddTool(ToolAddReaction, []string{ToolAddReaction}, "SLACK_MCP_REACTION_TOOL")
		assert.True(t, result, "add_reaction should be registered when explicitly in enabledTools")
	})
}

func TestShouldAddTool_WriteTool_File(t *testing.T) {
	t.Run("empty enabledTools and no env var - not registered", func(t *testing.T) {
		cleanup := setEnv("SLACK_MCP_FILE_TOOL", "")
		defer cleanup()

		result := shouldAddTool(ToolDownloadFile, []string{}, "SLACK_MCP_FILE_TOOL")
		assert.False(t, result, "download_file should NOT be registered when env var is not set")
	})

	t.Run("empty enabledTools and env var set - registered", func(t *testing.T) {
		cleanup := setEnv("SLACK_MCP_FILE_TOOL", "true")
		defer cleanup()

		result := shouldAddTool(ToolDownloadFile, []string{}, "SLACK_MCP_FILE_TOOL")
		assert.True(t, result, "download_file should be registered when env var is set")
	})

	t.Run("explicit enabledTools includes tool - registered without env var", func(t *testing.T) {
		cleanup := setEnv("SLACK_MCP_FILE_TOOL", "")
		defer cleanup()

		result := shouldAddTool(ToolDownloadFile, []string{ToolDownloadFile}, "SLACK_MCP_FILE_TOOL")
		assert.True(t, result, "download_file should be registered when explicitly in enabledTools")
	})
}

// setupMCPClientServer creates an MCP server with the given options and tool handler,
// wires up a client via stdio pipes, and returns the connected client.
func setupMCPClientServer(t *testing.T, opts []server.ServerOption, toolHandler server.ToolHandlerFunc) *client.Client {
	t.Helper()

	mcpSrv := server.NewMCPServer("test", "1.0.0", opts...)
	mcpSrv.AddTool(mcp.NewTool("test_tool",
		mcp.WithDescription("A test tool"),
	), toolHandler)

	serverReader, clientWriter := io.Pipe()
	clientReader, serverWriter := io.Pipe()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	stdioSrv := server.NewStdioServer(mcpSrv)
	go func() {
		_ = stdioSrv.Listen(ctx, serverReader, serverWriter)
	}()

	var logBuf bytes.Buffer
	tr := transport.NewIO(clientReader, clientWriter, io.NopCloser(&logBuf))
	err := tr.Start(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { tr.Close() })

	c := client.NewClient(tr)

	var initReq mcp.InitializeRequest
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	_, err = c.Initialize(ctx, initReq)
	require.NoError(t, err)

	return c
}

func TestIntegrationErrorRecoveryMiddleware(t *testing.T) {
	logger := zap.NewNop()

	t.Run("handler error is converted to isError tool result", func(t *testing.T) {
		c := setupMCPClientServer(t,
			[]server.ServerOption{server.WithToolHandlerMiddleware(buildErrorRecoveryMiddleware(logger))},
			func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return nil, fmt.Errorf("simulated tool error: invalid channel ID")
			},
		)

		var callReq mcp.CallToolRequest
		callReq.Params.Name = "test_tool"
		result, err := c.CallTool(context.Background(), callReq)

		require.NoError(t, err, "should not return a JSON-RPC error")
		require.NotNil(t, result)
		assert.True(t, result.IsError, "result should have isError=true")
		require.Len(t, result.Content, 1)
		textContent, ok := result.Content[0].(mcp.TextContent)
		require.True(t, ok, "content should be TextContent")
		assert.Contains(t, textContent.Text, "simulated tool error: invalid channel ID")
	})

	t.Run("without middleware handler error becomes JSON-RPC error", func(t *testing.T) {
		c := setupMCPClientServer(t,
			nil, // no error recovery middleware
			func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return nil, fmt.Errorf("simulated tool error: invalid channel ID")
			},
		)

		var callReq mcp.CallToolRequest
		callReq.Params.Name = "test_tool"
		result, err := c.CallTool(context.Background(), callReq)

		assert.Error(t, err, "should return a JSON-RPC error without middleware")
		assert.Nil(t, result)
	})

	t.Run("successful tool call passes through unchanged", func(t *testing.T) {
		c := setupMCPClientServer(t,
			[]server.ServerOption{server.WithToolHandlerMiddleware(buildErrorRecoveryMiddleware(logger))},
			func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return mcp.NewToolResultText("all good"), nil
			},
		)

		var callReq mcp.CallToolRequest
		callReq.Params.Name = "test_tool"
		result, err := c.CallTool(context.Background(), callReq)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError, "successful result should not have isError=true")
		require.Len(t, result.Content, 1)
		textContent, ok := result.Content[0].(mcp.TextContent)
		require.True(t, ok)
		assert.Equal(t, "all good", textContent.Text)
	})
}

func TestShouldAddTool_Matrix(t *testing.T) {
	tests := []struct {
		name         string
		enabledTools []string
		envVarValue  string
		expected     bool
	}{
		{
			name:         "empty ENABLED_TOOLS + empty env var = NOT registered",
			enabledTools: []string{},
			envVarValue:  "",
			expected:     false,
		},
		{
			name:         "empty ENABLED_TOOLS + env var=true = registered",
			enabledTools: []string{},
			envVarValue:  "true",
			expected:     true,
		},
		{
			name:         "empty ENABLED_TOOLS + env var=channel list = registered",
			enabledTools: []string{},
			envVarValue:  "C123,C456",
			expected:     true,
		},
		{
			name:         "includes tool + empty env var = registered",
			enabledTools: []string{ToolPostMessage},
			envVarValue:  "",
			expected:     true,
		},
		{
			name:         "includes tool + env var=list = registered",
			enabledTools: []string{ToolPostMessage},
			envVarValue:  "C123",
			expected:     true,
		},
		{
			name:         "excludes tool + empty env var = NOT registered",
			enabledTools: []string{ToolGetChannelMessages},
			envVarValue:  "",
			expected:     false,
		},
		{
			name:         "excludes tool + env var=true = NOT registered",
			enabledTools: []string{ToolGetChannelMessages},
			envVarValue:  "true",
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := setEnv("SLACK_MCP_ADD_MESSAGE_TOOL", tt.envVarValue)
			defer cleanup()

			result := shouldAddTool(ToolPostMessage, tt.enabledTools, "SLACK_MCP_ADD_MESSAGE_TOOL")
			assert.Equal(t, tt.expected, result)
		})
	}
}