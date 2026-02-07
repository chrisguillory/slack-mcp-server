package handler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gocarina/gocsv"
	"github.com/korotovsky/slack-mcp-server/pkg/provider"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/slack-go/slack"
	"go.uber.org/zap"
)

const (
	maxFileSize        = 50 * 1024 * 1024 // 50MB
	defaultDownloadDir = "./downloads"
)

type FileHandler struct {
	apiProvider     *provider.ApiProvider
	logger          *zap.Logger
	downloadDir     string // Container session path (e.g., /app/downloads/slack-mcp-XXXX)
	baseDownloadDir string // Container base path (e.g., /app/downloads)
	hostDownloadDir string // Host base path (e.g., /Users/chris/slack-mcp-server/downloads)
}

type FileDownloadResult struct {
	FileID    string `json:"file_id" csv:"file_id"`
	Name      string `json:"name" csv:"name"`
	Type      string `json:"type" csv:"type"`
	Size      int    `json:"size" csv:"size"`
	LocalPath string `json:"local_path" csv:"local_path"`
	Status    string `json:"status" csv:"status"`
}

func NewFileHandler(apiProvider *provider.ApiProvider, logger *zap.Logger, downloadDir string) *FileHandler {
	// Determine base download directory
	baseDir := downloadDir
	if baseDir == "" {
		// Use OS default temp location (like Granola MCP)
		// On macOS this uses $TMPDIR, typically /var/folders/.../T/
		baseDir = os.TempDir()
	}

	// Always create a unique subdirectory inside the base directory
	// This ensures isolation between multiple container instances and sessions
	// Creates something like: /app/downloads/slack-mcp-XXXXXX/ or /tmp/slack-mcp-XXXXXX/
	sessionDir, err := os.MkdirTemp(baseDir, "slack-mcp-*")
	if err != nil {
		logger.Error("Failed to create unique session directory, using base dir",
			zap.String("base", baseDir),
			zap.Error(err))
		sessionDir = baseDir
	} else {
		logger.Info("Created unique session download directory",
			zap.String("path", sessionDir))
	}

	// Read host download path for Docker volume mapping
	// If set, container paths will be translated to host paths
	hostDownloadDir := os.Getenv("SLACK_MCP_HOST_DOWNLOADS_PATH")
	if hostDownloadDir != "" {
		logger.Info("Docker volume mapping enabled",
			zap.String("container_base", baseDir),
			zap.String("container_session", sessionDir),
			zap.String("host_base", hostDownloadDir))
	}

	return &FileHandler{
		apiProvider:     apiProvider,
		logger:          logger,
		downloadDir:     sessionDir,
		baseDownloadDir: baseDir,
		hostDownloadDir: hostDownloadDir,
	}
}

func (fh *FileHandler) DownloadFileHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	fh.logger.Debug("DownloadFileHandler called", zap.Any("params", request.Params))

	// Parse file_ids parameter - handle both array and single string
	fileIDsParam := request.GetString("file_ids", "")
	var fileIDs []string

	if fileIDsParam != "" {
		// Single file ID provided as string
		fileIDs = []string{fileIDsParam}
	} else {
		// Try to get as array (this won't work with current mcp-go API, but keeping for future)
		return mcp.NewToolResultError("file_ids parameter is required (provide as comma-separated string)"), nil
	}

	// Support comma-separated file IDs
	if len(fileIDs) == 1 && strings.Contains(fileIDs[0], ",") {
		fileIDs = strings.Split(fileIDs[0], ",")
		for i, id := range fileIDs {
			fileIDs[i] = strings.TrimSpace(id)
		}
	}

	if len(fileIDs) == 0 {
		return mcp.NewToolResultError("At least one file_id must be provided"), nil
	}

	fh.logger.Debug("Processing file downloads", zap.Int("count", len(fileIDs)))

	// Get optional output_dir parameter
	outputDir := request.GetString("output_dir", fh.downloadDir)

	// In Docker mode, reject output_dir if it's a path — the caller's filesystem
	// is the host, not the container, so custom paths won't be accessible.
	if fh.hostDownloadDir != "" && outputDir != fh.downloadDir {
		return mcp.NewToolResultError("output_dir is not supported in Docker mode. Files are saved to the volume-mounted download directory and the host path is returned in local_path."), nil
	}

	// Make path absolute if it's relative
	if !filepath.IsAbs(outputDir) {
		absPath, err := filepath.Abs(outputDir)
		if err != nil {
			fh.logger.Error("Failed to convert path to absolute", zap.String("dir", outputDir), zap.Error(err))
			return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve path: %v", err)), nil
		}
		outputDir = absPath
	}

	// Ensure download directory exists
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fh.logger.Error("Failed to create download directory", zap.String("dir", outputDir), zap.Error(err))
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create download directory: %v", err)), nil
	}

	// Download each file using Slack API (handles authentication automatically)
	var results []FileDownloadResult
	for _, fileID := range fileIDs {
		result := fh.downloadFile(ctx, fileID, outputDir)
		results = append(results, result)
	}

	// Marshal results to CSV
	csvBytes, err := gocsv.MarshalBytes(&results)
	if err != nil {
		fh.logger.Error("Failed to marshal results to CSV", zap.Error(err))
		return mcp.NewToolResultError(fmt.Sprintf("Failed to format results: %v", err)), nil
	}

	return mcp.NewToolResultText(string(csvBytes)), nil
}

