package controllers

import (
	"Q115-STRM/internal/db"
	"Q115-STRM/internal/helpers"
	"Q115-STRM/internal/models"
	"Q115-STRM/internal/v115open"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type v115StatusResp struct {
	UserId      json.Number `json:"user_id"`
	Username    string      `json:"username"`
	UsedSpace   int64       `json:"used_space"`
	TotalSpace  int64       `json:"total_space"`
	MemberLevel string      `json:"member_level"`
	ExpireTime  string      `json:"expire_time"`
}

type KeyLockWithTimeout struct {
	mutexes sync.Map // key -> *sync.Mutex
	global  sync.Mutex
}

// LockWithTimeout 尝试获取锁，如果超时则返回 false
func (kl *KeyLockWithTimeout) LockWithTimeout(key string, timeout time.Duration) bool {
	kl.global.Lock()
	mutex, _ := kl.mutexes.LoadOrStore(key, &sync.Mutex{})
	kl.global.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return false // 超时
		default:
			if mutex.(*sync.Mutex).TryLock() {
				return true // 成功获取锁
			}
			time.Sleep(10 * time.Millisecond) // 短暂等待后重试
		}
	}
}

func (kl *KeyLockWithTimeout) Unlock(key string) {
	kl.global.Lock()
	mutex, ok := kl.mutexes.Load(key)
	kl.global.Unlock()

	if ok {
		mutex.(*sync.Mutex).Unlock()
	}
}

// Get115Status 查询115账号状态
// @Summary 查询115账号状态
// @Description 获取指定115账号的登录状态及存储信息
// @Tags 115开放平台
// @Accept json
// @Produce json
// @Param account_id query integer true "账号ID"
// @Success 200 {object} object
// @Failure 200 {object} object
// @Router /auth/115-status [get]
// @Security JwtAuth
// @Security ApiKeyAuth
func Get115Status(c *gin.Context) {
	type statusReq struct {
		AccountId uint `json:"account_id" form:"account_id"`
	}
	var req statusReq
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "参数错误", Data: nil})
		return
	}
	account, err := models.GetAccountById(req.AccountId)
	if err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "账号ID不存在", Data: nil})
		return
	}
	client := account.Get115Client()
	var resp v115StatusResp
	// 获取用户信息
	userInfo, err := client.UserInfo()
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "获取115用户信息失败: " + err.Error(), Data: nil})
		return
	}
	resp.UserId = userInfo.UserId
	resp.Username = userInfo.UserName
	resp.UsedSpace = userInfo.RtSpaceInfo.AllUse.Size
	resp.TotalSpace = userInfo.RtSpaceInfo.AllTotal.Size
	resp.MemberLevel = userInfo.VipInfo.LevelName
	if userInfo.VipInfo.Expire > 0 {
		resp.ExpireTime = helpers.FormatTimestamp(userInfo.VipInfo.Expire)
	} else {
		resp.ExpireTime = "未开通会员"
	}
	// 返回状态信息
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "获取115状态成功", Data: resp})
}

func GetFileDetail(c *gin.Context) {
	type fileDetailReq struct {
		AccountId uint   `json:"account_id" form:"account_id"`
		FileId    string `json:"file_id" form:"file_id"`
	}
	var req fileDetailReq
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "参数错误", Data: nil})
		return
	}
	account, err := models.GetAccountById(req.AccountId)
	if err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "账号ID不存在", Data: nil})
		return
	}
	fullPath := models.GetPathByPathFileId(account, req.FileId)
	if fullPath == "" {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "文件ID不存在或未找到对应路径", Data: nil})
		return
	}
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "获取文件详情成功", Data: fullPath})
}

var keyLock KeyLockWithTimeout

