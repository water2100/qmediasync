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
	Name   string      `json:"name"`
	FileId json.Number `json:"cid"`
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
	delete(raw, "s")
	withoutSize, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(withoutSize, (*fileAlias)(f)); err != nil {
		return err
	}
	if len(size) == 0 || string(size) == "null" {
		return nil
	}
	var number json.Number
	if err = json.Unmarshal(size, &number); err != nil {
		return err
	}
	f.FileSize, err = strconv.ParseInt(number.String(), 10, 64)
	return err
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
	data := map[string]string{"aid": "1", "cid": fileId, "show_dir": "1"}
	if limit != 0 {
		data["limit"] = helpers.IntToString(limit)
	}
	if offset != 0 {
		data["offset"] = helpers.IntToString(offset)
	}
	// /files 始终列出当前目录；参数保留只是为了不破坏现有调用方。
	_ = showCur
	_ = onlyDir
	_ = showDir
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
	paths := make([]FileDetailPath, 0)
	for _, segment := range strings.Split(path, "/") {
		if segment == "" || segment == "." {
			continue
		}
		list, err := c.GetFsList(ctx, currentID, true, false, true, 0, 1150)
		if err != nil {
			return nil, err
		}
		var found *File
		for i := range list.Data {
			if list.Data[i].FileName == segment {
				found = &list.Data[i]
				break
			}
		}
		if found == nil {
			return nil, fmt.Errorf("115 文件不存在: %s", path)
		}
		currentID = found.FileId
		paths = append(paths, FileDetailPath{FileId: found.FileId, Name: found.FileName})
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("115 文件不存在: %s", path)
	}
	last := paths[len(paths)-1]
	return &FileDetail{FileId: last.FileId, FileName: last.Name, Paths: paths, Path: filepath.ToSlash(filepath.Join(strings.Split(path, "/")...))}, nil
}

// GetFsDetailByCid 使用 Cookie 文件详情接口。
func (c *OpenClient) GetFsDetailByCid(ctx context.Context, fileId string) (*FileDetail, error) {
	if fileId == "" {
		return nil, fmt.Errorf("fileId is empty")
	}
	req := c.client.R().SetQueryParams(map[string]string{"file_id": fileId}).SetMethod("GET")
	var data FileDetail
	_, _, err := c.doAuthRequest(ctx, fmt.Sprintf("%s/files/get_info", COOKIE_BASE_URL), req, MakeRequestConfig(3, 1, 60), &data)
	if err != nil {
		return nil, fmt.Errorf("调用115 Cookie文件详情接口失败: %w", err)
	}
	if data.FileId == "" {
		return nil, fmt.Errorf("115 返回空数据")
	}
	pathStr := make([]string, 0, len(data.Paths))
	for _, item := range data.Paths {
		if item.FileId != "0" {
			pathStr = append(pathStr, item.Name)
		}
	}
	data.Path = filepath.ToSlash(filepath.Join(pathStr...))
	return &data, nil
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
	var result MkDirData
	data := map[string]string{"cname": fileName, "pid": parentFileId}
	_, _, err := c.doAuthRequest(ctx, fmt.Sprintf("%s/files/add", COOKIE_BASE_URL), c.client.R().SetFormData(data).SetMethod("POST"), MakeRequestConfig(0, 0, 0), &result)
	if err != nil {
		return "", fmt.Errorf("新建115目录失败: %w", err)
	}
	if result.FileId == "" {
		return "", fmt.Errorf("115 新建目录响应缺少文件ID")
	}
	return result.FileId, nil
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

