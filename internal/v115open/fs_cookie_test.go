package v115open

import (
	"encoding/json"
	"strings"
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

func TestCookieFileListAcceptsMixedScalarTypes(t *testing.T) {
	// 115's Cookie API returns different scalar JSON types for the same fields
	// depending on account, endpoint version, and whether the item is a file or
	// folder. This fixture intentionally mixes every supported representation.
	const payload = `{
		"state":"1", "code":"200", "message":"ok", "count":"2",
		"sys_count":1, "limit":1150, "offset":"0",
		"path":[{"cid":0,"name":"根目录"},{"cid":"10","name":"目录"}],
		"data":[
			{"cid":"10","n":"目录","aid":1,"pid":"0","t":1,"tp":"2","fc":"0"},
			{"fid":20,"n":"视频.mkv","fc":1,"pc":123,"s":42,"t":"3","tp":4,"play_long":0}
		]
	}`
	var response FileListResp
	if err := json.Unmarshal([]byte(payload), &response); err != nil {
		t.Fatalf("混合标量响应不应解析失败: %v", err)
	}
	normalizeCookieFileList(&response)
	if !response.State || response.Code != 200 || response.Count != 2 || response.SysCount != 1 {
		t.Fatalf("列表顶层字段错误: %#v", response)
	}
	if response.Limit.String() != "1150" || response.Offset.String() != "0" || response.Path[1].FileId != "10" {
		t.Fatalf("分页或路径字段错误: %#v", response)
	}
	if got := response.Data[0]; got.FileId != "10" || got.Aid != "1" || got.Pid != "0" || got.Utime != 1 || got.Ptime != 2 || got.FileCategory != TypeDir {
		t.Fatalf("目录字段错误: %#v", got)
	}
	if got := response.Data[1]; got.FileId != "20" || got.PickCode != "123" || got.FileSize != 42 || got.Utime != 3 || got.Ptime != 4 || got.PlayLong.String() != "0" {
		t.Fatalf("文件字段错误: %#v", got)
	}
}

func TestCookieFileListRejectsInvalidScalarWithFieldContext(t *testing.T) {
	var response FileListResp
	err := json.Unmarshal([]byte(`{"state":true,"count":"not-a-number","data":[]}`), &response)
	if err == nil {
		t.Fatal("无效数量必须返回错误")
	}
	if !strings.Contains(err.Error(), "列表数量") {
		t.Fatalf("错误必须指出字段上下文，实际为: %v", err)
	}
}

func TestCookieResponseEnvelopeAcceptsWebAPIFields(t *testing.T) {
	response, err := parse115Response([]byte(`{"state":true,"errNo":0,"error":"","data":[{"cid":"10","n":"目录","t":"1","tp":"2","fc":0}]}`))
	if err != nil || !response.State || response.Code != 0 {
		t.Fatalf("Cookie 响应头解析失败: %#v, %v", response, err)
	}
	var detail StructOrArray[File]
	if err := json.Unmarshal(response.Data, &detail); err != nil {
		t.Fatalf("get_info 数组响应解析失败: %v", err)
	}
	if len(detail.Array) != 1 || detail.Array[0].Cid != "10" || detail.Array[0].FileName != "目录" {
		t.Fatalf("目录详情映射错误: %#v", detail)
	}

	failure, err := parse115Response([]byte(`{"state":false,"errNo":"401","error":"未登录","data":[]}`))
	if err != nil || failure.State || failure.Code != 401 || failure.Message != "未登录" {
		t.Fatalf("Cookie 业务错误解析失败: %#v, %v", failure, err)
	}
}

