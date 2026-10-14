package v115open

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultNativeLoginApp = "ios"

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
	if loginApp == "" { loginApp = defaultNativeLoginApp }
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
	loginApp := token.LoginApp; if loginApp == "" { loginApp = defaultNativeLoginApp }
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
