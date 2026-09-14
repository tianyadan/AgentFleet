package auth
import "testing"
func TestAuthorized(t *testing.T) {
	roots := []string{
		"/Users/tianhaowen/Desktop/code_work_space/kfi-cloud-api",
		"/Users/tianhaowen/Desktop/code_work_space/kfi-cloud-admin",
	}
	cases := []struct{ cand string; ok bool }{
		{"/Users/tianhaowen/Desktop/code_work_space/kfi-cloud-api", true},
		{"/Users/tianhaowen/Desktop/code_work_space/kfi-cloud-admin/sub", true},
		{"/Users/tianhaowen/Desktop/code_work_space/kfi-cloud-vue", false},       // 未授权
		{"/Users/tianhaowen/Desktop/code_work_space/kfi-cloud-api/../secret", false}, // .. 越权
		{"../etc/passwd", false},
		{"/etc/passwd", false},
	}
	for _, c := range cases {
		_, ok := Authorized(c.cand, roots)
		if ok != c.ok {
			t.Errorf("Authorized(%q) = %v, want %v", c.cand, ok, c.ok)
		}
	}
}