func (fh *FileHandler) downloadFile(ctx context.Context, fileID string, outputDir string) FileDownloadResult {
	result := FileDownloadResult{
		FileID: fileID,
		Status: "pending",
	}

	// Parse file metadata from fileID format (if it contains metadata)
	// For now, we need to get the file URL - we'll need to fetch file info from Slack API
	// or reconstruct the URL from the fileID

	// Get file info from Slack API to get the URL and metadata
	fileInfo, _, _, err := fh.apiProvider.Slack().GetFileInfoContext(ctx, fileID, 0, 0)
	if err != nil {
		fh.logger.Error("Failed to get file info from Slack", zap.String("file_id", fileID), zap.Error(err))
		result.Status = fmt.Sprintf("error: failed to get file info: %v", err)
		return result
	}

	result.Name = fileInfo.Name
	result.Type = fileInfo.Filetype
	result.Size = fileInfo.Size

	// Check file size limit
	if fileInfo.Size > maxFileSize {
		fh.logger.Warn("File exceeds size limit",
			zap.String("file_id", fileID),
			zap.Int("size", fileInfo.Size),
			zap.Int("max_size", maxFileSize))
		result.Status = fmt.Sprintf("error: file size %d bytes exceeds limit of %d bytes", fileInfo.Size, maxFileSize)
		return result
	}

	// Use URLPrivate for download
	fileURL := fileInfo.URLPrivate
	if fileURL == "" {
		fh.logger.Error("File has no private URL", zap.String("file_id", fileID))
		result.Status = "error: file has no download URL"
		return result
	}

	// Sanitize filename and make it unique by prepending file ID
	// This prevents conflicts when multiple files have the same name
	safeName := sanitizeFilename(fileInfo.Name)
	ext := filepath.Ext(safeName)
	nameWithoutExt := strings.TrimSuffix(safeName, ext)
	uniqueName := fmt.Sprintf("%s-%s%s", fileID, nameWithoutExt, ext)
	localPath := filepath.Join(outputDir, uniqueName)

	// Create output file
	outFile, err := os.Create(localPath)
	if err != nil {
		fh.logger.Error("Failed to create output file", zap.String("path", localPath), zap.Error(err))
		result.Status = fmt.Sprintf("error: failed to create file: %v", err)
		return result
	}
	defer outFile.Close()

	// Use Slack API's GetFile method which handles authentication automatically
	err = fh.apiProvider.Slack().GetFileContext(ctx, fileURL, outFile)
	if err != nil {
		fh.logger.Error("Failed to download file", zap.String("url", fileURL), zap.Error(err))
		result.Status = fmt.Sprintf("error: download failed: %v", err)
		return result
	}

	// Get file size
	fileInfo2, err := outFile.Stat()
	if err != nil {
		fh.logger.Warn("Failed to get file size", zap.Error(err))
	}

	fh.logger.Info("File downloaded successfully",
		zap.String("file_id", fileID),
		zap.String("name", fileInfo.Name),
		zap.String("path", localPath),
		zap.Int64("bytes", fileInfo2.Size()))

	// Translate container path to host path if Docker volume mapping is enabled
	result.LocalPath = fh.translatePath(localPath)
	result.Status = "success"
	return result
}

