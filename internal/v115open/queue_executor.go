package v115open

import (
	"Q115-STRM/internal/helpers"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"golang.org/x/time/rate"
	"resty.dev/v3"
)

// RequestStatSaver 请求统计保存回调函数类型
type RequestStatSaver func(requestTime int64, url, method string, duration int64, isThrottled bool)

// QueueExecutor 请求队列执行器，负责管理所有API请求的队列和执行
type QueueExecutor struct {
	sync.RWMutex
	// 请求队列通道，缓冲100
	requestQueue chan *QueuedRequest
	// Worker数量
	workerCount int
	// 是否正在运行
	running bool
	// 停止通道
	stopChan chan struct{}
	// Worker停止信号
	workerStopChans []chan struct{}
	// 速率限制器（支持qps/qpm/qph）
	qpsLimiter *rate.Limiter // 每秒请求数限制
	qpmLimiter *rate.Limiter // 每分钟请求数限制
	qphLimiter *rate.Limiter // 每小时请求数限制
	// 全局限流管理器
	throttleManager *ThrottleManager
	// 统计数据
	stats *RequestStats
	// QPS配置
	qpsConfig int
	qpmConfig int
	qphConfig int
	// 请求统计保存回调函数
	statSaver RequestStatSaver
}

// 全局队列执行器实例
var globalExecutor *QueueExecutor
var executorOnce sync.Once

// GetGlobalExecutor 获取全局队列执行器实例（单例）
func GetGlobalExecutor() *QueueExecutor {
	executorOnce.Do(func() {
		// 默认QPS=10，QPM=500，QPH=10000
		globalExecutor = NewQueueExecutor(3, 200, 12000)
		globalExecutor.Start()
	})
	return globalExecutor
}

// SetGlobalExecutorConfig 设置全局执行器的速率限制配置
func SetGlobalExecutorConfig(qps, qpm, qph int) {
	executor := GetGlobalExecutor()
	executor.SetRateLimitConfig(qps, qpm, qph)
}

// SetGlobalExecutorStatSaver 设置全局执行器的统计保存回调函数
func SetGlobalExecutorStatSaver(saver RequestStatSaver) {
	executor := GetGlobalExecutor()
	executor.SetStatSaver(saver)
}

// NewQueueExecutor 创建新的队列执行器
func NewQueueExecutor(qps, qpm, qph int) *QueueExecutor {
	// 计算Worker数量：max(qps, 5) + 3
	workerCount := qps
	if workerCount < 5 {
		workerCount = 5
	}
	workerCount += 3

	executor := &QueueExecutor{
		requestQueue:    make(chan *QueuedRequest, 100), // 缓冲100
		workerCount:     workerCount,
		stopChan:        make(chan struct{}),
		workerStopChans: make([]chan struct{}, 0, workerCount),
		throttleManager: NewThrottleManager(),
		stats:           NewRequestStats(10000),
		qpsConfig:       qps,
		qpmConfig:       qpm,
		qphConfig:       qph,
	}

	// 创建速率限制器
	// qps: 每秒请求数
	executor.qpsLimiter = rate.NewLimiter(rate.Limit(qps), qps)
	// qpm: 每分钟请求数
	executor.qpmLimiter = rate.NewLimiter(rate.Every(time.Minute/time.Duration(qpm)), qpm)
	// qph: 每小时请求数
	executor.qphLimiter = rate.NewLimiter(rate.Every(time.Hour/time.Duration(qph)), qph)

	return executor
}

// SetRateLimitConfig 设置速率限制配置
func (qe *QueueExecutor) SetRateLimitConfig(qps, qpm, qph int) {
	qe.Lock()

	qe.qpsConfig = qps
	qe.qpmConfig = qpm
	qe.qphConfig = qph

	// 更新限制器
	qe.qpsLimiter = rate.NewLimiter(rate.Limit(qps), qps)
	qe.qpmLimiter = rate.NewLimiter(rate.Every(time.Minute/time.Duration(qpm)), qpm)
	qe.qphLimiter = rate.NewLimiter(rate.Every(time.Hour/time.Duration(qph)), qph)

	// 重新计算Worker数量
	newWorkerCount := qps
	if newWorkerCount < 5 {
		newWorkerCount = 5
	}
	newWorkerCount += 3

	needRestart := newWorkerCount != qe.workerCount && qe.running
	oldWorkerCount := qe.workerCount

	qe.Unlock() // 在可能调用Stop/Start之前释放锁，避免死锁

	if needRestart {
		// 如果Worker数量改变且正在运行，需要重启
		helpers.V115Log.Warnf("速率限制配置已更改，将重启执行器以应用新的Worker数量：%d -> %d", oldWorkerCount, newWorkerCount)
		qe.Stop()
		qe.Lock()
		qe.workerCount = newWorkerCount
		qe.Unlock()
		qe.Start()
	}
}

