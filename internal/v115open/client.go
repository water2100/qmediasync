package v115open

import (
	"Q115-STRM/internal/helpers"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"resty.dev/v3"
)

// OpenClient HTTP客户端
type OpenClient struct {
	AppId           string // 应用ID
	AccountId       uint   // 账号ID
	client          *resty.Client
	AccessToken     string // 访问令牌
	RefreshTokenStr string // 刷新令牌
	Cookie          string // 原生二维码登录会话
	AuthMode        string // 认证方式
}

// 全局HTTP客户端实例
var cachedClients map[string]*OpenClient = make(map[string]*OpenClient, 0)
var cachedClientsMutex sync.RWMutex

func UpdateToken(accountId uint, cookie string, refreshToken string) {
	for _, client := range cachedClients {
		if client.AccountId == accountId {
			client.SetCredentials(cookie, refreshToken, "115_cookie")
		}
	}
}

// NewHttpClient 创建新的HTTP客户端
func GetClient(accountId uint, appId string, credential string, refreshToken string, authMode string) *OpenClient {
	cachedClientsMutex.RLock()
	defer cachedClientsMutex.RUnlock()
	clientKey := fmt.Sprintf("%d", accountId)
	if client, exists := cachedClients[clientKey]; exists {
		client.SetCredentials(credential, refreshToken, authMode)
		return client
	}

	client := resty.New()
	openClient := &OpenClient{
		client:    client,
		AppId:     appId,
		AccountId: accountId,
	}
	openClient.SetCredentials(credential, refreshToken, authMode)
	cachedClients[clientKey] = openClient
	return openClient
}

func (c *OpenClient) SetCredentials(credential string, refreshToken string, authMode string) {
	c.AuthMode = authMode
	c.Cookie = credential
	c.AccessToken = ""
	c.RefreshTokenStr = refreshToken
}

// doRequest 带重试的请求方法（使用全局队列）
func (c *OpenClient) doRequest(url string, req *resty.Request, options *RequestConfig) (*resty.Response, *RespBase[json.RawMessage], error) {
	// 设置超时时间
	req.SetTimeout(options.Timeout)
	// 设置默认头
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", DEFAULTUA)
	}

	var lastErr error
	for attempt := 0; attempt <= options.MaxRetries; attempt++ {
		// 使用全局队列执行器处理请求
		executor := GetGlobalExecutor()
		respChan := make(chan *RequestResponse, 1)

		queuedReq := &QueuedRequest{
			URL:             url,
			Method:          req.Method,
			Request:         req,
			BypassRateLimit: options.BypassRateLimit,
			ResponseChan:    respChan,
			CreatedAt:       time.Now(),
			Ctx:             context.Background(),
		}

		// 将请求加入队列
		executor.EnqueueRequest(queuedReq)

		// 等待响应
		queueResp := <-respChan

		if queueResp.Error == nil && queueResp.RespData != nil {
			// 请求成功，转换为RespBase格式
			respBase := &RespBase[json.RawMessage]{
				State:   0,
				Code:    queueResp.RespData.Code,
				Message: queueResp.RespData.Message,
				Data:    queueResp.RespData.Data,
			}
			if queueResp.RespData.State {
				respBase.State = 1
			}
			return queueResp.Response, respBase, nil
		}

		lastErr = queueResp.Error

		// Token相关错误不重试
		if queueResp.RespData != nil {
			switch queueResp.RespData.Code {
			case REFRESH_TOKEN_INVALID:
				// 转换为RespBase格式返回
				respBase := &RespBase[json.RawMessage]{
					State:   0,
					Code:    queueResp.RespData.Code,
					Message: queueResp.RespData.Message,
					Data:    queueResp.RespData.Data,
				}
				if queueResp.RespData.State {
					respBase.State = 1
				}
				return queueResp.Response, respBase, lastErr
			}
		}

		// 如果是限流错误，不重试
		if queueResp.IsThrottled {
			helpers.V115Log.Warn("检测到限流，停止重试")
			if queueResp.RespData != nil {
				respBase := &RespBase[json.RawMessage]{
					State:   0,
					Code:    queueResp.RespData.Code,
					Message: queueResp.RespData.Message,
					Data:    queueResp.RespData.Data,
				}
				if queueResp.RespData.State {
					respBase.State = 1
				}
				return queueResp.Response, respBase, lastErr
			}
			return queueResp.Response, nil, lastErr
		}

		// 其他错误开始重试
		if attempt < options.MaxRetries && lastErr != nil {
			helpers.V115Log.Warnf("%s %s 请求失败:%+v", req.Method, url, lastErr)
			helpers.V115Log.Warnf("%s %s 请求失败，%+v秒后重试 (第%d次尝试)", req.Method, url, options.RetryDelay.Seconds(), attempt+1)
			time.Sleep(options.RetryDelay)
		}
	}
	return nil, nil, lastErr
}

