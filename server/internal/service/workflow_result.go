package service

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// NodeArtifact 节点产物引用（不传文件正文）。
type NodeArtifact struct {
	Type  string `json:"type"` // file|image|doc|other
	Ref   string `json:"ref"`
	Label string `json:"label,omitempty"`
}

// NodeResult 节点交给下游的结构化结果（写入 output_json）。
type NodeResult struct {
	Status    string         `json:"status"`
	Summary   string         `json:"summary"`
	Result    string         `json:"result"`
	Artifacts []NodeArtifact `json:"artifacts"`
	// Text 兼容旧面板展示，不作为下游主交接字段
	Text string `json:"text,omitempty"`
}

// UpstreamHandoff 带上来源标记的直接上游结果。
type UpstreamHandoff struct {
	SourceNodeID string     `json:"source_node_id"`
	SourceType   string     `json:"source_type,omitempty"`
	Result       NodeResult `json:"result"`
}

const (
	nodeSummaryMax = 500
	nodeResultMax  = 2500
	nodeTextMax    = 4000
	maxArtifacts  = 20
)

var (
	reMDImage = regexp.MustCompile(`!\[[^\]]*\]\(([^)\s]+)`)
	reMDLink  = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+\.(?:png|jpe?g|gif|webp|pdf|md|txt|json|ya?ml|csv|html))`)
	// 匹配常见相对/绝对路径（避免吞掉整句）
	rePath = regexp.MustCompile(`(?:^|[\s"'(])((?:\./|\.\./|/)[A-Za-z0-9_./\-]+(?:\.[A-Za-z0-9]{1,8})?)`)
)

// BuildAgentNodeResult 从 Agent 正文压缩为可交接的节点结果。
func BuildAgentNodeResult(text, status string) NodeResult {
	text = strings.TrimSpace(text)
	if status == "" {
		status = inferPassFail(text)
	}
	if nr, ok := tryParseNodeResultJSON(text); ok {
		if nr.Status == "" {
			nr.Status = status
		}
		return normalizeNodeResult(nr, text)
	}
	summary := firstParagraph(text)
	if summary == "" {
		summary = truncate(text, nodeSummaryMax)
	} else {
		summary = truncate(summary, nodeSummaryMax)
	}
	return normalizeNodeResult(NodeResult{
		Status:    status,
		Summary:   summary,
		Result:    truncate(text, nodeResultMax),
		Artifacts: extractArtifacts(text),
		Text:      truncate(text, nodeTextMax),
	}, text)
}

// ParseNodeResult 解析 output_json（兼容旧 {text,status,summary}）。
func ParseNodeResult(raw string) NodeResult {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return NodeResult{Status: "PASS", Artifacts: []NodeArtifact{}}
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return NodeResult{
			Status: "PASS", Summary: truncate(raw, nodeSummaryMax),
			Result: truncate(raw, nodeResultMax), Artifacts: []NodeArtifact{}, Text: truncate(raw, nodeTextMax),
		}
	}
	nr := NodeResult{
		Status:  strings.TrimSpace(fmtString(m["status"])),
		Summary: strings.TrimSpace(fmtString(m["summary"])),
		Result:  strings.TrimSpace(fmtString(m["result"])),
		Text:    strings.TrimSpace(fmtString(m["text"])),
	}
	if nr.Status == "" {
		nr.Status = "PASS"
	}
	if nr.Result == "" {
		nr.Result = nr.Text
	}
	if nr.Summary == "" {
		nr.Summary = truncate(nr.Result, nodeSummaryMax)
	}
	nr.Artifacts = parseArtifactsField(m["artifacts"])
	return normalizeNodeResult(nr, nr.Text)
}

// CollectDirectUpstream 仅收集直接上游节点结果（并行汇入则多个）。
func CollectDirectUpstream(g *WorkflowGraph, nodeID string, outputs map[string]string) []UpstreamHandoff {
	if g == nil || outputs == nil {
		return nil
	}
	var out []UpstreamHandoff
	seen := map[string]bool{}
	for _, e := range g.ins[nodeID] {
		src := e.Source
		if src == "" || seen[src] {
			continue
		}
		seen[src] = true
		raw, ok := outputs[src]
		if !ok || strings.TrimSpace(raw) == "" {
			continue
		}
		st := ""
		if n, ok := g.Node(src); ok {
			st = n.Type
		}
		// start 节点通常无实质产出，跳过以免噪声
		if st == "start" {
			continue
		}
		out = append(out, UpstreamHandoff{
			SourceNodeID: src,
			SourceType:   st,
			Result:       ParseNodeResult(raw),
		})
	}
	return out
}

