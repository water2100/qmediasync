package v115open

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// NativeLoginApp 是 115 服务端识别的登录会话槽位。它们不是泛化的
// 浏览器/操作系统名称；错误地默认其中一个会挤掉用户该槽位上的既有设备。
type NativeLoginApp struct {
	App         string `json:"app"`
	Description string `json:"description"`
}

var NativeLoginApps = []NativeLoginApp{
	{App: "web", Description: "115生活_网页端"},
	{App: "ios", Description: "115生活_苹果端"},
	{App: "115ios", Description: "115_苹果端"},
	{App: "bandroid", Description: "未知: android"},
	{App: "115ipad", Description: "115_苹果平板端"},
	{App: "tv", Description: "115生活_安卓电视端"},
	{App: "apple_tv", Description: "115生活_苹果电视端"},
	{App: "qios", Description: "115管理_苹果端"},
	{App: "os_windows", Description: "115生活_Windows端"},
	{App: "os_linux", Description: "115生活_Linux端"},
	{App: "wechatmini", Description: "115生活_微信小程序端"},
	{App: "alipaymini", Description: "115生活_支付宝小程序"},
	{App: "harmony", Description: "115_鸿蒙端"},
}

func IsNativeLoginApp(app string) bool {
	for _, candidate := range NativeLoginApps {
		if candidate.App == app {
			return true
		}
	}
	return false
}

type NativeQrStatus int

const (
	NativeQrStatusWaiting NativeQrStatus = iota
	NativeQrStatusScanned
	NativeQrStatusConfirmed
	NativeQrStatusExpired  NativeQrStatus = -1
	NativeQrStatusCanceled NativeQrStatus = -2
)

type NativeQrToken struct {
	UID string `json:"uid"`
	Time string `json:"time"`
	Sign string `json:"sign"`
	QrCode string `json:"qrcode"`
	LoginApp string `json:"login_app"`
}

type NativeQrLogin struct { Cookie string; UserID string; Name string }
type nativeQrResponse struct { State any `json:"state"`; Message string `json:"message"`; Data json.RawMessage `json:"data"` }

func (c *OpenClient) GetNativeQrCode(loginApp string) (*NativeQrToken, error) {
	resp, err := c.client.R().SetTimeout(15 * time.Second).Get("https://qrcodeapi.115.com/api/1.0/web/1.0/token/")
	if err != nil { return nil, fmt.Errorf("获取115二维码失败: %w", err) }
	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 { return nil, fmt.Errorf("获取115二维码失败: HTTP %d", resp.StatusCode()) }
	token, err := decodeNativeQrToken(resp.Bytes()); if err != nil { return nil, err }
	if loginApp == "" {
		return nil, fmt.Errorf("请选择115登录设备")
	}
	if !IsNativeLoginApp(loginApp) {
		return nil, fmt.Errorf("不支持的115登录设备: %s", loginApp)
	}
	token.LoginApp = loginApp
	token.QrCode = "https://qrcodeapi.115.com/api/1.0/mac/1.0/qrcode?uid=" + url.QueryEscape(token.UID)
	return token, nil
}

func (c *OpenClient) GetNativeQrStatus(token *NativeQrToken) (NativeQrStatus, error) {
	params := url.Values{"uid": {token.UID}, "time": {token.Time}, "sign": {token.Sign}}
	urls := []string{"https://qrcodeapi.115.com/get/status/?" + params.Encode(), "https://qrcodeapi.115.com/api/1.0/web/1.0/status/?" + params.Encode()}
	var lastErr error
	for _, endpoint := range urls {
		resp, err := c.client.R().SetTimeout(15 * time.Second).Get(endpoint)
		if err != nil { lastErr = err; continue }
		if resp.StatusCode() < 200 || resp.StatusCode() >= 300 { lastErr = fmt.Errorf("HTTP %d", resp.StatusCode()); continue }
		status, err := parseNativeQrStatus(resp.Bytes()); if err == nil { return status, nil }; lastErr = err
	}
	return NativeQrStatusExpired, fmt.Errorf("查询115二维码状态失败: %w", lastErr)
}

