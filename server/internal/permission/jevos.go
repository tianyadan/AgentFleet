package permission

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var errJevosDown = errors.New("jevos unavailable")

// JEVOSResult 本地 JEVOS 多问号 noul 汇总。
type JEVOSResult struct {
	Destructive    float64 `json:"destructive"`
	DataLoss       float64 `json:"data_loss"`
	ServiceImpact  float64 `json:"service_impact"`
	PermissionRisk float64 `json:"permission_risk"`
	CredentialRisk float64 `json:"credential_risk"`
	NeedsReview    float64 `json:"needs_review"`
	RiskScore      float64 `json:"risk_score"`
	Raw            any     `json:"raw,omitempty"`
}

// JevosClient 审核接口。
type JevosClient interface {
	Review(ctx context.Context, a ToolAction) (JEVOSResult, error)
}

type jevosHTTP struct {
	base    string
	timeout time.Duration
	client  *http.Client
}

// NewJevosClient 连接本地 jevos（POST /v1/systemone）。
func NewJevosClient(base string, timeoutSec int) JevosClient {
	if strings.TrimSpace(base) == "" {
		base = "http://127.0.0.1:8017"
	}
	if timeoutSec <= 0 {
		timeoutSec = 20
	}
	return &jevosHTTP{
		base:    strings.TrimRight(base, "/"),
		timeout: time.Duration(timeoutSec) * time.Second,
		client:  &http.Client{Timeout: time.Duration(timeoutSec) * time.Second},
	}
}

func (j *jevosHTTP) Review(ctx context.Context, a ToolAction) (JEVOSResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cctx, cancel := context.WithTimeout(ctx, j.timeout)
	defer cancel()
	// 问题显式要求结合 state 中的 agent_rules / agent_policy / conversation_context，
	// 避免仅凭命令字符串「直觉」否决已授权的正常开发操作。
	ctxHint := " Use agent_rules, agent_policy, and conversation_context in state. If the agent is authorized for this kind of work inside its workspace, answer low. Prefer human review over hard refusal when unsure."
	body := map[string]any{
		"model": "jev-latest",
		"state": a.JSONState(),
		"questions": map[string]any{
			"destructive":     noulQ("Would this command destroy data, disks, or the system beyond a reversible project change?" + ctxHint),
			"data_loss":       noulQ("Would this likely cause unrecoverable data loss?" + ctxHint),
			"service_impact":  noulQ("Would this stop, restart, or disrupt a running service outside the agent's authorized workspace?" + ctxHint),
			"permission_risk": noulQ("Would this change permissions, ownership, or security boundaries beyond what agent_policy allows?" + ctxHint),
			"credential_risk": noulQ("Would this expose, send, or modify credentials or secrets?" + ctxHint),
			"needs_review":    noulQ("Should a human review this before it runs? Answer high only if genuinely ambiguous; authorized routine project work should be low." + ctxHint),
		},
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(cctx, http.MethodPost, j.base+"/v1/systemone", bytes.NewReader(raw))
	if err != nil {
		return JEVOSResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := j.client.Do(req)
	if err != nil {
		return JEVOSResult{}, fmt.Errorf("%w: %v", errJevosDown, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return JEVOSResult{}, fmt.Errorf("%w: HTTP %d %s", errJevosDown, resp.StatusCode, truncate(string(b), 200))
	}
	var parsed struct {
		Answers map[string]struct {
			Noul float64 `json:"noul"`
		} `json:"answers"`
	}
	if err := json.Unmarshal(b, &parsed); err != nil {
		return JEVOSResult{}, fmt.Errorf("%w: %v", errJevosDown, err)
	}
	if parsed.Answers == nil {
		return JEVOSResult{}, fmt.Errorf("%w: empty answers", errJevosDown)
	}
	out := JEVOSResult{
		Destructive:    parsed.Answers["destructive"].Noul,
		DataLoss:       parsed.Answers["data_loss"].Noul,
		ServiceImpact:  parsed.Answers["service_impact"].Noul,
		PermissionRisk: parsed.Answers["permission_risk"].Noul,
		CredentialRisk: parsed.Answers["credential_risk"].Noul,
		NeedsReview:    parsed.Answers["needs_review"].Noul,
		Raw:            json.RawMessage(b),
	}
	out.RiskScore = maxNoul(out.Destructive, out.DataLoss, out.ServiceImpact, out.PermissionRisk, out.CredentialRisk, out.NeedsReview)
	return out, nil
}

func noulQ(instr string) map[string]any {
	return map[string]any{"type": "noul", "instructions": instr}
}

func maxNoul(xs ...float64) float64 {
	m := 0.0
	for _, x := range xs {
		if x > m {
			m = x
		}
	}
	return m
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// DecisionFromRisk 边界：<=0.45 ALLOW；>0.45 且 <0.60 REVIEW；>=0.60 DENY。
// 兼容旧单指标用法；Gateway 请用 DecisionFromJevos。
func DecisionFromRisk(score float64) string {
	if score <= 0.45 {
		return DecisionAllow
	}
	if score >= 0.60 {
		return DecisionDeny
	}
	return DecisionReview
}

// DecisionFromJevos 综合多问号结果：
// - needs_review 高分 → 人工（REVIEW），绝不因此直接 DENY；
// - 危害类问号均低且 needs_review 不高 → ALLOW；
// - 其余不确定 → REVIEW（弹人工），Hard Deny / 数字人策略层负责硬拒绝。
func DecisionFromJevos(jr JEVOSResult) string {
	harm := maxNoul(jr.Destructive, jr.DataLoss, jr.ServiceImpact, jr.PermissionRisk, jr.CredentialRisk)
	if harm <= 0.45 && jr.NeedsReview <= 0.50 {
		return DecisionAllow
	}
	return DecisionReview
}
