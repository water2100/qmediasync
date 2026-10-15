package v115open

import (
	"Q115-STRM/internal/helpers"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss/credentials"
)

type DownloadUrlData struct {
	FileName string      `json:"file_name"`
	FileSize json.Number `json:"file_size"`
	PickCode string      `json:"pick_code"`
	Sha1     string      `json:"sha1"`
	Url      struct {
		Url string `json:"url"`
	} `json:"url"`
}

type DownloadUrlResp struct {
	RespBaseBool[map[string]DownloadUrlData]
}

// ├─file_id	string	文件id
// ├─parent_id	string	文件父目录id
// ├─file_name	string	文件名称
// ├─file_size	string	文件大小
// ├─file_sha1	string	文件哈希值
// ├─file_type	string	文件类型
// ├─is_private	string	文件是否加密隐藏；0：否；1：是
// ├─play_long	string	视频文件时长
// ├─ user_def	int	视频文件记忆选中的清晰度；1:标清 2:高清 3:超清 4:1080P 5:4k;100:原画
// ├─ user_rotate	int	记忆视频旋转角度；0, 90, 180, 270
// ├─ user_turn	int	视频翻转方向：0：不翻转；1：水平翻转；2：垂直翻转
// ├─ multitrack_list	array	视频多音轨列表
//
//	├─ title	string	音轨标题
//	├─ is_selected	string	音轨是否上次选中；1：选中
//
// ├─ definition_list_new	array	视频所有用可切换的清晰度列表;1:标清 2:高清 3:超清 4:1080P 5:4k;100:原画
// ├─ video_url	array	视频各清晰度的播放地址信息
//
//	├─ url	string	播放地址
//	├─ height	int	视频高度
//	├─ width	int	视频宽度
//	├─ definition	int	视频清晰度
//	├─ title	int	视频清晰度名称
//	├─ definition_n	int	视频清晰度(新)
type VideoPlayUrlData struct {
	FileId         string `json:"file_id"`
	ParentId       string `json:"parent_id"`
	FileName       string `json:"file_name"`
	FileSize       string `json:"file_size"`
	FileSha1       string `json:"file_sha1"`
	FileType       string `json:"file_type"`
	IsPrivate      string `json:"is_private"`
	PlayLong       string `json:"play_long"`
	UserDef        int    `json:"user_def"`
	UserRotate     int    `json:"user_rotate"`
	UserTurn       int    `json:"user_turn"`
	MultitrackList []struct {
		Title      string `json:"title"`
		IsSelected string `json:"is_selected"`
	} `json:"multitrack_list"`
	DefinitionListNew map[string]string `json:"definition_list_new"` // 清晰度列表{
	VideoUrl          []struct {
		Url         string `json:"url"`
		Height      int    `json:"height"`
		Width       int    `json:"width"`
		Definition  int    `json:"definition"`
		Title       string `json:"title"`
		DefinitionN int    `json:"definition_n"`
	} `json:"video_url"`
}

// ├─ endpoint	string	上传域名
// ├─ AccessKeySecret	string	上传凭证-密钥
// ├─ SecurityToken	string	上传凭证-token
// ├─ Expiration	string	上传凭证-过期日期
// ├─ AccessKeyId	string	上传凭证-ID

type UploadToken struct {
	Endpoint        string `json:"endpoint"`
	AccessKeySecret string `json:"AccessKeySecret"`
	SecurityToken   string `json:"SecurityToken"`
	Expiration      string `json:"Expiration"`
	AccessKeyId     string `json:"AccessKeyId"`
}
type UploadResultCallBack struct {
	Callback    string `json:"callback"`
	CallbackVar string `json:"callback_var"`
}
type UploadResult[T any] struct {
	PickCode  string `json:"pick_code"`
	Status    int    `json:"status"`
	FileId    string `json:"file_id"`
	Target    string `json:"target"`
	Bucket    string `json:"bucket"`
	Object    string `json:"object"`
	SignKey   string `json:"sign_key"`
	SignCheck string `json:"sign_check"`
	Callback  T      `json:"callback"`
}

// 获取文件下载地址
// POST 域名 + /open/ufile/downurl
func (c *OpenClient) GetDownloadUrl(ctx context.Context, pickCode string, userAgent string, bypassRateLimit bool) string {
	params := map[string]string{
		"pick_code": pickCode,
	}
	url := fmt.Sprintf("%s/open/ufile/downurl", OPEN_BASE_URL)
	req := c.client.R().SetFormData(params).SetMethod("POST").SetHeader("User-Agent", userAgent)
	respData := DownloadUrlResp{}
	config := MakeRequestConfig(0, 0, 0)
	config.BypassRateLimit = bypassRateLimit
	_, respBytes, err := c.doAuthRequest(ctx, url, req, config, nil)
	if err != nil {
		helpers.V115Log.Errorf("获取文件下载地址失败: %v", err)
		return ""
	}
	jsonErr := json.Unmarshal(respBytes, &respData)
	if jsonErr != nil || !respData.State {
		helpers.V115Log.Errorf("获取文件下载地址失败: %v", jsonErr)
		return ""
	}
	data := respData.Data
	var first DownloadUrlData
	for _, v := range data {
		first = v
		break
	}
	return first.Url.Url
}