// FormatUpstreamForPrompt 生成下游交接块（含勿重复指令）。
func FormatUpstreamForPrompt(ups []UpstreamHandoff) string {
	if len(ups) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n【直接上游节点交接】\n")
	b.WriteString("以下为直接上游已完成工作与产出。请优先使用这些结果继续你的职责；")
	b.WriteString("不要重复执行上游已经完成的工作；仅当上游结果不足以完成本职时才允许补充执行。\n")
	for i, u := range ups {
		b.WriteString("\n--- 上游 #")
		b.WriteString(strconv.Itoa(i + 1))
		b.WriteString(" · 来源节点 `")
		b.WriteString(u.SourceNodeID)
		b.WriteString("`")
		if u.SourceType != "" {
			b.WriteString("（类型 ")
			b.WriteString(u.SourceType)
			b.WriteString("）")
		}
		b.WriteString(" ---\n")
		b.WriteString("status: ")
		b.WriteString(u.Result.Status)
		b.WriteString("\nsummary（完成了什么）: ")
		b.WriteString(u.Result.Summary)
		b.WriteString("\nresult（核心产出）: ")
		b.WriteString(u.Result.Result)
		if len(u.Result.Artifacts) > 0 {
			b.WriteString("\nartifacts（产物引用）:\n")
			for _, a := range u.Result.Artifacts {
				b.WriteString("- [")
				b.WriteString(a.Type)
				b.WriteString("] ")
				if a.Label != "" {
					b.WriteString(a.Label)
					b.WriteString(" · ")
				}
				b.WriteString(a.Ref)
				b.WriteString("\n")
			}
		} else {
			b.WriteString("\nartifacts: （无）\n")
		}
	}
	b.WriteString("\n【本节点要求】只完成你的职责范围内的增量工作，在结尾可用 status=PASS/FAIL 标明结论。\n")
	return b.String()
}

func normalizeNodeResult(nr NodeResult, fallbackText string) NodeResult {
	nr.Status = strings.TrimSpace(nr.Status)
	if nr.Status == "" {
		nr.Status = "PASS"
	}
	nr.Summary = truncate(strings.TrimSpace(nr.Summary), nodeSummaryMax)
	nr.Result = truncate(strings.TrimSpace(nr.Result), nodeResultMax)
	nr.Text = truncate(strings.TrimSpace(nr.Text), nodeTextMax)
	if nr.Text == "" {
		nr.Text = truncate(fallbackText, nodeTextMax)
	}
	if nr.Result == "" {
		nr.Result = truncate(nr.Text, nodeResultMax)
	}
	if nr.Summary == "" {
		nr.Summary = truncate(nr.Result, nodeSummaryMax)
	}
	if nr.Artifacts == nil {
		nr.Artifacts = []NodeArtifact{}
	}
	if len(nr.Artifacts) > maxArtifacts {
		nr.Artifacts = nr.Artifacts[:maxArtifacts]
	}
	return nr
}

func tryParseNodeResultJSON(text string) (NodeResult, bool) {
	// 尝试提取 ```json ... ``` 或首个 {...}
	s := text
	if i := strings.Index(s, "```json"); i >= 0 {
		rest := s[i+7:]
		if j := strings.Index(rest, "```"); j >= 0 {
			s = strings.TrimSpace(rest[:j])
		}
	} else if i := strings.Index(s, "{"); i >= 0 {
		if j := strings.LastIndex(s, "}"); j > i {
			s = s[i : j+1]
		}
	}
	var nr NodeResult
	if err := json.Unmarshal([]byte(s), &nr); err != nil {
		return NodeResult{}, false
	}
	if nr.Summary == "" && nr.Result == "" && nr.Status == "" && len(nr.Artifacts) == 0 {
		return NodeResult{}, false
	}
	return nr, true
}

func firstParagraph(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	for _, p := range strings.Split(text, "\n\n") {
		p = strings.TrimSpace(p)
		if p == "" || strings.HasPrefix(p, "```") {
			continue
		}
		return p
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func extractArtifacts(text string) []NodeArtifact {
	seen := map[string]bool{}
	var out []NodeArtifact
	add := func(typ, ref string) {
		ref = strings.TrimSpace(ref)
		if ref == "" || seen[ref] || len(out) >= maxArtifacts {
			return
		}
		seen[ref] = true
		out = append(out, NodeArtifact{Type: typ, Ref: truncate(ref, 300)})
	}
	for _, m := range reMDImage.FindAllStringSubmatch(text, -1) {
		add("image", m[1])
	}
	for _, m := range reMDLink.FindAllStringSubmatch(text, -1) {
		add("file", m[1])
	}
	for _, m := range rePath.FindAllStringSubmatch(text, -1) {
		ref := m[1]
		typ := "file"
		low := strings.ToLower(ref)
		if strings.HasSuffix(low, ".png") || strings.HasSuffix(low, ".jpg") || strings.HasSuffix(low, ".jpeg") || strings.HasSuffix(low, ".gif") || strings.HasSuffix(low, ".webp") {
			typ = "image"
		} else if strings.HasSuffix(low, ".md") || strings.HasSuffix(low, ".pdf") || strings.HasSuffix(low, ".doc") || strings.HasSuffix(low, ".docx") {
			typ = "doc"
		}
		add(typ, ref)
	}
	return out
}

func parseArtifactsField(v any) []NodeArtifact {
	if v == nil {
		return []NodeArtifact{}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return []NodeArtifact{}
	}
	var list []NodeArtifact
	if err := json.Unmarshal(b, &list); err != nil {
		return []NodeArtifact{}
	}
	return list
}

func fmtString(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return ""
		}
		s := string(b)
		if len(s) >= 2 && s[0] == '"' {
			return strings.Trim(s, `"`)
		}
		return s
	}
}