// Get115UrlByPickCode 查询115直链并重定向
// @Summary 获取115文件直链
// @Description 根据pickcode查询115文件直链并按需302跳转
// @Tags 115开放平台
// @Accept json
// @Produce json
// @Param pickcode query string true "文件PickCode"
// @Param userid query string false "115用户ID"
// @Param force query integer false "是否强制直链播放，1为直链，0使用本地代理时会走代理"
// @Success 302 {string} string "重定向到文件直链"
// @Failure 200 {object} object
// @Router /115/newurl [get]
func Get115UrlByPickCode(c *gin.Context) {
	type fileIdReq struct {
		UserId   string `json:"userid" form:"userid"`
		PickCode string `json:"pickcode" form:"pickcode"`
		Force    int    `json:"force" form:"force"`
	}
	var req fileIdReq
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "参数错误", Data: nil})
		return
	}
	pickCode := req.PickCode
	userId := req.UserId
	var account *models.Account
	if userId == "" {
		// 查询SyncFile
		syncFile := models.GetFileByPickCode(pickCode)
		if syncFile == nil {
			c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "文件PickCode不存在", Data: nil})
			return
		}
		var err error
		account, err = models.GetAccountById(syncFile.AccountId)
		if err != nil {
			c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "账号ID不存在", Data: nil})
			return
		}
		// helpers.AppLogger.Infof("通过PickCode查询到115账号: %s", account.Username)
	} else {
		var err error
		// 通过userId查询账号
		account, err = models.GetAccountByUserId(userId)
		if err != nil {
			c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "用户ID不存在", Data: nil})
			return
		}
		// helpers.AppLogger.Infof("通过用户ID查询到115账号: %s", account.Username)
	}
	ua := c.Request.UserAgent()
	client := account.Get115Client()
	// helpers.AppLogger.Infof("检查是否具有直链播放标记， force=%d", req.Force)
	cacheKey := fmt.Sprintf("115url:%s, ua=%s", pickCode, ua)
	// helpers.AppLogger.Infof("准备获取115文件下载链接: pickcode=%s, ua=%s，8095播放=%d 加锁10秒", pickCode, ua, req.Force)
	if keyLock.LockWithTimeout(cacheKey, 10*time.Second) {
		defer keyLock.Unlock(cacheKey)
		// helpers.AppLogger.Debugf("是否启用本地代理：%d", models.SettingsGlobal.LocalProxy)
		if req.Force == 0 && models.SettingsGlobal.LocalProxy == 1 {
			// 跳转到本地代理时使用统一的UA
			ua = v115open.DEFAULTUA
			helpers.AppLogger.Infof("因为直链标识=%d, 本地播放代理开关=%d，所以使用默认UA: %s", req.Force, models.SettingsGlobal.LocalProxy, ua)
		}
		cachedUrl := string(db.Cache.Get(cacheKey))
		if cachedUrl != "" {
			helpers.AppLogger.Debug("命中115下载链接缓存")
			if !checkURLValidity(cachedUrl, ua) {
				helpers.AppLogger.Debug("115下载链接缓存已失效，准备重新获取")
				db.Cache.Delete(cacheKey)
				cachedUrl = ""
			}
		}
		if cachedUrl == "" {
			cachedUrl = client.GetDownloadUrl(context.Background(), pickCode, ua, true)
			if cachedUrl == "" {
				c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "获取115下载链接失败", Data: nil})
				return
			}
			helpers.AppLogger.Debug("已获取115下载链接")
			// 缓存50分钟
			db.Cache.Set(cacheKey, []byte(cachedUrl), 3000)
		}
		if req.Force == 0 {
			if models.SettingsGlobal.LocalProxy == 1 {
				// 跳转到本地代理
				helpers.AppLogger.Debug("通过本地代理访问115下载链接")
				proxyUrl := fmt.Sprintf("/proxy-115?url=%s", url.QueryEscape(cachedUrl))
				c.Redirect(http.StatusFound, proxyUrl)
			} else {
				helpers.AppLogger.Debug("重定向到115下载链接")
				c.Redirect(http.StatusFound, cachedUrl)
			}
		} else {
			helpers.AppLogger.Debug("重定向到115直链")
			c.Redirect(http.StatusFound, cachedUrl)
		}
	}
}