// translatePath converts container paths to host paths for Docker volume mapping.
// If SLACK_MCP_HOST_DOWNLOADS_PATH is set, replaces the container base path with the host base path.
// Example: /app/downloads/slack-mcp-XXXX/file.png → /Users/chris/slack-mcp-server/downloads/slack-mcp-XXXX/file.png
// This preserves the unique session subdirectory name in the host path
func (fh *FileHandler) translatePath(containerPath string) string {
	if fh.hostDownloadDir == "" {
		// No translation needed (native execution)
		return containerPath
	}

	// Replace container base path with host base path, preserving subdirectories
	if strings.HasPrefix(containerPath, fh.baseDownloadDir) {
		// Get relative path from base (includes session subdir and filename)
		relativePath := strings.TrimPrefix(containerPath, fh.baseDownloadDir)
		relativePath = strings.TrimPrefix(relativePath, string(filepath.Separator))

		// Join with host base path
		hostPath := filepath.Join(fh.hostDownloadDir, relativePath)
		fh.logger.Debug("Translated container path to host path",
			zap.String("container", containerPath),
			zap.String("host", hostPath))
		return hostPath
	}

	// Path doesn't start with base download dir, return as-is
	return containerPath
}

// sanitizeFilename removes or replaces characters that are problematic in filenames
func sanitizeFilename(name string) string {
	// Replace non-ASCII characters (e.g., Unicode non-breaking spaces in Slack filenames)
	var asciiName strings.Builder
	for _, r := range name {
		if r > 127 {
			asciiName.WriteRune('_')
		} else {
			asciiName.WriteRune(r)
		}
	}
	name = asciiName.String()

	// Replace path separators and other problematic characters
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	safe := replacer.Replace(name)

	// Limit filename length (leave room for extension)
	if len(safe) > 200 {
		// Try to preserve extension
		ext := filepath.Ext(safe)
		base := safe[:len(safe)-len(ext)]
		if len(base) > 200-len(ext) {
			base = base[:200-len(ext)]
		}
		safe = base + ext
	}

	return safe
}

// Cleanup removes the temporary download directory and all its contents.
// Should be called when the server exits to clean up downloaded files (like Granola MCP).
func (fh *FileHandler) Cleanup() {
	fh.logger.Info("FileHandler.Cleanup() called")

	if fh.downloadDir == "" {
		fh.logger.Warn("Cleanup skipped: downloadDir is empty")
		return
	}

	fh.logger.Info("Cleanup checking directory",
		zap.String("path", fh.downloadDir),
		zap.Bool("contains_slack_mcp", strings.Contains(fh.downloadDir, "slack-mcp-")))

	// Only clean up if it's a temp directory we created (contains "slack-mcp-")
	if !strings.Contains(fh.downloadDir, "slack-mcp-") {
		fh.logger.Info("Skipping cleanup of non-temp directory",
			zap.String("path", fh.downloadDir))
		return
	}

	fh.logger.Info("Starting cleanup of temporary download directory",
		zap.String("path", fh.downloadDir))

	err := os.RemoveAll(fh.downloadDir)
	if err != nil {
		fh.logger.Error("Failed to cleanup temp directory",
			zap.String("path", fh.downloadDir),
			zap.Error(err))
	} else {
		fh.logger.Info("Temp directory cleaned up successfully",
			zap.String("path", fh.downloadDir))
	}
}

// --- get_file_info ---

type FileInfoResult struct {
	ID              string `csv:"id"`
	Name            string `csv:"name"`
	Title           string `csv:"title"`
	Filetype        string `csv:"filetype"`
	Mimetype        string `csv:"mimetype"`
	Size            int    `csv:"size"`
	User            string `csv:"user"`
	Created         string `csv:"created"`
	IsPublic        string `csv:"is_public"`
	PublicURLShared string `csv:"public_url_shared"`
	Permalink       string `csv:"permalink"`
	PermalinkPublic string `csv:"permalink_public"`
	URLPrivate      string `csv:"url_private"`
	Channels        string `csv:"channels"`
	Groups          string `csv:"groups"`
	IMs             string `csv:"ims"`
	Shares          string `csv:"shares"`
	CommentsCount   int    `csv:"comments_count"`
	IsExternal      string `csv:"is_external"`
}