// SetStatSaver 设置统计保存回调函数
func (qe *QueueExecutor) SetStatSaver(saver RequestStatSaver) {
	qe.Lock()
	defer qe.Unlock()
	qe.statSaver = saver
}

// Start 启动队列执行器
func (qe *QueueExecutor) Start() {
	qe.Lock()
	if qe.running {
		qe.Unlock()
		return
	}
	qe.running = true
	// 检查queueRequest通道是否已关闭，若已关闭则重新创建
	if qe.requestQueue == nil {
		qe.requestQueue = make(chan *QueuedRequest, 100)
	}
	qe.Unlock()

	helpers.V115Log.Infof("启动115 OpenAPI队列执行器，Worker数量: %d, QPS: %d, QPM: %d, QPH: %d",
		qe.workerCount, qe.qpsConfig, qe.qpmConfig, qe.qphConfig)

	// 启动Worker
	for i := 0; i < qe.workerCount; i++ {
		stopChan := make(chan struct{})
		qe.workerStopChans = append(qe.workerStopChans, stopChan)
		go qe.worker(i, stopChan)
	}
}

// Stop 停止队列执行器
func (qe *QueueExecutor) Stop() {
	qe.Lock()
	if !qe.running {
		qe.Unlock()
		return
	}
	qe.running = false
	qe.Unlock()

	helpers.V115Log.Infof("停止115 OpenAPI队列执行器")

	// 停止所有Worker
	for _, stopChan := range qe.workerStopChans {
		select {
		case stopChan <- struct{}{}:
		default:
		}
	}

	// 关闭队列通道
	close(qe.requestQueue)
	qe.requestQueue = nil
}

// worker Worker协程，处理队列中的请求
func (qe *QueueExecutor) worker(id int, stopChan chan struct{}) {
	for {
		select {
		case <-stopChan:
			helpers.V115Log.Debugf("Worker %d 已停止", id)
			return
		case req, ok := <-qe.requestQueue:
			if !ok {
				// 通道已关闭
				return
			}

			// 处理请求
			qe.handleRequest(req)
		}
	}
}

// handleRequest 处理单个请求
func (qe *QueueExecutor) handleRequest(req *QueuedRequest) {
	startTime := time.Now()

	// 检查限流状态
	if qe.throttleManager.IsThrottled() {
		helpers.V115Log.Debugf("系统处于限流状态，等待恢复...")
		qe.throttleManager.WaitThrottleRecovery(req.Ctx)
	}

	// 如果不绕过速率限制，则检查三层限制
	if !req.BypassRateLimit {
		// 等待QPS限制
		if err := qe.qpsLimiter.Wait(req.Ctx); err != nil {
			req.ResponseChan <- &RequestResponse{
				Error:    fmt.Errorf("QPS限制错误: %w", err),
				Duration: time.Since(startTime).Milliseconds(),
			}
			return
		}

		// 等待QPM限制
		if err := qe.qpmLimiter.Wait(req.Ctx); err != nil {
			req.ResponseChan <- &RequestResponse{
				Error:    fmt.Errorf("QPM限制错误: %w", err),
				Duration: time.Since(startTime).Milliseconds(),
			}
			return
		}

		// 等待QPH限制
		if err := qe.qphLimiter.Wait(req.Ctx); err != nil {
			req.ResponseChan <- &RequestResponse{
				Error:    fmt.Errorf("QPH限制错误: %w", err),
				Duration: time.Since(startTime).Milliseconds(),
			}
			return
		}
	} else {
		// 播放请求：只检查限流状态，不检查速率限制
		// 但仍然等待限流恢复
		if qe.throttleManager.IsThrottled() {
			qe.throttleManager.WaitThrottleRecovery(req.Ctx)
		}
	}

	// 发送请求
	response, respData, respBytes, err := qe.executeRequest(req)

	duration := time.Since(startTime).Milliseconds()

	// 检查是否是限流响应
	isThrottled := false
	if respData != nil && respData.Code == REQUEST_MAX_LIMIT_CODE {
		isThrottled = true
		qe.throttleManager.MarkThrottled(qe.stats)
	}

	// 记录请求
	qe.stats.RecordRequest(RequestLogEntry{
		Timestamp:   time.Now(),
		Duration:    duration,
		IsThrottled: isThrottled,
		URL:         req.URL,
		Method:      req.Method,
	})

	// 异步写入数据库（如果设置了回调函数）
	if qe.statSaver != nil {
		go qe.statSaver(time.Now().Unix(), req.URL, req.Method, duration, isThrottled)
	}

	// 发送响应
	respChan := req.ResponseChan
	select {
	case respChan <- &RequestResponse{
		Response:    response,
		RespData:    respData,
		RespBytes:   respBytes,
		Error:       err,
		Duration:    duration,
		IsThrottled: isThrottled,
	}:
	default:
		helpers.V115Log.Warnf("响应通道已关闭或已满，丢弃响应: %s %s", req.Method, req.URL)
	}

	// 关闭响应通道
	close(respChan)
}

