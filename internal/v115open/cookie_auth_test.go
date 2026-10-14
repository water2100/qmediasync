package v115open

import (
	"testing"
)

func TestDecodeNativeQrToken(t *testing.T) {
	token, err := decodeNativeQrToken([]byte(`{"state":true,"data":{"uid":"uid-1","time":1710000000,"sign":"sign-1"}}`))
	if err != nil {
		t.Fatalf("解析二维码令牌失败: %v", err)
	}
	if token.UID != "uid-1" || token.Time != "1710000000" || token.Sign != "sign-1" {
		t.Fatalf("二维码令牌不正确: %+v", token)
	}
}

func TestParseNativeQrStatus(t *testing.T) {
	cases := []struct {
		body string
		want NativeQrStatus
	}{
		{`{"state":true,"data":{"status":0}}`, NativeQrStatusWaiting},
		{`{"state":true,"data":{"status":1}}`, NativeQrStatusScanned},
		{`{"state":true,"data":{"status":2}}`, NativeQrStatusConfirmed},
		{`{"state":true,"data":{"status":-1}}`, NativeQrStatusExpired},
		{`{"state":true,"data":{"status":-2}}`, NativeQrStatusCanceled},
	}
	for _, tc := range cases {
		got, err := parseNativeQrStatus([]byte(tc.body))
		if err != nil {
			t.Fatalf("解析状态失败: %v", err)
		}
		if got != tc.want {
			t.Fatalf("状态不正确: got %v, want %v", got, tc.want)
		}
	}
}

func TestNormalizeNativeCookie(t *testing.T) {
	cookie, err := normalizeNativeCookie(map[string]any{
		"UID":  "uid",
		"CID":  "cid",
		"SEID": "seid",
		"KID":  "kid",
	})
	if err != nil {
		t.Fatalf("规范化 Cookie 失败: %v", err)
	}
	const want = "UID=uid; CID=cid; SEID=seid; KID=kid"
	if cookie != want {
		t.Fatalf("Cookie 不正确: got %q, want %q", cookie, want)
	}
}

func TestNormalizeNativeCookieRejectsMissingRequiredValue(t *testing.T) {
	_, err := normalizeNativeCookie(map[string]any{
		"UID":  "uid",
		"CID":  "cid",
		"SEID": "seid",
	})
	if err == nil {
		t.Fatal("缺少 KID 时应返回错误")
	}
}