func (fh *FileHandler) GetFileInfoHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	fh.logger.Debug("GetFileInfoHandler called", zap.Any("params", request.Params))

	fileID := request.GetString("file_id", "")
	if fileID == "" {
		return mcp.NewToolResultError("file_id parameter is required"), nil
	}

	fileInfo, _, _, err := fh.apiProvider.Slack().GetFileInfoContext(ctx, fileID, 0, 0)
	if err != nil {
		fh.logger.Error("Failed to get file info from Slack",
			zap.String("file_id", fileID), zap.Error(err))
		return mcp.NewToolResultErrorFromErr("Failed to get file info", err), nil
	}

	result := []FileInfoResult{{
		ID:              fileInfo.ID,
		Name:            fileInfo.Name,
		Title:           fileInfo.Title,
		Filetype:        fileInfo.Filetype,
		Mimetype:        fileInfo.Mimetype,
		Size:            fileInfo.Size,
		User:            fileInfo.User,
		Created:         time.Unix(int64(fileInfo.Created), 0).Format(time.RFC3339),
		IsPublic:        fmt.Sprintf("%t", fileInfo.IsPublic),
		PublicURLShared: fmt.Sprintf("%t", fileInfo.PublicURLShared),
		Permalink:       fileInfo.Permalink,
		PermalinkPublic: fileInfo.PermalinkPublic,
		URLPrivate:      fileInfo.URLPrivate,
		Channels:        strings.Join(fileInfo.Channels, ","),
		Groups:          strings.Join(fileInfo.Groups, ","),
		IMs:             strings.Join(fileInfo.IMs, ","),
		Shares:          flattenShares(fileInfo.Shares),
		CommentsCount:   fileInfo.CommentsCount,
		IsExternal:      fmt.Sprintf("%t", fileInfo.IsExternal),
	}}

	csvBytes, err := gocsv.MarshalBytes(&result)
	if err != nil {
		fh.logger.Error("Failed to marshal file info to CSV", zap.Error(err))
		return mcp.NewToolResultErrorFromErr("Failed to format file info", err), nil
	}

	return mcp.NewToolResultText(string(csvBytes)), nil
}

// flattenShares converts the nested Share struct into "type:channel_id:ts|..." format
func flattenShares(shares slack.Share) string {
	var parts []string
	for channelID, infos := range shares.Public {
		for _, info := range infos {
			parts = append(parts, fmt.Sprintf("public:%s:%s", channelID, info.Ts))
		}
	}
	for channelID, infos := range shares.Private {
		for _, info := range infos {
			parts = append(parts, fmt.Sprintf("private:%s:%s", channelID, info.Ts))
		}
	}
	return strings.Join(parts, "|")
}

// --- upload_file ---

type FileUploadResult struct {
	FileID  string `csv:"file_id"`
	Title   string `csv:"title"`
	Channel string `csv:"channel"`
	Status  string `csv:"status"`
}

