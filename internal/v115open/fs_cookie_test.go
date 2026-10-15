package v115open

import (
	"encoding/json"
	"testing"
)

func TestCookieFileListMaps115Fields(t *testing.T) {
	var response FileListResp
	if err := json.Unmarshal([]byte(`{"state":true,"count":2,"data":[{"cid":10,"n":"目录","aid":1,"pid":0,"t":"1","tp":2},{"fid":20,"n":"视频.mkv","fc":1,"pc":"pick","s":"42","t":"3","tp":4}]}`), &response); err != nil {
		t.Fatalf("解析 Cookie 文件列表失败: %v", err)
	}
	normalizeCookieFileList(&response)
	if response.Data[0].FileId != "10" || response.Data[0].FileCategory != TypeDir || response.Data[0].FileName != "目录" || response.Data[0].Aid != "1" || response.Data[0].Pid != "0" {
		t.Fatalf("目录字段映射错误: %#v", response.Data[0])
	}
	if response.Data[1].FileId != "20" || response.Data[1].FileName != "视频.mkv" || response.Data[1].FileSize != 42 || response.Data[1].Utime != 3 || response.Data[1].Ptime != 4 {
		t.Fatalf("文件字段映射错误: %#v", response.Data[1])
	}
}

