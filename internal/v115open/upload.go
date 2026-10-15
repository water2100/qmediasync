package v115open

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// The Cookie endpoints for large downloads and uploads use 115-specific
// encryption and signing. The pinned p115client helper handles that protocol;
// credentials are passed on stdin and never appear in process arguments.
type cookieBridgeRequest struct {
	Operation string `json:"operation"`
	Cookie    string `json:"cookie"`
	LoginApp  string `json:"login_app"`
	PickCode  string `json:"pickcode,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
	FilePath  string `json:"file_path,omitempty"`
	ParentID  string `json:"parent_id,omitempty"`
}

type cookieBridgeResponse struct {
	OK             bool   `json:"ok"`
	URL            string `json:"url,omitempty"`
	FileID         string `json:"file_id,omitempty"`
	RequiresCookie bool   `json:"requires_cookie,omitempty"`
	Error          string `json:"error,omitempty"`
}

func (c *OpenClient) runCookieBridge(ctx context.Context, request cookieBridgeRequest) (*cookieBridgeResponse, error) {
	if c.Cookie == "" {
		return nil, fmt.Errorf("115账号需要重新扫码登录")
	}
	request.Cookie = c.Cookie
	request.LoginApp = c.LoginApp
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("准备115 Cookie请求失败: %w", err)
	}
	python := os.Getenv("QMS_115_PYTHON")
	if python == "" {
		python = "/opt/qms-python/bin/python"
	}
	cmd := exec.CommandContext(ctx, python, "/app/scripts/115_cookie_bridge.py")
	cmd.Stdin = bytes.NewReader(payload)
	// Never collect stderr: dependencies may include request headers in tracebacks.
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("115 Cookie辅助程序执行失败")
	}
	var response cookieBridgeResponse
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return nil, fmt.Errorf("解析115 Cookie辅助程序响应失败")
	}
	if !response.OK {
		if strings.Contains(response.Error, "login") || strings.Contains(response.Error, "cookie") {
			c.markCookieInvalid("115登录状态已失效，请重新扫码登录")
			return nil, fmt.Errorf("115账号需要重新扫码登录")
		}
		return nil, fmt.Errorf("115 Cookie操作失败: %s", response.Error)
	}
	return &response, nil
}

func (c *OpenClient) GetDownloadUrl(ctx context.Context, pickCode string, userAgent string, bypassRateLimit bool) string {
	if pickCode == "" {
		return ""
	}
	response, err := c.runCookieBridge(ctx, cookieBridgeRequest{
		Operation: "download_url", PickCode: pickCode, UserAgent: userAgent,
	})
	if err != nil || response.URL == "" || response.RequiresCookie {
		return ""
	}
	return response.URL
}

type VideoPlayUrlData struct {
	VideoUrl []struct {
		Url string `json:"url"`
	} `json:"video_url"`
}

func (c *OpenClient) GetVideoPlayUrl(ctx context.Context, pickCode string, userAgent string) *VideoPlayUrlData {
	url := c.GetDownloadUrl(ctx, pickCode, userAgent, false)
	if url == "" {
		return nil
	}
	result := &VideoPlayUrlData{}
	result.VideoUrl = append(result.VideoUrl, struct {
		Url string `json:"url"`
	}{Url: url})
	return result
}

func (c *OpenClient) Upload(ctx context.Context, filePath string, parentFileId string, _ string, _ string) (string, error) {
	if filePath == "" || parentFileId == "" {
		return "", fmt.Errorf("115上传缺少本地文件或目标目录")
	}
	response, err := c.runCookieBridge(ctx, cookieBridgeRequest{
		Operation: "upload_file", FilePath: filePath, ParentID: parentFileId,
	})
	if err != nil {
		return "", err
	}
	if response.FileID == "" {
		return "", fmt.Errorf("115上传响应缺少文件ID")
	}
	return response.FileID, nil
}