// GetLoginQrCodeOpen 获取115原生二维码登录信息。
// @Summary 获取115登录二维码
// @Description 生成115开放平台登录二维码并异步轮询状态
// @Tags 115开放平台
// @Accept json
// @Produce json
// @Param account_id body integer true "账号ID"
// @Success 200 {object} object
// @Failure 200 {object} object
// @Router /auth/115-qrcode-open [post]
// @Security JwtAuth
// @Security ApiKeyAuth
func GetLoginQrCodeOpen(c *gin.Context) {
	type qrcodeReq struct {
		AccountId uint   `json:"account_id" form:"account_id"`
		LoginApp  string `json:"login_app" form:"login_app"`
	}
	var req qrcodeReq
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "参数错误", Data: nil})
		return
	}
	account, err := models.GetAccountById(req.AccountId)
	if err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "账号ID不存在", Data: nil})
		return
	}
	if account.SourceType != models.SourceType115 {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "账号不是115网盘账号", Data: nil})
		return
	}
	if req.LoginApp == "" {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "请选择115登录设备"})
		return
	}
	client := account.Get115Client()
	qrCodeData, err := client.GetNativeQrCode(req.LoginApp)
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "获取二维码失败: " + err.Error(), Data: nil})
		return
	}
	// 轮询二维码状态
	go func(codeData *v115open.NativeQrToken) {
		for {
			status, err := client.GetNativeQrStatus(codeData)
			if err != nil {
				db.Cache.Set("qr_error:"+codeData.UID, []byte("查询二维码状态失败，请重新扫码"), 60)
				break
			}
			db.Cache.Set("qr_status:"+codeData.UID, []byte(strconv.Itoa(int(status))), 60)
			if status == v115open.NativeQrStatusExpired || status == v115open.NativeQrStatusCanceled {
				break
			}
			if status == v115open.NativeQrStatusConfirmed {
				login, loginErr := client.LoginWithNativeQrCode(codeData)
				if loginErr != nil || !account.Update115Cookie(login.Cookie, login.UserID, login.Name, codeData.LoginApp) {
					db.Cache.Set("qr_error:"+codeData.UID, []byte("登录失败，请重新扫码"), 60)
					break
				}
				v115open.UpdateToken(account.ID, login.Cookie, "")
				break
			}
			time.Sleep(1500 * time.Millisecond)
		}
	}(qrCodeData)
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "获取二维码成功", Data: qrCodeData})
}

// GetNativeLoginApps 返回115原生二维码登录支持的全部会话槽位。
// 客户端必须显式选择，避免默认设备顶掉用户已有的115会话。
func GetNativeLoginApps(c *gin.Context) {
	c.JSON(http.StatusOK, APIResponse[[]v115open.NativeLoginApp]{
		Code:    Success,
		Message: "",
		Data:    v115open.NativeLoginApps,
	})
}

// GetQrCodeStatus 查询二维码扫码状态
// @Summary 查询115二维码扫码状态
// @Description 查询指定二维码UID的扫码进度
// @Tags 115开放平台
// @Accept json
// @Produce json
// @Param uid body string true "二维码UID"
// @Param account_id body integer true "账号ID"
// @Success 200 {object} object
// @Failure 200 {object} object
// @Router /auth/115-qrcode-status [post]
// @Security JwtAuth
// @Security ApiKeyAuth
func GetQrCodeStatus(c *gin.Context) {
	type qrcodeReq struct {
		Uid       string `json:"uid" form:"uid"`
		AccountId uint   `json:"account_id" form:"account_id"`
	}
	var req qrcodeReq
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "参数错误", Data: nil})
		return
	}
	uid := req.Uid
	s := v115open.NativeQrStatusWaiting
	cachedStatus := db.Cache.Get("qr_status:" + uid)
	if cachedStatus != nil {
		statusInt, err := strconv.Atoi(string(cachedStatus))
		if err == nil {
			s = v115open.NativeQrStatus(statusInt)
		}
	}
	data := map[string]any{"status": s}
	if reason := db.Cache.Get("qr_error:" + uid); reason != nil {
		data["reason"] = string(reason)
	}
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "", Data: data})
}

