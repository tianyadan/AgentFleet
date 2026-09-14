package permission

import "testing"

func TestParseVerdict(t *testing.T) {
	cases := []struct {
		in     string
		allow  bool
		hasRea bool
		ok     bool
	}{
		{"ALLOW\n只读文件,无副作用", true, true, true},
		{"\n\nDENY\n会删除文件", false, true, true},
		{"allow\n小写也应识别", true, true, true},
		{"**ALLOW**\n带 markdown 加粗", true, true, true},
		{"结论: DENY\n越权访问", false, false, false}, // 首行非纯判定词 → 不识别
		{"", false, false, false},
		{"   \n  ", false, false, false},
		{"DENY", false, false, true},          // 无理由也可
		{"APPROVED\n可以", false, false, false}, // 不是 ALLOW 前缀精确匹配
	}
	for _, c := range cases {
		var v Verdict
		allow, reason, ok := parseVerdict(c.in, &v)
		if ok != c.ok {
			t.Errorf("parseVerdict(%q) ok=%v want %v", c.in, ok, c.ok)
			continue
		}
		if ok && allow != c.allow {
			t.Errorf("parseVerdict(%q) allow=%v want %v", c.in, allow, c.allow)
		}
		if c.hasRea && reason == "" {
			t.Errorf("parseVerdict(%q) 应解析出理由", c.in)
		}
	}
}

// 验证命令场景会额外解析出含义与风险。
func TestParseVerdictExplain(t *testing.T) {
	in := "ALLOW\n只读命令\n查询文件内容\nmid"
	var v Verdict
	allow, _, ok := parseVerdict(in, &v)
	if !ok || !allow {
		t.Fatalf("应识别 ALLOW")
	}
	if v.Meaning != "查询文件内容" {
		t.Errorf("meaning=%q want 查询文件内容", v.Meaning)
	}
	if v.Risk != "mid" {
		t.Errorf("risk=%q want mid", v.Risk)
	}
}