func (c *OpenClient) LoginWithNativeQrCode(token *NativeQrToken) (*NativeQrLogin, error) {
	loginApp := token.LoginApp
	if !IsNativeLoginApp(loginApp) {
		return nil, fmt.Errorf("不支持的115登录设备: %s", loginApp)
	}
	endpoint := "https://passportapi.115.com/app/1.0/" + url.PathEscape(loginApp) + "/1.0/login/qrcode/"
	resp, err := c.client.R().SetTimeout(20 * time.Second).SetFormData(map[string]string{"account": token.UID}).Post(endpoint)
	if err != nil { return nil, fmt.Errorf("提交115二维码登录失败: %w", err) }
	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 { return nil, fmt.Errorf("提交115二维码登录失败: HTTP %d", resp.StatusCode()) }
	var payload nativeQrResponse
	if err := json.Unmarshal(resp.Bytes(), &payload); err != nil { return nil, fmt.Errorf("解析115二维码登录响应失败: %w", err) }
	if !nativeStateOK(payload.State) { return nil, fmt.Errorf("115二维码登录失败: %s", payload.Message) }
	var data struct { Cookie map[string]any `json:"cookie"`; UserID any `json:"user_id"`; UserName string `json:"user_name"` }
	if err := json.Unmarshal(payload.Data, &data); err != nil { return nil, fmt.Errorf("解析115登录凭证失败: %w", err) }
	cookie, err := normalizeNativeCookie(data.Cookie); if err != nil { return nil, err }
	userID := valueToString(data.UserID)
	if userID == "" { userID = strings.Split(strings.TrimPrefix(strings.Split(cookie, ";")[0], "UID="), "_")[0] }
	return &NativeQrLogin{Cookie: cookie, UserID: userID, Name: data.UserName}, nil
}

func decodeNativeQrToken(body []byte) (*NativeQrToken, error) {
	var payload nativeQrResponse
	if err := json.Unmarshal(body, &payload); err != nil { return nil, fmt.Errorf("解析115二维码响应失败: %w", err) }
	if !nativeStateOK(payload.State) { return nil, fmt.Errorf("获取115二维码失败: %s", payload.Message) }
	var data struct { UID string `json:"uid"`; Time any `json:"time"`; Sign string `json:"sign"` }
	if err := json.Unmarshal(payload.Data, &data); err != nil { return nil, fmt.Errorf("解析115二维码数据失败: %w", err) }
	if data.UID == "" || valueToString(data.Time) == "" || data.Sign == "" { return nil, fmt.Errorf("115二维码数据不完整") }
	return &NativeQrToken{UID: data.UID, Time: valueToString(data.Time), Sign: data.Sign}, nil
}

func parseNativeQrStatus(body []byte) (NativeQrStatus, error) {
	var payload nativeQrResponse
	if err := json.Unmarshal(body, &payload); err != nil { return NativeQrStatusExpired, fmt.Errorf("解析115二维码状态失败: %w", err) }
	if !nativeStateOK(payload.State) { return NativeQrStatusExpired, fmt.Errorf("查询115二维码状态失败: %s", payload.Message) }
	var data struct { Status int `json:"status"` }
	if err := json.Unmarshal(payload.Data, &data); err != nil { return NativeQrStatusExpired, fmt.Errorf("解析115二维码状态数据失败: %w", err) }
	switch NativeQrStatus(data.Status) { case NativeQrStatusWaiting, NativeQrStatusScanned, NativeQrStatusConfirmed, NativeQrStatusExpired, NativeQrStatusCanceled: return NativeQrStatus(data.Status), nil }
	return NativeQrStatusExpired, fmt.Errorf("未知的115二维码状态: %d", data.Status)
}

func normalizeNativeCookie(values map[string]any) (string, error) {
	keys := []string{"UID", "CID", "SEID", "KID"}; parts := make([]string, 0, len(keys))
	for _, key := range keys { value := valueToString(values[key]); if value == "" { return "", fmt.Errorf("115登录响应缺少 %s Cookie", key) }; parts = append(parts, key+"="+value) }
	return strings.Join(parts, "; "), nil
}
func nativeStateOK(state any) bool { switch value := state.(type) { case bool: return value; case float64: return value != 0; case string: return value == "1" || strings.EqualFold(value, "true") }; return false }
func valueToString(value any) string { switch v := value.(type) { case nil: return ""; case string: return v; case float64: return strconv.FormatInt(int64(v), 10); case json.Number: return string(v); default: return fmt.Sprint(v) } }