// 生成并返回115 oauth登录地址
func GetOAuthUrl(c *gin.Context) {
	accountID := helpers.StringToInt(c.Query("account_id"))
	account, err := models.GetAccountById(uint(accountID))
	if err != nil || account.SourceType != models.SourceType115 {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "115账号不存在", Data: nil})
		return
	}
	redirectURL := c.Query("redirect_url")
	if redirectURL == "" {
		redirectURL = "/"
	}
	c.JSON(http.StatusOK, APIResponse[string]{Code: Success, Message: "请使用115 App扫码登录", Data: fmt.Sprintf("/api/115/native-login?account_id=%d&redirect_url=%s", account.ID, url.QueryEscape(redirectURL))})
}

// Native115LoginPage 为旧版前端提供原生二维码登录页面，避免继续跳转作者授权服务。
func Native115LoginPage(c *gin.Context) {
	accountID := helpers.StringToInt(c.Query("account_id"))
	account, err := models.GetAccountById(uint(accountID))
	if err != nil || account.SourceType != models.SourceType115 {
		c.String(http.StatusBadRequest, "115账号不存在")
		return
	}
	redirectURL, _ := json.Marshal(c.Query("redirect_url"))
	loginApps, _ := json.Marshal(v115open.NativeLoginApps)
	page := fmt.Sprintf(`<!doctype html><html lang="zh-CN"><meta charset="utf-8"><title>115 扫码登录</title><style>body{font-family:sans-serif;text-align:center;margin:48px;color:#202124}img{width:280px;height:280px;display:none}#status{margin:18px}select,button{font-size:16px;padding:8px;margin:4px}</style><h2>使用官方 115 App 扫码登录</h2><p>请选择一个未被你日常设备使用的登录会话槽位。</p><select id="app"></select><button id="start">生成二维码</button><br><img id="qr" alt="115 登录二维码"><p id="status">请先选择登录设备</p><script>const accountId=%d,redirectURL=%s,apps=%s;const status=document.getElementById('status'),qr=document.getElementById('qr'),app=document.getElementById('app'),start=document.getElementById('start');for(const item of apps){const option=document.createElement('option');option.value=item.app;option.textContent=item.description+' ('+item.app+')';app.append(option)}let timer;start.onclick=()=>{if(timer)clearInterval(timer);qr.style.display='none';start.disabled=true;status.textContent='正在获取二维码…';fetch('/api/auth/115-qrcode-open',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({account_id:accountId,login_app:app.value})}).then(r=>r.json()).then(r=>{if(r.code!==200)throw Error(r.message);qr.src=r.data.qrcode;qr.style.display='inline-block';status.textContent='请在 115 App 中扫码并确认';timer=setInterval(()=>fetch('/api/auth/115-qrcode-status',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({account_id:accountId,uid:r.data.uid})}).then(x=>x.json()).then(x=>{if(x.data.reason){clearInterval(timer);start.disabled=false;status.textContent=x.data.reason;return}if(x.data.status===1)status.textContent='已扫码，请在 115 App 确认';if(x.data.status===2){clearInterval(timer);status.textContent='登录成功，正在返回…';setTimeout(()=>location.href=redirectURL,800)}if(x.data.status===-1||x.data.status===-2){clearInterval(timer);start.disabled=false;status.textContent='二维码已失效或取消，请重新生成'}}),1500)}).catch(e=>{start.disabled=false;status.textContent='获取二维码失败：'+e.message})};</script></html>`, account.ID, string(redirectURL), string(loginApps))
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(page))
}

func ConfirmOAuthCode(c *gin.Context) {
	c.JSON(http.StatusGone, APIResponse[any]{Code: BadRequest, Message: "115 OAuth 已停用，请使用原生二维码登录", Data: nil})
}

