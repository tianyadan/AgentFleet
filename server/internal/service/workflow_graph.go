package service

import (
	"encoding/json"
	"fmt"
	"strings"
)

// GraphNode Canvas 节点。
type GraphNode struct {
	ID   string         `json:"id"`
	Type string         `json:"type"`
	Data map[string]any `json:"data"`
}

// GraphEdge Canvas 边。
type GraphEdge struct {
	ID           string         `json:"id"`
	Source       string         `json:"source"`
	Target       string         `json:"target"`
	SourceHandle string         `json:"sourceHandle"`
	Data         map[string]any `json:"data"`
}

// WorkflowGraph 解析后的图。
type WorkflowGraph struct {
	Nodes    []GraphNode   `json:"nodes"`
	Edges    []GraphEdge   `json:"edges"`
	Settings map[string]any `json:"settings"`
	byID     map[string]GraphNode
	outs     map[string][]GraphEdge
	ins      map[string][]GraphEdge
}

// ParseWorkflowGraph 从 definition.graph_json 解析。
func ParseWorkflowGraph(raw string) (*WorkflowGraph, error) {
	var g WorkflowGraph
	if err := json.Unmarshal([]byte(raw), &g); err != nil {
		return nil, err
	}
	g.byID = map[string]GraphNode{}
	g.outs = map[string][]GraphEdge{}
	g.ins = map[string][]GraphEdge{}
	for _, n := range g.Nodes {
		t := strings.ToLower(strings.TrimSpace(n.Type))
		if t == "" {
			if n.Data != nil {
				t = strings.ToLower(fmt.Sprint(n.Data["nodeType"]))
			}
		}
		n.Type = t
		g.byID[n.ID] = n
	}
	// 重建 Nodes 带规范化 type
	norm := make([]GraphNode, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		norm = append(norm, g.byID[n.ID])
	}
	g.Nodes = norm
	for _, e := range g.Edges {
		g.outs[e.Source] = append(g.outs[e.Source], e)
		g.ins[e.Target] = append(g.ins[e.Target], e)
	}
	return &g, nil
}

// AllowAllCommands 编排设置：除 rm 外自动放行命令。
func (g *WorkflowGraph) AllowAllCommands() bool {
	if g == nil || g.Settings == nil {
		return false
	}
	raw, ok := g.Settings["allow_all_commands"]
	if !ok || raw == nil {
		return false
	}
	switch v := raw.(type) {
	case bool:
		return v
	case float64:
		return v != 0
	case string:
		s := strings.TrimSpace(strings.ToLower(v))
		return s == "1" || s == "true" || s == "yes" || s == "on"
	default:
		return false
	}
}

func (g *WorkflowGraph) StartNode() (GraphNode, error) {
	for _, n := range g.Nodes {
		if n.Type == "start" {
			return n, nil
		}
	}
	return GraphNode{}, fmt.Errorf("missing start node")
}

func (g *WorkflowGraph) Node(id string) (GraphNode, bool) {
	n, ok := g.byID[id]
	return n, ok
}

func (g *WorkflowGraph) OutEdges(id string) []GraphEdge {
	return g.outs[id]
}

func (g *WorkflowGraph) Successors(id string) []string {
	var ids []string
	for _, e := range g.outs[id] {
		ids = append(ids, e.Target)
	}
	return ids
}

func (g *WorkflowGraph) DataString(n GraphNode, key string) string {
	if n.Data == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(n.Data[key]))
}

// DataBool 读取节点配置布尔值（兼容 true/1/"true"/"1"）。
func (g *WorkflowGraph) DataBool(n GraphNode, key string) bool {
	if n.Data == nil {
		return false
	}
	raw, ok := n.Data[key]
	if !ok || raw == nil {
		return false
	}
	switch v := raw.(type) {
	case bool:
		return v
	case float64:
		return v != 0
	case int:
		return v != 0
	case int64:
		return v != 0
	case string:
		s := strings.TrimSpace(strings.ToLower(v))
		return s == "1" || s == "true" || s == "yes" || s == "on"
	default:
		s := strings.TrimSpace(strings.ToLower(fmt.Sprint(v)))
		return s == "1" || s == "true" || s == "yes" || s == "on"
	}
}

func (g *WorkflowGraph) DataInt64(n GraphNode, key string) int64 {
	if n.Data == nil {
		return 0
	}
	switch v := n.Data[key].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	case string:
		var n int64
		fmt.Sscanf(v, "%d", &n)
		return n
	default:
		var n int64
		fmt.Sscanf(fmt.Sprint(v), "%d", &n)
		return n
	}
}

func (g *WorkflowGraph) DataStringSlice(n GraphNode, key string) []string {
	if n.Data == nil {
		return nil
	}
	raw, ok := n.Data[key]
	if !ok || raw == nil {
		return nil
	}
	switch t := raw.(type) {
	case []any:
		var out []string
		for _, x := range t {
			s := strings.TrimSpace(fmt.Sprint(x))
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return t
	default:
		return nil
	}
}

// EdgeBranch true/false/default。
func EdgeBranch(e GraphEdge) string {
	h := strings.ToLower(strings.TrimSpace(e.SourceHandle))
	if h == "true" || h == "false" || h == "reject" || h == "approve" {
		return h
	}
	if e.Data != nil {
		b := strings.ToLower(strings.TrimSpace(fmt.Sprint(e.Data["branch"])))
		if b != "" {
			return b
		}
	}
	return "default"
}

// FlattenOutput 把 output_json 展平为条件上下文。
func FlattenOutput(outputJSON string) map[string]any {
	out := map[string]any{}
	if strings.TrimSpace(outputJSON) == "" {
		return out
	}
	var m map[string]any
	if json.Unmarshal([]byte(outputJSON), &m) == nil {
		for k, v := range m {
			out[k] = v
		}
		if t, ok := m["text"].(string); ok {
			out["summary"] = t
			out["text"] = t
		}
		return out
	}
	out["text"] = outputJSON
	out["summary"] = outputJSON
	return out
}