func (fh *FileHandler) UploadFileHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	fh.logger.Debug("UploadFileHandler called", zap.Any("params", request.Params))

	filePath := request.GetString("file_path", "")
	if filePath == "" {
		return mcp.NewToolResultError("file_path parameter is required"), nil
	}

	channelID := request.GetString("channel_id", "")
	if channelID == "" {
		return mcp.NewToolResultError("channel_id parameter is required"), nil
	}

	// Resolve channel names (#channel or @user) to IDs
	if strings.HasPrefix(channelID, "#") || strings.HasPrefix(channelID, "@") {
		channelsMaps := fh.apiProvider.ProvideChannelsMaps()
		chn, ok := channelsMaps.ChannelsInv[channelID]
		if !ok {
			fh.logger.Error("Channel not found", zap.String("channel", channelID))
			return mcp.NewToolResultError(fmt.Sprintf("channel %q not found", channelID)), nil
		}
		channelID = channelsMaps.Channels[chn].ID
	}

	title := request.GetString("title", "")
	initialComment := request.GetString("initial_comment", "")
	threadTs := request.GetString("thread_ts", "")

	// Docker mode: if the given path doesn't exist locally, try translating
	// host path back to container path (reverse of download's translatePath)
	actualFilePath := filePath
	if _, err := os.Stat(actualFilePath); err != nil && fh.hostDownloadDir != "" && strings.HasPrefix(filePath, fh.hostDownloadDir) {
		relativePath := strings.TrimPrefix(filePath, fh.hostDownloadDir)
		relativePath = strings.TrimPrefix(relativePath, string(filepath.Separator))
		translated := filepath.Join(fh.baseDownloadDir, relativePath)
		fh.logger.Debug("File not found at given path, trying container path",
			zap.String("host", filePath),
			zap.String("container", translated))
		actualFilePath = translated
	}

	fileHandle, err := os.Open(actualFilePath)
	if err != nil {
		fh.logger.Error("Failed to open file", zap.String("path", actualFilePath), zap.Error(err))
		return mcp.NewToolResultError(fmt.Sprintf("Failed to open file: %v", err)), nil
	}
	defer fileHandle.Close()

	fileStat, err := fileHandle.Stat()
	if err != nil {
		fh.logger.Error("Failed to stat file", zap.String("path", actualFilePath), zap.Error(err))
		return mcp.NewToolResultError(fmt.Sprintf("Failed to read file info: %v", err)), nil
	}

	if fileStat.IsDir() {
		return mcp.NewToolResultError("file_path must point to a file, not a directory"), nil
	}

	if fileStat.Size() > int64(maxFileSize) {
		return mcp.NewToolResultError(fmt.Sprintf("file size %d bytes exceeds limit of %d bytes", fileStat.Size(), maxFileSize)), nil
	}

	if fileStat.Size() == 0 {
		return mcp.NewToolResultError("file is empty (0 bytes)"), nil
	}

	filename := filepath.Base(actualFilePath)
	if title == "" {
		title = filename
	}

	fh.logger.Debug("Uploading file to Slack",
		zap.String("file", actualFilePath),
		zap.String("channel", channelID),
		zap.String("title", title),
		zap.Int64("size", fileStat.Size()))

	params := slack.UploadFileV2Parameters{
		Reader:          fileHandle,
		Filename:        filename,
		FileSize:        int(fileStat.Size()),
		Title:           title,
		InitialComment:  initialComment,
		Channel:         channelID,
		ThreadTimestamp: threadTs,
	}

	fileSummary, err := fh.apiProvider.Slack().UploadFileV2Context(ctx, params)
	if err != nil {
		fh.logger.Error("Failed to upload file to Slack", zap.Error(err))
		return mcp.NewToolResultErrorFromErr("Failed to upload file", err), nil
	}

	fh.logger.Info("File uploaded successfully",
		zap.String("file_id", fileSummary.ID),
		zap.String("title", fileSummary.Title),
		zap.String("channel", channelID))

	result := []FileUploadResult{{
		FileID:  fileSummary.ID,
		Title:   fileSummary.Title,
		Channel: channelID,
		Status:  "uploaded",
	}}

	csvBytes, err := gocsv.MarshalBytes(&result)
	if err != nil {
		fh.logger.Error("Failed to marshal upload result to CSV", zap.Error(err))
		return mcp.NewToolResultErrorFromErr("Failed to format upload result", err), nil
	}

	return mcp.NewToolResultText(string(csvBytes)), nil
}

// --- make_file_public ---

type FilePublicResult struct {
	FileID          string `csv:"file_id"`
	PermalinkPublic string `csv:"permalink_public"`
}

func (fh *FileHandler) MakeFilePublicHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	fh.logger.Debug("MakeFilePublicHandler called", zap.Any("params", request.Params))

	fileID := request.GetString("file_id", "")
	if fileID == "" {
		return mcp.NewToolResultError("file_id parameter is required"), nil
	}

	fileInfo, _, _, err := fh.apiProvider.Slack().ShareFilePublicURLContext(ctx, fileID)
	if err != nil {
		fh.logger.Error("Failed to make file public",
			zap.String("file_id", fileID), zap.Error(err))
		return mcp.NewToolResultErrorFromErr("Failed to make file public", err), nil
	}

	fh.logger.Info("File made public",
		zap.String("file_id", fileInfo.ID),
		zap.String("permalink_public", fileInfo.PermalinkPublic))

	result := []FilePublicResult{{
		FileID:          fileInfo.ID,
		PermalinkPublic: fileInfo.PermalinkPublic,
	}}

	csvBytes, err := gocsv.MarshalBytes(&result)
	if err != nil {
		fh.logger.Error("Failed to marshal result to CSV", zap.Error(err))
		return mcp.NewToolResultErrorFromErr("Failed to format result", err), nil
	}

	return mcp.NewToolResultText(string(csvBytes)), nil
}