// GetQueueStats 获取115 OpenAPI请求队列的统计数据
func GetQueueStats(c *gin.Context) {
	// 获取查询参数，支持查询不同时间窗口的统计
	timeWindowStr := c.DefaultQuery("time_window", "3600") // 默认3600秒（1小时）
	timeWindow, err := strconv.ParseInt(timeWindowStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "time_window参数无效", Data: nil})
		return
	}

	duration := time.Duration(timeWindow) * time.Second

	// 获取全局队列执行器
	executor := v115open.GetGlobalExecutor()

	// 获取统计数据
	stats := executor.GetStats(duration)

	// 获取限流状态
	throttleStatus := executor.GetThrottleStatus()

	// 构建响应数据
	responseData := gin.H{
		"total_requests":           stats.TotalRequests,
		"qps_count":                stats.QPSCount,
		"qpm_count":                stats.QPMCount,
		"qph_count":                stats.QPHCount,
		"throttled_count":          stats.ThrottledCount,
		"avg_response_time_ms":     stats.AvgResponseTime,
		"last_throttle_time":       stats.LastThrottleTime,
		"throttle_wait_time":       stats.ThrottledWaitTime.String(),
		"throttle_recover_time":    stats.ThrottleRecoverTime,
		"time_window_seconds":      timeWindow,
		"is_throttled":             throttleStatus.IsThrottled,
		"throttled_elapsed_time":   throttleStatus.ElapsedTime.String(),
		"throttled_remaining_time": throttleStatus.RemainingTime.String(),
	}

	c.JSON(http.StatusOK, APIResponse[gin.H]{Code: Success, Message: "获取队列统计数据成功", Data: responseData})
}