// 获取视频播放链接
// POST 域名 + /open/ufile/downurl
func (c *OpenClient) GetVideoPlayUrl(ctx context.Context, pickCode string, userAgent string) *VideoPlayUrlData {
	params := map[string]string{
		"pick_code": pickCode,
	}
	url := fmt.Sprintf("%s/open/video/play", OPEN_BASE_URL)
	req := c.client.R().SetQueryParams(params).SetMethod("GET").SetHeader("User-Agent", userAgent)
	respData := &VideoPlayUrlData{}
	_, _, err := c.doAuthRequest(ctx, url, req, MakeRequestConfig(0, 0, 0), respData)
	if err != nil {
		helpers.V115Log.Errorf("获取视频播放地址失败: %v", err)
		return nil
	}
	return respData
}

// 初始化上传进程
// POST 域名 + /open/upload/init
func (c *OpenClient) Upload(ctx context.Context, filePath string, parentFileId string, signKey string, signVal string) (string, error) {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		helpers.V115Log.Errorf("获取文件信息失败: %v", err)
		return "", err
	}
	fileName := fileInfo.Name()
	fileSize := fileInfo.Size()
	fileSha1, err := helpers.FileSHA1(filePath)
	if err != nil {
		helpers.V115Log.Errorf("计算文件 SHA1 失败: %v", err)
		return "", err
	}
	preSha1, err := helpers.FileSHA1Partial(filePath, 0, 128)
	if err != nil {
		helpers.V115Log.Errorf("计算文件前128位 SHA1 失败: %v", err)
		return "", err
	}
	params := map[string]string{
		"file_name": fileName,
		"file_size": fmt.Sprintf("%d", fileSize),
		"target":    fmt.Sprintf("U_1_%s", parentFileId),
		"fileid":    fileSha1,
		"pre_id":    preSha1,
		"topupload": "0",
	}
	helpers.V115Log.Debug("准备115文件上传")
	if signKey != "" && signVal != "" {
		params["sign_key"] = signKey
		params["sign_val"] = signVal
	}
	url := fmt.Sprintf("%s/open/upload/init", OPEN_BASE_URL)
	req := c.client.R().SetFormData(params).SetMethod("POST")
	respData := &UploadResult[json.RawMessage]{}
	_, _, uErr := c.doAuthRequest(ctx, url, req, MakeRequestConfig(1, 1, 15), respData)
	if uErr != nil {
		helpers.V115Log.Errorf("上传失败: %v", uErr)
		return "", uErr
	}
	status := respData.Status
	if status == 7 {
		// 需要二次认证
		signCheck := respData.SignCheck
		signKey := respData.SignKey
		// 将signCheck用_分割
		signParts := strings.Split(signCheck, "-")
		if len(signParts) != 2 {
			helpers.V115Log.Errorf("签名检查格式错误: %v", signParts)
			return "", fmt.Errorf("签名检查格式错误: %v", signParts)
		}
		offset := helpers.StringToInt64(signParts[0])
		length := helpers.StringToInt64(signParts[1])
		helpers.V115Log.Debug("115上传需要二次签名验证")
		signVal, _ := helpers.FileSHA1Partial(filePath, offset, length)
		params["sign_key"] = signKey
		params["sign_val"] = signVal
		// fmt.Printf("二次认证参数: sign_key=%s, sign_val=%s\n", params["sign_key"], params["sign_val"])
		// 需要二次认证，再次请求接口
		return c.Upload(ctx, filePath, parentFileId, signKey, signVal)
	}
	if status == 2 {
		// 秒传成功
		return respData.FileId, nil
	}
	if status == 6 {
		helpers.V115Log.Error("签名验证后失败")
		return "", fmt.Errorf("签名验证后失败")
	}
	if status == 8 {
		helpers.V115Log.Error("签名认证失败")
		return "", fmt.Errorf("签名认证失败")
	}
	if status == 1 {
		// 非秒传，开始普通上传流程
		// 获取上传凭证
		uploadToken := c.GetUploadToken(ctx)
		if uploadToken == nil {
			helpers.V115Log.Error("获取上传凭证失败")
			return "", fmt.Errorf("获取上传凭证失败")
		}
		// 准备调用OSS对象存储上传文件，准备参数
		callbackData := &UploadResultCallBack{}
		json.Unmarshal(respData.Callback, callbackData)
		callback := callbackData.Callback
		callbackVar := callbackData.CallbackVar
		bucket := respData.Bucket
		objectId := respData.Object
		helpers.V115Log.Debug("开始115 OSS文件上传")
		callbackResult, ossErr := OssUploadFile(uploadToken.Endpoint, uploadToken.AccessKeyId, uploadToken.AccessKeySecret, uploadToken.SecurityToken, bucket, objectId, callback, callbackVar, filePath, fileSize, fileSha1)
		if ossErr != nil {
			// helpers.V115Log.Error("OSS上传失败: %v", ossErr)
			return "", ossErr
		}
		if callbackResult == nil {
			helpers.V115Log.Error("OSS上传回调结果为空")
			return "", fmt.Errorf("OSS上传回调结果为空")
		}
		if callbackResult["message"].(string) != "" {
			helpers.V115Log.Errorf("OSS上传回调失败: %v", callbackResult["message"])
			return "", fmt.Errorf("OSS上传回调失败: %v", callbackResult["message"])
		}

		return callbackResult["data"].(map[string]interface{})["file_id"].(string), nil
	}
	return respData.FileId, nil
}

