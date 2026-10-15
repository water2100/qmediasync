package v115open

import (
	"Q115-STRM/internal/helpers"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

type FileType string

const (
	TypeFile FileType = "1"
	TypeDir  FileType = "0"
)

// UnmarshalJSON 兼容 115 将 fc 返回为数字或字符串的两种格式。
func (f *FileType) UnmarshalJSON(data []byte) error {
	var stringValue string
	if err := json.Unmarshal(data, &stringValue); err == nil {
		*f = FileType(stringValue)
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(data, &number); err != nil {
		return err
	}
	*f = FileType(number.String())
	return nil
}

type FileParentPath struct {
	Name   string `json:"name"`
	FileId string `json:"cid"`
}

// UnmarshalJSON keeps the Cookie API boundary tolerant: 115 may return the
// same identifier as either a JSON number or a JSON string.
func (p *FileParentPath) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	var err error
	p.Name, err = parse115String(raw["name"])
	if err != nil {
		return fmt.Errorf("解析路径名称失败: %w", err)
	}
	p.FileId, err = parse115String(raw["cid"])
	if err != nil {
		return fmt.Errorf("解析路径ID失败: %w", err)
	}
	return nil
}

type FileDetailPath struct {
	FileId string `json:"file_id"`
	Name   string `json:"file_name"`
}

// File 统一承接原 OpenAPI 与 Cookie /files 接口的字段。
// Cookie 接口中目录用 cid、文件用 fid，文件名为 n、大小为 s。
type File struct {
	FileId       string      `json:"fid"`
	Cid          string      `json:"cid"`
	Aid          string      `json:"aid"`
	Pid          string      `json:"pid"`
	FileCategory FileType    `json:"fc"`
	FileName     string      `json:"n"`
	PickCode     string      `json:"pc"`
	Utime        int64       `json:"t"`
	Ptime        int64       `json:"tp"`
	Sha1         string      `json:"sha1"`
	FileSize     int64       `json:"s"`
	Fta          string      `json:"fta"`
	Ico          string      `json:"ico"`
	VImg         string      `json:"v_img"`
	Thumbnail    string      `json:"thumb"`
	Uo           string      `json:"uo"`
	PlayLong     json.Number `json:"play_long"`
}

// UnmarshalJSON 兼容 115 在不同接口/版本中把文件大小返回为字符串或数字的差异。
func (f *File) UnmarshalJSON(data []byte) error {
	type fileAlias File
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	size := raw["s"]
	utime := raw["t"]
	ptime := raw["tp"]
	stringFields := map[string]*string{
		"fid": &f.FileId, "cid": &f.Cid, "aid": &f.Aid, "pid": &f.Pid,
		"n": &f.FileName, "pc": &f.PickCode, "sha": &f.Sha1, "fta": &f.Fta,
		"ico": &f.Ico, "v_img": &f.VImg, "thumb": &f.Thumbnail, "uo": &f.Uo,
	}
	delete(raw, "s")
	delete(raw, "t")
	delete(raw, "tp")
	playLong := raw["play_long"]
	delete(raw, "play_long")
	for key, target := range stringFields {
		value := raw[key]
		delete(raw, key)
		var parseErr error
		*target, parseErr = parse115String(value)
		if parseErr != nil {
			return fmt.Errorf("解析文件字段 %s 失败: %w", key, parseErr)
		}
	}
	withoutSize, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(withoutSize, (*fileAlias)(f)); err != nil {
		return err
	}
	if f.FileSize, err = parse115Int64(size); err != nil {
		return fmt.Errorf("解析文件大小失败: %w", err)
	}
	if f.Utime, err = parse115Int64(utime); err != nil {
		return fmt.Errorf("解析文件更新时间失败: %w", err)
	}
	f.Ptime, err = parse115Int64(ptime)
	if err != nil {
		return fmt.Errorf("解析文件创建时间失败: %w", err)
	}
	playLongString, err := parse115String(playLong)
	if err != nil {
		return fmt.Errorf("解析文件时长失败: %w", err)
	}
	f.PlayLong = json.Number(playLongString)
	return nil
}

func parse115Int64(raw json.RawMessage) (int64, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, nil
	}
	var stringValue string
	if err := json.Unmarshal(raw, &stringValue); err == nil {
		if stringValue == "" {
			return 0, nil
		}
		return strconv.ParseInt(stringValue, 10, 64)
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err != nil {
		return 0, err
	}
	return strconv.ParseInt(number.String(), 10, 64)
}

func parse115String(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var stringValue string
	if err := json.Unmarshal(raw, &stringValue); err == nil {
		return stringValue, nil
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err != nil {
		return "", err
	}
	return number.String(), nil
}

type FileListResp struct {
	RespBaseBool[[]File]
	Count    int              `json:"count"`
	SysCount int              `json:"sys_count"`
	Limit    json.Number      `json:"limit"`
	Offset   json.Number      `json:"offset"`
	Path     []FileParentPath `json:"path"`
	PathStr  string           `json:"path_str"`
}

// UnmarshalJSON is deliberately the only decoding boundary for /files.
// 115's Cookie endpoints do not keep a stable JSON scalar type: ids, counts,
// and pagination values can all change between a number and a string.  Decode
// them explicitly here so callers always receive the stable application model.
func (r *FileListResp) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	state, err := parse115Bool(raw["state"])
	if err != nil {
		return fmt.Errorf("解析列表状态失败: %w", err)
	}
	code, err := parse115Int(raw["code"])
	if err != nil {
		return fmt.Errorf("解析列表状态码失败: %w", err)
	}
	message, err := parse115String(raw["message"])
	if err != nil {
		return fmt.Errorf("解析列表消息失败: %w", err)
	}
	count, err := parse115Int(raw["count"])
	if err != nil {
		return fmt.Errorf("解析列表数量失败: %w", err)
	}
	sysCount, err := parse115Int(raw["sys_count"])
	if err != nil {
		return fmt.Errorf("解析系统文件数量失败: %w", err)
	}
	limit, err := parse115String(raw["limit"])
	if err != nil {
		return fmt.Errorf("解析列表页大小失败: %w", err)
	}
	offset, err := parse115String(raw["offset"])
	if err != nil {
		return fmt.Errorf("解析列表偏移失败: %w", err)
	}

	var files []File
	if value := raw["data"]; len(value) > 0 && string(value) != "null" {
		if err := json.Unmarshal(value, &files); err != nil {
			return fmt.Errorf("解析列表项目失败: %w", err)
		}
	}
	var paths []FileParentPath
	if value := raw["path"]; len(value) > 0 && string(value) != "null" {
		if err := json.Unmarshal(value, &paths); err != nil {
			return fmt.Errorf("解析列表路径失败: %w", err)
		}
	}
	pathStr, err := parse115String(raw["path_str"])
	if err != nil {
		return fmt.Errorf("解析列表路径文本失败: %w", err)
	}

	r.RespBaseBool = RespBaseBool[[]File]{State: state, Code: code, Message: message, Data: files}
	r.Count = count
	r.SysCount = sysCount
	r.Limit = json.Number(limit)
	r.Offset = json.Number(offset)
	r.Path = paths
	r.PathStr = pathStr
	return nil
}

func parse115Int(raw json.RawMessage) (int, error) {
	value, err := parse115Int64(raw)
	if err != nil {
		return 0, err
	}
	if int64(int(value)) != value {
		return 0, fmt.Errorf("数值超出 int 范围: %d", value)
	}
	return int(value), nil
}

func parse115Bool(raw json.RawMessage) (bool, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return false, nil
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err == nil {
		return value, nil
	}
	text, err := parse115String(raw)
	if err != nil {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "1", "true":
		return true, nil
	case "0", "false", "":
		return false, nil
	default:
		return false, fmt.Errorf("无效布尔值 %q", text)
	}
}

type FileDetail struct {
	Count        json.Number      `json:"count"`
	FileSize     string           `json:"size"`
	FileSizeByte int64            `json:"size_byte"`
	FolderCount  json.Number      `json:"folder_number"`
	PlayLong     json.Number      `json:"play_long"`
	ShowPlayLong json.Number      `json:"show_play_long"`
	Ptime        string           `json:"ptime"`
	Utime        string           `json:"utime"`
	FileName     string           `json:"file_name"`
	PickCode     string           `json:"pick_code"`
	Sha1         string           `json:"sha1"`
	FileId       string           `json:"file_id"`
	OpenTime     json.Number      `json:"open_time"`
	IsMark       string           `json:"is_mark"`
	FileCategory FileType         `json:"file_category"`
	Paths        []FileDetailPath `json:"paths"`
	Path         string           `json:"path"`
}

type MkDirData struct {
	FileName string `json:"file_name"`
	FileId   string `json:"file_id"`
}

func (d *FileDetail) GetFullPath() string {
	baseDir := make([]string, 0, len(d.Paths))
	for _, item := range d.Paths {
		if item.FileId == "0" {
			baseDir = append(baseDir, "")
		} else {
			baseDir = append(baseDir, item.Name)
		}
	}
	return strings.Join(baseDir, "/")
}

// GetFsList 使用原生 Cookie 的 webapi /files，不再访问 OpenAPI。
func (c *OpenClient) GetFsList(ctx context.Context, fileId string, showCur bool, onlyDir bool, showDir bool, offset int, limit int) (*FileListResp, error) {
	data := cookieFileListParams(fileId, showCur, onlyDir, showDir, offset, limit)
	req := c.client.R().SetQueryParams(data).SetMethod("GET")
	_, respBytes, err := c.doAuthRequest(ctx, fmt.Sprintf("%s/files", COOKIE_BASE_URL), req, MakeRequestConfig(3, 1, 15), nil)
	if err != nil {
		return nil, fmt.Errorf("调用115 Cookie文件列表接口失败: %w", err)
	}
	respData := FileListResp{}
	if err := json.Unmarshal(respBytes, &respData); err != nil {
		return nil, fmt.Errorf("解析115 Cookie文件列表响应失败: %w", err)
	}
	normalizeCookieFileList(&respData)
	pathStr := make([]string, 0, len(respData.Path))
	for _, item := range respData.Path {
		if item.FileId != "0" {
			pathStr = append(pathStr, item.Name)
		}
	}
	respData.PathStr = filepath.ToSlash(filepath.Join(pathStr...))
	return &respData, nil
}

func cookieFileListParams(fileId string, showCur bool, onlyDir bool, showDir bool, offset int, limit int) map[string]string {
	data := map[string]string{"aid": "1", "cid": fileId}
	if showCur {
		data["cur"] = "1"
	} else {
		data["cur"] = "0"
	}
	if onlyDir {
		data["nf"] = "1"
		data["show_dir"] = "1"
	} else if showDir {
		data["show_dir"] = "1"
	}
	if limit != 0 {
		data["limit"] = helpers.IntToString(limit)
	}
	if offset != 0 {
		data["offset"] = helpers.IntToString(offset)
	}
	return data
}

func normalizeCookieFileList(respData *FileListResp) {
	for i := range respData.Data {
		if respData.Data[i].FileId == "" {
			respData.Data[i].FileId = respData.Data[i].Cid
			respData.Data[i].FileCategory = TypeDir
		}
	}
}

// GetFsDetailByPath 通过逐层列目录解析路径，避免旧 /open/folder/get_info。
func (c *OpenClient) GetFsDetailByPath(ctx context.Context, path string) (*FileDetail, error) {
	path = strings.Trim(strings.ReplaceAll(path, "\\", "/"), "/")
	if path == "" || path == "." {
		return nil, fmt.Errorf("不能查询根目录的详情")
	}
	currentID := "0"
	paths := []FileDetailPath{{FileId: "0"}}
	segments := strings.Split(path, "/")
	var lastFile File
	for segmentIndex, segment := range segments {
		if segment == "" || segment == "." {
			continue
		}
		var found *File
		for offset := 0; ; {
			list, err := c.GetFsList(ctx, currentID, true, false, true, offset, 1150)
			if err != nil {
				return nil, err
			}
			for i := range list.Data {
				if list.Data[i].FileName == segment {
					found = &list.Data[i]
					break
				}
			}
			if found != nil || len(list.Data) < 1150 || offset+len(list.Data) >= list.Count {
				break
			}
			offset += len(list.Data)
		}
		if found == nil {
			return nil, fmt.Errorf("115 文件不存在: %s", path)
		}
		if segmentIndex < len(segments)-1 && found.FileCategory != TypeDir {
			return nil, fmt.Errorf("115 路径中包含文件: %s", path)
		}
		currentID = found.FileId
		lastFile = *found
		paths = append(paths, FileDetailPath{FileId: found.FileId, Name: found.FileName})
	}
	if len(paths) == 1 {
		return nil, fmt.Errorf("115 文件不存在: %s", path)
	}
	return &FileDetail{
		FileId: lastFile.FileId, FileName: lastFile.FileName, FileCategory: lastFile.FileCategory,
		FileSizeByte: lastFile.FileSize, PickCode: lastFile.PickCode, Sha1: lastFile.Sha1,
		Ptime: strconv.FormatInt(lastFile.Ptime, 10), Utime: strconv.FormatInt(lastFile.Utime, 10),
		Paths: paths, Path: filepath.ToSlash(filepath.Join(strings.Split(path, "/")...)),
	}, nil
}

// GetFsDetailByCid 使用 Cookie 文件详情接口。
func (c *OpenClient) GetFsDetailByCid(ctx context.Context, fileId string) (*FileDetail, error) {
	if fileId == "" {
		return nil, fmt.Errorf("fileId is empty")
	}
	req := c.client.R().SetQueryParams(map[string]string{"file_id": fileId}).SetMethod("GET")
	var info StructOrArray[File]
	_, _, err := c.doAuthRequest(ctx, fmt.Sprintf("%s/files/get_info", COOKIE_BASE_URL), req, MakeRequestConfig(3, 1, 60), &info)
	if err != nil {
		return nil, fmt.Errorf("调用115 Cookie文件详情接口失败: %w", err)
	}
	var file File
	if info.Value != nil {
		file = *info.Value
	} else if len(info.Array) > 0 {
		file = info.Array[0]
	} else {
		return nil, fmt.Errorf("115 返回空数据")
	}
	if file.FileId == "" {
		file.FileId = file.Cid
	}
	if file.FileId == "" {
		return nil, fmt.Errorf("115 文件详情缺少 ID")
	}
	pathID := fileId
	if file.FileCategory != TypeDir {
		pathID = file.Cid
	}
	list, err := c.GetFsList(ctx, pathID, true, false, true, 0, 1)
	if err != nil {
		return nil, fmt.Errorf("获取115目录路径失败: %w", err)
	}
	paths := make([]FileDetailPath, 0, len(list.Path))
	pathNames := make([]string, 0, len(list.Path))
	for _, item := range list.Path {
		paths = append(paths, FileDetailPath{FileId: item.FileId, Name: item.Name})
		if item.FileId != "0" {
			pathNames = append(pathNames, item.Name)
		}
	}
	if file.FileCategory != TypeDir {
		paths = append(paths, FileDetailPath{FileId: file.FileId, Name: file.FileName})
		pathNames = append(pathNames, file.FileName)
	}
	return &FileDetail{
		FileId: file.FileId, FileName: file.FileName, FileCategory: file.FileCategory,
		FileSizeByte: file.FileSize, PickCode: file.PickCode, Sha1: file.Sha1,
		Ptime: strconv.FormatInt(file.Ptime, 10), Utime: strconv.FormatInt(file.Utime, 10),
		Paths: paths, Path: filepath.ToSlash(filepath.Join(pathNames...)),
	}, nil
}

func (c *OpenClient) ReName(ctx context.Context, fileId string, newName string) (bool, error) {
	return c.cookieMutation(ctx, "/files/batch_rename", map[string]string{fmt.Sprintf("files_new_name[%s]", fileId): newName})
}

func (c *OpenClient) Move(ctx context.Context, fileIds []string, toFileId string) (bool, error) {
	data := indexedFileIDs(fileIds)
	data["pid"] = toFileId
	return c.cookieMutation(ctx, "/files/move", data)
}

func (c *OpenClient) Copy(ctx context.Context, fileIds []string, toFileId string, overwrite bool) (bool, error) {
	data := indexedFileIDs(fileIds)
	data["pid"] = toFileId
	if !overwrite {
		data["nodupli"] = "1"
	}
	return c.cookieMutation(ctx, "/files/copy", data)
}

func (c *OpenClient) Del(ctx context.Context, fileIds []string, parentFileId string) (bool, error) {
	data := indexedFileIDs(fileIds)
	data["ignore_warn"] = "1"
	return c.cookieMutation(ctx, "/rb/delete", data)
}

func (c *OpenClient) MkDir(ctx context.Context, parentFileId string, fileName string) (string, error) {
	data := map[string]string{"cname": fileName, "pid": parentFileId}
	_, body, err := c.doAuthRequest(ctx, fmt.Sprintf("%s/files/add", COOKIE_BASE_URL), c.client.R().SetFormData(data).SetMethod("POST"), MakeRequestConfig(0, 0, 0), nil)
	if err != nil {
		return "", fmt.Errorf("新建115目录失败: %w", err)
	}
	fileID, err := cookieCreatedFolderID(body)
	if err != nil {
		return "", fmt.Errorf("解析115新建目录响应失败: %w", err)
	}
	if fileID == "" {
		return "", fmt.Errorf("115 新建目录响应缺少文件ID")
	}
	return fileID, nil
}

func cookieCreatedFolderID(body []byte) (string, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return "", err
	}
	// /files/add returns cid at the response top level, unlike /open/folder/add.
	for _, key := range []string{"cid", "file_id"} {
		if id, err := parse115String(root[key]); err != nil || id != "" {
			return id, err
		}
	}
	var data map[string]json.RawMessage
	if err := json.Unmarshal(root["data"], &data); err != nil {
		return "", nil
	}
	for _, key := range []string{"cid", "file_id"} {
		if id, err := parse115String(data[key]); err != nil || id != "" {
			return id, err
		}
	}
	return "", nil
}

func indexedFileIDs(fileIDs []string) map[string]string {
	data := make(map[string]string, len(fileIDs))
	for i, fileID := range fileIDs {
		data[fmt.Sprintf("fid[%d]", i)] = fileID
	}
	return data
}

func (c *OpenClient) cookieMutation(ctx context.Context, path string, data map[string]string) (bool, error) {
	_, _, err := c.doAuthRequest(ctx, fmt.Sprintf("%s%s", COOKIE_BASE_URL, path), c.client.R().SetFormData(data).SetMethod("POST"), MakeRequestConfig(0, 0, 0), nil)
	if err != nil {
		return false, err
	}
	return true, nil
}