// SetQueueRateLimit 设置115 OpenAPI请求队列的速率限制参数
func SetQueueRateLimit(c *gin.Context) {
	var req struct {
		QPS int `json:"qps" binding:"required,min=1,max=1000"`
		QPM int `json:"qpm" binding:"required,min=1,max=100000"`
		QPH int `json:"qph" binding:"required,min=1,max=1000000"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "请求参数错误: " + err.Error(), Data: nil})
		return
	}

	// 设置全局执行器的速率限制配置
	v115open.SetGlobalExecutorConfig(req.QPS, req.QPM, req.QPH)

	helpers.AppLogger.Infof("115 OpenAPI队列速率限制已更新: QPS=%d, QPM=%d, QPH=%d", req.QPS, req.QPM, req.QPH)

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "速率限制配置成功", Data: gin.H{
		"qps": req.QPS,
		"qpm": req.QPM,
		"qph": req.QPH,
	}})
}

// GetRequestStatsByDay 获取指定日期范围内的请求统计（按天分组）
func GetRequestStatsByDay(c *gin.Context) {
	// 获取查询参数
	startDateStr := c.DefaultQuery("start_date", time.Now().AddDate(0, 0, -7).Format("2006-01-02")) // 默认最近7天
	endDateStr := c.DefaultQuery("end_date", time.Now().Format("2006-01-02"))

	// 解析日期
	startDate, err := time.ParseInLocation("2006-01-02", startDateStr, time.Local)
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "start_date 参数格式错误，应为 YYYY-MM-DD", Data: nil})
		return
	}
	endDate, err := time.ParseInLocation("2006-01-02", endDateStr, time.Local)
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "end_date 参数格式错误，应为 YYYY-MM-DD", Data: nil})
		return
	}

	// 设置时间范围（开始时间为当天0点，结束时间为当天23:59:59）
	startTime := startDate.Unix()
	endTime := endDate.Add(24*time.Hour - time.Second).Unix()

	// 获取按天分组的统计数据
	dailyStats, err := models.GetDailyRequestStats(startTime, endTime)
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "查询统计数据失败: " + err.Error(), Data: nil})
		return
	}

	// 获取总请求数和限流请求数
	totalCount, _ := models.GetRequestStatsCount(startTime, endTime)
	throttledCount, _ := models.GetThrottledRequestsCount(startTime, endTime)

	responseData := gin.H{
		"start_date":            startDateStr,
		"end_date":              endDateStr,
		"total_requests":        totalCount,
		"total_throttled":       throttledCount,
		"daily_stats":           dailyStats,
		"query_time_range_days": int(endDate.Sub(startDate).Hours() / 24),
	}

	c.JSON(http.StatusOK, APIResponse[gin.H]{Code: Success, Message: "获取日统计数据成功", Data: responseData})
}

// GetRequestStatsByHour 获取指定日期范围内的请求统计（按小时分组）
func GetRequestStatsByHour(c *gin.Context) {
	// 获取查询参数
	startDateStr := c.DefaultQuery("start_date", time.Now().AddDate(0, 0, -1).Format("2006-01-02")) // 默认昨天
	endDateStr := c.DefaultQuery("end_date", time.Now().Format("2006-01-02"))                       // 默认今天

	// 解析日期
	startDate, err := time.ParseInLocation("2006-01-02", startDateStr, time.Local)
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "start_date 参数格式错误，应为 YYYY-MM-DD", Data: nil})
		return
	}
	endDate, err := time.ParseInLocation("2006-01-02", endDateStr, time.Local)
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "end_date 参数格式错误，应为 YYYY-MM-DD", Data: nil})
		return
	}

	// 设置时间范围
	startTime := startDate.Unix()
	endTime := endDate.Add(24*time.Hour - time.Second).Unix()

	// 获取按小时分组的统计数据
	hourlyStats, err := models.GetHourlyRequestStats(startTime, endTime)
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "查询统计数据失败: " + err.Error(), Data: nil})
		return
	}

	// 获取总请求数和限流请求数
	totalCount, _ := models.GetRequestStatsCount(startTime, endTime)
	throttledCount, _ := models.GetThrottledRequestsCount(startTime, endTime)

	responseData := gin.H{
		"start_date":            startDateStr,
		"end_date":              endDateStr,
		"total_requests":        totalCount,
		"total_throttled":       throttledCount,
		"hourly_stats":          hourlyStats,
		"query_time_range_days": int(endDate.Sub(startDate).Hours() / 24),
	}

	c.JSON(http.StatusOK, APIResponse[gin.H]{Code: Success, Message: "获取小时统计数据成功", Data: responseData})
}

// CleanOldRequestStats 清理旧的请求统计数据
func CleanOldRequestStats(c *gin.Context) {
	type cleanReq struct {
		Days int `json:"days" binding:"required,min=1,max=365"`
	}

	var req cleanReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "请求参数错误: " + err.Error(), Data: nil})
		return
	}

	err := models.CleanOldRequestStats(req.Days)
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "清理统计数据失败: " + err.Error(), Data: nil})
		return
	}

	helpers.AppLogger.Infof("已清理 %d 天前的请求统计数据", req.Days)

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: fmt.Sprintf("已清理 %d 天前的请求统计数据", req.Days), Data: nil})
}

// checkURLValidity 使用HEAD请求检查URL是否有效
// 返回true表示URL有效（2xx状态码），false表示URL已失效
// ua参数：必须使用当前请求的USER-AGENT访问115链接（否则返回403）
func checkURLValidity(urlStr string, ua string) bool {
	helpers.AppLogger.Debug("开始检查115下载链接有效性")
	client := &http.Client{
		Timeout: 3 * time.Second,
		Transport: &http.Transport{
			ResponseHeaderTimeout: 2 * time.Second, // 等待响应头的超时
			DisableKeepAlives:     true,            // 禁用长连接，请求完立即关闭
			TLSHandshakeTimeout:   1 * time.Second, // TLS握手超时
			MaxIdleConns:          0,               // 不保持空闲连接
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// 不跟随重定向，只检查第一次响应
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequest("HEAD", urlStr, nil)
	if err != nil {
		helpers.AppLogger.Errorf("创建HEAD请求失败: %v", err)
		return false
	}

	// 设置User-Agent，这是关键！115链接必须使用请求时的UA
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}

	resp, err := client.Do(req)
	if err != nil {
		helpers.AppLogger.Errorf("HEAD请求失败: %v", err)
		return false
	}
	defer resp.Body.Close()

	// 2xx状态码表示有效
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		helpers.AppLogger.Infof("URL有效性检查通过: 状态码=%d", resp.StatusCode)
		return true
	}

	helpers.AppLogger.Infof("URL已失效: 状态码=%d", resp.StatusCode)
	return false
}
