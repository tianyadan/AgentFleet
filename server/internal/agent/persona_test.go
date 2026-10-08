package agent

import (
	"strings"
	"testing"
)

// 自称应为「{名字} 数字员工」。
func TestEmployeeDisplayName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", "数字员工"},
		{"小明", "小明 数字员工"},
		{"小明 数字员工", "小明 数字员工"},
	}
	for _, c := range cases {
		if got := EmployeeDisplayName(c.in); got != c.want {
			t.Fatalf("EmployeeDisplayName(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestSystemPromptUsesEmployeeNameNotEbotBrand(t *testing.T) {
	p := SystemPrompt(Persona{Name: "前台助理", Style: "简洁"}, []string{"/tmp/ws"})
	if !strings.Contains(p, "前台助理 数字员工") {
		t.Fatalf("expected employee display name in prompt, got: %s", p)
	}
	if strings.Contains(p, "E-bot") {
		t.Fatalf("prompt must not mention E-bot brand: %s", p)
	}
}
