package store

import "testing"

// CanControlAuto 会话归属是否允许该客户端开关自动审核。
func TestCanControlAuto(t *testing.T) {
	cases := []struct {
		owner, client string
		want          bool
	}{
		{"127.0.0.1", "127.0.0.1", true},
		{"admin", "127.0.0.1", true}, // 管理台智能体会话
		{"admin", "admin", true},
		{"10.0.0.1", "127.0.0.1", false},
		{"", "127.0.0.1", false},
		{"127.0.0.1", "", false},
	}
	for _, tc := range cases {
		got := CanControlAuto(tc.owner, tc.client)
		if got != tc.want {
			t.Fatalf("CanControlAuto(%q,%q)=%v want %v", tc.owner, tc.client, got, tc.want)
		}
	}
}