// doAuthRequest 带重试的认证请求方法（使用全局队列）
func (c *OpenClient) doAuthRequest(ctx context.Context, url string, req *resty.Request, options *RequestConfig, respData any) (*resty.Response, []byte, error) {
	if c.Cookie == "" {
		return nil, nil, fmt.Errorf("115账号需要重新扫码登录")
	}
	req.SetTimeout(options.Timeout)
	// 设置默认头
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", DEFAULTUA)
	}
	req.SetHeader("Cookie", c.Cookie).SetDoNotParseResponse(true)

	var lastErr error
	var lastRespBytes []byte
	for attempt := 0; attempt <= options.MaxRetries; attempt++ {
		// 使用全局队列执行器处理请求
		executor := GetGlobalExecutor()
		respChan := make(chan *RequestResponse, 1)

		queuedReq := &QueuedRequest{
			URL:             url,
			Method:          req.Method,
			Request:         req,
			BypassRateLimit: options.BypassRateLimit,
			ResponseChan:    respChan,
			CreatedAt:       time.Now(),
			Ctx:             ctx,
		}

		// 将请求加入队列
		executor.EnqueueRequest(queuedReq)

		// 等待响应
		queueResp := <-respChan
		if c.isCookieInvalid(queueResp) {
			c.markCookieInvalid("115登录状态已失效，请重新扫码登录")
			return queueResp.Response, queueResp.RespBytes, fmt.Errorf("115账号需要重新扫码登录")
		}

		if queueResp.Error == nil && queueResp.RespData != nil {
			// 请求成功
			if respData != nil && queueResp.RespData.State {
				// 解包响应数据
				if unmarshalErr := json.Unmarshal(queueResp.RespData.Data, respData); unmarshalErr != nil {
					return queueResp.Response, queueResp.RespBytes, fmt.Errorf("解析115响应数据失败: %w", unmarshalErr)
				}
			}
			// 响应可能含有文件名、路径等私有数据；日志只保留请求结果摘要。
			helpers.V115Log.Debugf("115请求成功: %s %s (code=%d)", req.Method, url, queueResp.RespData.Code)
			return queueResp.Response, queueResp.RespBytes, nil
		}

		lastErr = queueResp.Error

		// 如果是限流错误，不重试
		if queueResp.IsThrottled {
			helpers.V115Log.Warn("检测到限流，等待1分钟后重试")
			// 等待1分钟后重试
			time.Sleep(1 * time.Minute)
			continue
		}

		// 其他错误开始重试
		if attempt < options.MaxRetries && lastErr != nil {
			helpers.V115Log.Warnf("%s %s 请求失败:%+v", req.Method, url, lastErr)
			helpers.V115Log.Warnf("%s %s 请求失败，%+v秒后重试 (第%d次尝试)", req.Method, url, options.RetryDelay.Seconds(), attempt+1)
			time.Sleep(options.RetryDelay)
		}
		lastRespBytes = queueResp.RespBytes
	}
	return nil, lastRespBytes, lastErr
}

func (c *OpenClient) isCookieInvalid(response *RequestResponse) bool {
	if response.Response != nil && (response.Response.StatusCode() == 401 || response.Response.StatusCode() == 403) {
		return true
	}
	if response.RespData == nil {
		return false
	}
	if response.RespData.Code == 401 || response.RespData.Code == 403 || response.RespData.Code == ACCESS_TOKEN_AUTH_FAIL || response.RespData.Code == ACCESS_AUTH_INVALID || response.RespData.Code == ACCESS_TOKEN_EXPIRY_CODE {
		return true
	}
	message := strings.ToLower(response.RespData.Message)
	return strings.Contains(message, "未登录") || strings.Contains(message, "登录失效") || strings.Contains(message, "cookie") || strings.Contains(message, "login")
}

func (c *OpenClient) markCookieInvalid(reason string) {
	c.Cookie = ""
	helpers.PublishSync(helpers.V115TokenInValidEvent, map[string]any{"account_id": c.AccountId, "reason": reason})
}

