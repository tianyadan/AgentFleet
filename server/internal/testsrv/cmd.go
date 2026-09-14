package testsrv

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var containerRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`)

// BuildCommand 仅允许 docker ps / docker logs 模板。
func BuildCommand(action string, args map[string]interface{}) (string, error) {
	switch action {
	case "containers", "ps":
		return "docker ps --format '{{.ID}} {{.Names}} {{.Status}} {{.Image}}'", nil
	case "logs":
		cid, _ := args["container_id"].(string)
		cid = strings.TrimSpace(cid)
		if !containerRe.MatchString(cid) {
			return "", fmt.Errorf("非法 container_id")
		}
		n := 200
		switch v := args["tail"].(type) {
		case float64:
			n = int(v)
		case int:
			n = v
		case string:
			if x, err := strconv.Atoi(v); err == nil {
				n = x
			}
		}
		if n < 1 {
			n = 1
		}
		if n > 500 {
			n = 500
		}
		return fmt.Sprintf("docker logs --tail %d %s", n, cid), nil
	default:
		return "", fmt.Errorf("不支持的动作: %s(仅 containers/logs)", action)
	}
}
