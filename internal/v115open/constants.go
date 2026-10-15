package v115open

import (
	"Q115-STRM/internal/helpers"
	"fmt"
)

const (
	// API常量
	ACCESS_TOKEN_AUTH_FAIL   = 40140126 // 刷新访问凭证
	ACCESS_TOKEN_EXPIRY_CODE = 40140125 // 刷新访问凭证
	ACCESS_AUTH_INVALID      = 40140124 // 刷新访问凭证
	REFRESH_TOKEN_INVALID    = 40140116 // 重新授权
	REQUEST_MAX_LIMIT_CODE   = 770004   // 访问频率过高
	REQUEST_RATE_LIMIT_CODE  = 406      // 已达到当前访问上限，购买更高等级VIP可获更多额度
	OPEN_BASE_URL            = "https://proapi.115.com"
	COOKIE_BASE_URL          = "https://webapi.115.com"

	// 重试配置
	DEFAULT_MAX_RETRIES = 3
	DEFAULT_RETRY_DELAY = 1

	// 超时配置
	DEFAULT_TIMEOUT = 30 // 秒
)

var DEFAULTUA = fmt.Sprintf("QMediaSync-GoClient/%s", helpers.Version)