// executeRequest 执行HTTP请求
func (qe *QueueExecutor) executeRequest(req *QueuedRequest) (*resty.Response, *RespBaseBool[json.RawMessage], []byte, error) {
	req.Request.SetForceResponseContentType("application/json")

	var response *resty.Response
	var err error

	method := req.Request.Method
	switch method {
	case "GET":
		response, err = req.Request.Get(req.URL)
	case "POST":
		response, err = req.Request.Post(req.URL)
	default:
		return nil, nil, nil, fmt.Errorf("不支持的HTTP方法: %s", method)
	}

	if err != nil {
		return response, nil, nil, err
	}

	// 解析响应
	defer response.Body.Close()
	resBytes, ioErr := io.ReadAll(response.Body)
	if ioErr != nil {
		return response, nil, nil, ioErr
	}

	resp, bodyErr := parse115Response(resBytes)
	if bodyErr != nil {
		return response, nil, resBytes, bodyErr
	}

	// 不记录响应 data：其中可能包含文件名、目录结构等用户私有信息。
	helpers.V115Log.Infof("队列执行 %s %s state=%v, code=%d",
		req.Request.Method, req.URL, resp.State, resp.Code)

	// 检查错误码
	switch resp.Code {
	case ACCESS_TOKEN_AUTH_FAIL, ACCESS_AUTH_INVALID, ACCESS_TOKEN_EXPIRY_CODE:
		helpers.V115Log.Warn("访问凭证已过期")
		return response, resp, resBytes, fmt.Errorf("token expired")
	case REFRESH_TOKEN_INVALID:
		helpers.V115Log.Error("访问凭证无效，请重新登录")
		return response, resp, resBytes, fmt.Errorf("token expired")
	case REQUEST_MAX_LIMIT_CODE:
		helpers.V115Log.Warn("检测到限流响应")
		return response, resp, resBytes, fmt.Errorf("访问频率过高")
	}

	if !resp.State || (resp.Code != 0 && resp.Code != 200) {
		return response, resp, resBytes, fmt.Errorf("115请求失败，错误码：%d", resp.Code)
	}

	return response, resp, resBytes, nil
}

// parse115Response accepts the scalar variants used by both the OpenAPI and
// the Cookie web API. The latter uses errNo/error instead of code/message.
func parse115Response(data []byte) (*RespBaseBool[json.RawMessage], error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("解析115响应失败: %w", err)
	}
	state, err := parse115Bool(raw["state"])
	if err != nil {
		return nil, fmt.Errorf("解析115响应状态失败: %w", err)
	}
	codeRaw := raw["code"]
	if len(codeRaw) == 0 || string(codeRaw) == "null" {
		codeRaw = raw["errNo"]
	}
	code, err := parse115Int(codeRaw)
	if err != nil {
		return nil, fmt.Errorf("解析115响应错误码失败: %w", err)
	}
	messageRaw := raw["message"]
	if len(messageRaw) == 0 || string(messageRaw) == "null" {
		messageRaw = raw["error"]
	}
	if len(messageRaw) == 0 || string(messageRaw) == "null" {
		messageRaw = raw["msg"]
	}
	message, err := parse115String(messageRaw)
	if err != nil {
		return nil, fmt.Errorf("解析115响应消息失败: %w", err)
	}
	return &RespBaseBool[json.RawMessage]{State: state, Code: code, Message: message, Data: raw["data"]}, nil
}

// EnqueueRequest 将请求加入队列
func (qe *QueueExecutor) EnqueueRequest(req *QueuedRequest) {
	qe.RLock()
	if !qe.running {
		qe.RUnlock()
		helpers.V115Log.Error("队列执行器未启动")
		return
	}
	qe.RUnlock()

	// 发送到队列（如果缓冲满则阻塞）
	qe.requestQueue <- req
}

// GetStats 获取统计数据
func (qe *QueueExecutor) GetStats(duration time.Duration) *StatsSnapshot {
	return qe.stats.GetStats(duration)
}

// GetThrottleStatus 获取限流状态
func (qe *QueueExecutor) GetThrottleStatus() ThrottleStatus {
	return qe.throttleManager.GetThrottleStatus()
}

// SetThrottledForTesting 手动设置限流状态（仅用于测试）
func (qe *QueueExecutor) SetThrottledForTesting(throttled bool) {
	if throttled {
		qe.throttleManager.MarkThrottled(qe.stats)
	} else {
		qe.throttleManager.ClearThrottled()
	}
}

