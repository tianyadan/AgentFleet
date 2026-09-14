package service

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"colleague-avatar/server/internal/store"
)

// imageExt 允许通过 file API 返回的图片扩展名。
var imageExt = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".bmp":  "image/bmp",
	".svg":  "image/svg+xml",
}

// AgentWorkspaceDir 智能体工作目录(自定义优先,否则全局根)。
func (s *Service) AgentWorkspaceDir(a *store.ManagedAgent) string {
	if a != nil {
		if ws := strings.TrimSpace(a.WorkspacePath); ws != "" {
			return ws
		}
	}
	return s.Cfg.WorkspaceRoot
}

// ResolveAgentImage 将相对/绝对路径解析到智能体工作区内的图片文件。
// 禁止目录穿越与非图片类型。
func (s *Service) ResolveAgentImage(a *store.ManagedAgent, raw string) (absPath, contentType string, err error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "file://")
	if raw == "" {
		return "", "", fmt.Errorf("path required")
	}
	root := s.AgentWorkspaceDir(a)
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", "", err
	}
	var candidate string
	if filepath.IsAbs(raw) {
		candidate = raw
	} else {
		candidate = filepath.Join(rootAbs, raw)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", "", err
	}
	rel, err := filepath.Rel(rootAbs, candidate)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", "", fmt.Errorf("path outside workspace")
	}
	ext := strings.ToLower(filepath.Ext(candidate))
	ct, ok := imageExt[ext]
	if !ok {
		return "", "", fmt.Errorf("not an image")
	}
	st, err := os.Stat(candidate)
	if err != nil {
		return "", "", err
	}
	if st.IsDir() {
		return "", "", fmt.Errorf("is a directory")
	}
	return candidate, ct, nil
}

// DetectImageContentType 补充探测(扩展名未知时)。
func DetectImageContentType(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return "application/octet-stream"
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	ct := http.DetectContentType(buf[:n])
	if strings.HasPrefix(ct, "image/") {
		return ct
	}
	return ""
}
