package v115open

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type UserInfoSpace struct {
	Size       int64  `json:"size"`
	SizeFormat string `json:"size_format"`
}
type UserInfo struct {
	UserId      json.Number `json:"user_id"`
	UserName    string      `json:"user_name"`
	UserFaceS   string      `json:"user_face_s"`
	UserFaceM   string      `json:"user_face_m"`
	UserFaceL   string      `json:"user_face_l"`
	RtSpaceInfo struct {
		AllTotal  UserInfoSpace `json:"all_total"`
		AllRemain UserInfoSpace `json:"all_remain"`
		AllUse    UserInfoSpace `json:"all_use"`
	} `json:"rt_space_info"`
	VipInfo struct {
		LevelName string `json:"level_name"`
		Expire    int64  `json:"expire"`
	} `json:"vip_info"`
} // 115用户信息，每次都通过接口请求，作为全局变量使用

// UserInfo 通过 Cookie 验证会话并从 UID Cookie 读取用户 ID。
// 原 /open/user/info 仅支持 OpenAPI AccessToken，不能用于原生二维码 Cookie 会话。
func (c *OpenClient) UserInfo() (*UserInfo, error) {
	if c.Cookie == "" {
		return nil, fmt.Errorf("115账号需要重新扫码登录")
	}
	// 以目录列表请求作为 Cookie 有效性校验；不记录 Cookie 内容。
	req := c.client.R().SetQueryParams(map[string]string{"aid": "1", "cid": "0", "limit": "1"}).SetMethod("GET")
	_, _, err := c.doAuthRequest(context.Background(), fmt.Sprintf("%s/files", COOKIE_BASE_URL), req, MakeRequestConfig(1, 1, 15), nil)
	if err != nil {
		return nil, err
	}
	for _, item := range strings.Split(c.Cookie, ";") {
		parts := strings.SplitN(strings.TrimSpace(item), "=", 2)
		if len(parts) != 2 || parts[0] != "UID" {
			continue
		}
		userID := strings.SplitN(parts[1], "_", 2)[0]
		if userID != "" {
			return &UserInfo{UserId: json.Number(userID)}, nil
		}
	}
	return nil, fmt.Errorf("115 Cookie 缺少 UID")
}

