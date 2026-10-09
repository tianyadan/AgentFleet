package permission

import (
	"context"
	"fmt"
)

// PermissionDecision Gateway 统一输出。
type PermissionDecision struct {
	Decision          string
	RiskScore         float64
	Reason            string
	DecidedBy         string
	RuleID            string
	JEVOSResult       any
	SessionAuthorized bool
}

// EvaluateOpts 策略与编排例外（仍不能覆盖 Hard Deny）。
type EvaluateOpts struct {
	Policy          *AgentPolicy
	AllowAllExceptRm bool
	SkipJevos        bool
}

// Gateway 平台权限网关：Hard Deny → Safe Allow → Session → JEVOS → REVIEW。
type Gateway struct {
	Sessions SessionStore
	Jevos    JevosClient
}

func NewGateway(ss SessionStore, j JevosClient) *Gateway {
	return &Gateway{Sessions: ss, Jevos: j}
}

// Evaluate 五层决策。JEVOS 失败一律 REVIEW。
func (g *Gateway) Evaluate(ctx context.Context, a ToolAction, opt EvaluateOpts) PermissionDecision {
	if ctx == nil {
		ctx = context.Background()
	}
	if IsHardDeny(a) {
		return PermissionDecision{Decision: DecisionDeny, Reason: "命中平台 Hard Deny", DecidedBy: DecidedByStatic, RuleID: staticRuleID(a, true)}
	}
	if opt.Policy != nil {
		if d := applyPolicyAction(a, *opt.Policy); d.Decision == DecisionDeny {
			return d
		}
		if opt.Policy.AllowNetwork && (a.ActionType == ActionNetworkRead || a.ToolName == "WebFetch" || a.ToolName == "WebSearch") {
			return PermissionDecision{Decision: DecisionAllow, Reason: "数字员工已授权联网", DecidedBy: DecidedByStatic, RuleID: "agent_policy_network"}
		}
	}
	if IsSafeAllow(a) {
		return PermissionDecision{Decision: DecisionAllow, Reason: "静态只读安全规则", DecidedBy: DecidedByStatic, RuleID: staticRuleID(a, false)}
	}
	if opt.AllowAllExceptRm && a.ActionType != ActionFileDelete && !LooksLikeRm("Bash", a.Args) {
		return PermissionDecision{Decision: DecisionAllow, Reason: "编排允许全部命令(除rm)", DecidedBy: DecidedByStatic, RuleID: "workflow_allow_all"}
	}
	if g.Sessions != nil && a.ConversationID > 0 {
		ok, err := g.Sessions.Match(a.ConversationID, a.Signature())
		if err == nil && ok {
			return PermissionDecision{Decision: DecisionAllow, Reason: "会话已授权相同 signature", DecidedBy: DecidedBySession, SessionAuthorized: true}
		}
	}
	if !a.SideEffecting() {
		return PermissionDecision{Decision: DecisionReview, Reason: "静态规则无法确定，需人工确认", DecidedBy: DecidedByStatic, RuleID: "unknown_readonly"}
	}
	if opt.SkipJevos || g.Jevos == nil {
		return PermissionDecision{Decision: DecisionReview, Reason: "有副作用且未走 JEVOS，需人工确认", DecidedBy: DecidedByJevos, RuleID: "jevos_skipped"}
	}
	jr, err := g.Jevos.Review(ctx, a)
	if err != nil {
		return PermissionDecision{Decision: DecisionReview, Reason: "JEVOS 不可用，转人工: " + err.Error(), DecidedBy: DecidedByJevos, RuleID: "jevos_error"}
	}
	dec := DecisionFromJevos(jr)
	harm := maxNoul(jr.Destructive, jr.DataLoss, jr.ServiceImpact, jr.PermissionRisk, jr.CredentialRisk)
	reason := fmt.Sprintf("JEVOS harm=%.2f needs_review=%.2f → %s", harm, jr.NeedsReview, dec)
	return PermissionDecision{
		Decision: dec, RiskScore: jr.RiskScore, Reason: reason,
		DecidedBy: DecidedByJevos, RuleID: "jevos_band", JEVOSResult: jr,
	}
}

func applyPolicyAction(a ToolAction, p AgentPolicy) PermissionDecision {
	input := a.Args
	if input == nil {
		input = map[string]interface{}{"command": a.Command}
	}
	dec := Decision{Behavior: Ask, Reason: ""}
	out := ApplyAgentPolicy(a.ToolName, input, dec, p)
	if out.Behavior == Deny {
		return PermissionDecision{Decision: DecisionDeny, Reason: out.Reason, DecidedBy: DecidedByStatic, RuleID: "agent_policy"}
	}
	return PermissionDecision{Decision: DecisionReview}
}

func (g *Gateway) RememberSession(a ToolAction) error {
	if g.Sessions == nil || a.ConversationID <= 0 {
		return nil
	}
	return g.Sessions.Grant(a.ConversationID, SessionGrantFrom(a))
}

// MapLegacyBehavior 把 Gateway 结论映射到 hook 的 allow/deny（REVIEW 由调用方转人工）。
func MapLegacyBehavior(d PermissionDecision) Behavior {
	if d.Decision == DecisionAllow {
		return Allow
	}
	return Deny
}
