package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// EnsureCursorCompactHook 在工作区写入/合并 preCompact → avatar-hook，便于自动压缩同步。
func EnsureCursorCompactHook(dir, hookBin, backend string, convID int64) {
	dir = strings.TrimSpace(dir)
	hookBin = strings.TrimSpace(hookBin)
	if dir == "" || hookBin == "" || convID <= 0 {
		return
	}
	hookDir := filepath.Join(dir, ".cursor")
	_ = os.MkdirAll(hookDir, 0o755)
	path := filepath.Join(hookDir, "hooks.json")

	root := map[string]interface{}{"version": 1, "hooks": map[string]interface{}{}}
	if raw, err := os.ReadFile(path); err == nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, &root)
	}
	hooks, _ := root["hooks"].(map[string]interface{})
	if hooks == nil {
		hooks = map[string]interface{}{}
		root["hooks"] = hooks
	}
	wrapper := filepath.Join(hookDir, "avatar-precompact.sh")
	script := "#!/bin/sh\n" +
		"export AVATAR_CONV_ID=" + strconv.FormatInt(convID, 10) + "\n" +
		"export AVATAR_BACKEND_URL=" + shellSingleQuote(backend) + "\n" +
		"exec " + shellSingleQuote(hookBin) + "\n"
	_ = os.WriteFile(wrapper, []byte(script), 0o755)

	list, _ := hooks["preCompact"].([]interface{})
	for _, it := range list {
		m, _ := it.(map[string]interface{})
		if m != nil && m["command"] == wrapper {
			return
		}
	}
	list = append(list, map[string]interface{}{"command": wrapper, "timeout": 30})
	hooks["preCompact"] = list
	root["version"] = 1
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, out, 0o644)
}

// EnsureCodexCompactHooks 在工作区写入 Codex Pre/PostCompact hook（若 Codex 读取项目 hooks）。
func EnsureCodexCompactHooks(dir, hookBin, backend string, convID int64) {
	dir = strings.TrimSpace(dir)
	hookBin = strings.TrimSpace(hookBin)
	if dir == "" || hookBin == "" || convID <= 0 {
		return
	}
	hookDir := filepath.Join(dir, ".codex")
	_ = os.MkdirAll(hookDir, 0o755)
	wrapper := filepath.Join(hookDir, "avatar-compact.sh")
	script := "#!/bin/sh\n" +
		"export AVATAR_CONV_ID=" + strconv.FormatInt(convID, 10) + "\n" +
		"export AVATAR_BACKEND_URL=" + shellSingleQuote(backend) + "\n" +
		"exec " + shellSingleQuote(hookBin) + "\n"
	_ = os.WriteFile(wrapper, []byte(script), 0o755)

	path := filepath.Join(hookDir, "hooks.json")
	doc := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreCompact": []interface{}{
				map[string]interface{}{
					"hooks": []interface{}{
						map[string]interface{}{"type": "command", "command": wrapper, "timeout": 30},
					},
				},
			},
			"PostCompact": []interface{}{
				map[string]interface{}{
					"hooks": []interface{}{
						map[string]interface{}{"type": "command", "command": wrapper, "timeout": 30},
					},
				},
			},
		},
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, out, 0o644)
}

func shellSingleQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