// 获取115上传凭证
// GET /open/upload/get_token
func (c *OpenClient) GetUploadToken(ctx context.Context) *UploadToken {
	url := fmt.Sprintf("%s/open/upload/get_token", OPEN_BASE_URL)
	req := c.client.R().SetMethod("GET")
	respData := &UploadToken{}
	_, _, uErr := c.doAuthRequest(ctx, url, req, MakeRequestConfig(0, 0, 0), respData)
	if uErr != nil {
		helpers.V115Log.Errorf("获取上传凭证失败: %v", uErr)
		return nil
	}
	return respData
}

// 上传分片
func (c *OpenClient) UploadResume(ctx context.Context, pickCode string, fileSize int, parentFileId string, fileSha1 string) {
}

func OssUploadFile(endPoint string, accessKeyId string, accessKeySecret string, securityToken string, bucketName string, objectId string, callback string, callbackVar string, filePath string, fileSize int64, fileSha1 string) (map[string]any, error) {
	cfg := oss.LoadDefaultConfig().
		WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKeyId, accessKeySecret, securityToken)).
		WithRegion("cn-shenzhen"). // 填写Bucket所在地域，以华东1（杭州）为例，Region填写为cn-hangzhou
		WithEndpoint(endPoint)     // 填写Bucket所在地域对应的公网Endpoint。以华东1（杭州）为例，Endpoint填写为'https://oss-cn-hangzhou.aliyuncs.com'
	// 创建OSS客户端
	client := oss.NewClient(cfg)
	// 将回调参数转换为JSON并进行Base64编码，以便将其作为回调参数传递
	callbackJson := map[string]string{}
	json.Unmarshal([]byte(callback), &callbackJson)
	// 定义回调参数
	callbackMap := map[string]string{
		"callbackUrl":      callbackJson["callbackUrl"],         // 设置回调服务器的URL，例如https://example.com:23450。
		"callbackBody":     callbackJson["callbackBody"],        // 设置回调请求体。
		"callbackBodyType": "application/x-www-form-urlencoded", //设置回调请求体类型。
	}
	callbackStr, err := json.Marshal(callbackMap)
	if err != nil {
		helpers.V115Log.Errorf("failed to marshal callback map: %v", err)
	}
	callbackVarJsonMap := map[string]string{}
	json.Unmarshal([]byte(callbackVar), &callbackVarJsonMap)
	callbackVarMap := make(map[string]string)
	callbackVarJsonMap["bucket"] = bucketName
	callbackVarJsonMap["object"] = objectId
	callbackVarJsonMap["size"] = helpers.Int64ToString(fileSize)
	callbackVarJsonMap["sha1"] = fileSha1
	body := callbackJson["callbackBody"]
	for k, v := range callbackVarJsonMap {
		callbackVarMap[k] = v
		key := fmt.Sprintf("${%s}", k)
		body = strings.ReplaceAll(body, key, v)
	}
	callbackVarStr, _ := json.Marshal(callbackVarMap)
	callbackBase64 := base64.StdEncoding.EncodeToString(callbackStr)
	callbackVarBase64 := base64.StdEncoding.EncodeToString(callbackVarStr)
	// 创建上传对象的请求
	putRequest := &oss.PutObjectRequest{
		Bucket:       oss.Ptr(bucketName),      // 存储空间名称
		Key:          oss.Ptr(objectId),        // 对象名称
		StorageClass: oss.StorageClassStandard, // 指定对象的存储类型为标准存储
		Acl:          oss.ObjectACLPrivate,     // 指定对象的访问权限为私有访问
		Callback:     oss.Ptr(callbackBase64),  // 填写回调参数
		CallbackVar:  oss.Ptr(callbackVarBase64),
	}
	// 执行上传对象的请求
	result, err := client.PutObjectFromFile(context.TODO(), putRequest, filePath)
	if err != nil {
		helpers.V115Log.Errorf("OSS上传失败： %v", err)
		return nil, err
	}
	// 打印上传对象的结果
	helpers.V115Log.Debug("115 OSS文件上传完成")
	return result.CallbackResult, nil
}
