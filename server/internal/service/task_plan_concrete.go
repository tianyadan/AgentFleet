package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"colleague-avatar/server/internal/store"
)

var vagueStepRe = regexp.MustCompile(`(?i)(正在规划|正在执行|正在输出|规划中|\b执行中\b|输出结果|处理中|开始任务|整理并回复|理解需求与目标|执行任务)`)

// PlanConcreteSteps 产出 4～8 条具体可执行步骤,禁止空话。
func (s *Service) PlanConcreteSteps(ctx context.Context, a *store.ManagedAgent, question string, callees []AgentRef) []string {
	q := strings.TrimSpace(question)
	if q == "" {
		q = "完成用户下达的任务"
	}
	var raw []string
	if a != nil {
		pctx, cancel := context.WithTimeout(ctx, 45*time.Second)
		raw = s.planStepsViaEngine(pctx, a, q, callees)
		cancel()
	}
	out := filterConcreteSteps(raw)
	if len(out) >= 4 {
		if len(out) > 8 {
			out = out[:8]
		}
		return out
	}
	return HeuristicPlanSteps(q, callees)
}

func filterConcreteSteps(steps []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range steps {
		s = strings.TrimSpace(s)
		s = strings.TrimLeft(s, "0123456789.、)） ")
		if s == "" || vagueStepRe.MatchString(s) {
			continue
		}
		if utf8.RuneCountInString(s) < 4 {
			continue
		}
		key := strings.ToLower(s)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, s)
	}
	return out
}

// HeuristicPlanSteps 无模型/拆分失败时的具体兜底(保证 ≥4 且无空话)。
func HeuristicPlanSteps(question string, callees []AgentRef) []string {
	q := strings.TrimSpace(question)
	steps := []string{"确认任务目标：" + truncateRunes(q, 36)}
	for _, p := range splitTaskHints(q) {
		steps = append(steps, "落实："+p)
		if len(steps) >= 5 {
			break
		}
	}
	if len(steps) < 3 {
		steps = append(steps,
			"收集与任务相关的代码、配置或日志线索",
			"按目标完成核心查询或改动",
		)
	}
	for _, c := range callees {
		name := c.Name
		if name == "" {
			name = fmt.Sprintf("#%d", c.ID)
		}
		steps = append(steps, "整合「"+name+"」返回的专项结果")
	}
	if len(callees) > 0 {
		steps = append(steps, "汇总各位同事产出并给出明确结论")
	} else {
		steps = append(steps, "核对结果是否满足要求后回复用户")
	}
	out := filterConcreteSteps(steps)
	if len(out) < 4 {
		// 极端兜底,仍避免「正在…」空话
		out = append(out,
			"列出需要触达的文件或接口清单",
			"逐项完成清单中的关键动作",
			"记录关键输出与异常",
			"向用户汇报结论与下一步建议",
		)
		out = filterConcreteSteps(out)
	}
	if len(out) > 8 {
		out = out[:8]
	}
	return out
}

func splitTaskHints(q string) []string {
	q = strings.ReplaceAll(q, "\n", "。")
	seps := regexp.MustCompile(`[。；;！!？?]+`)
	raw := seps.Split(q, -1)
	var out []string
	for _, r := range raw {
		r = strings.TrimSpace(r)
		r = strings.TrimLeft(r, "-*•、 ")
		if utf8.RuneCountInString(r) < 6 {
			continue
		}
		out = append(out, truncateRunes(r, 48))
		if len(out) >= 5 {
			break
		}
	}
	return out
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func parseStepsJSON(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	i := strings.Index(raw, "[")
	j := strings.LastIndex(raw, "]")
	if i >= 0 && j > i {
		raw = raw[i : j+1]
	}
	var arr []string
	if err := json.Unmarshal([]byte(raw), &arr); err == nil {
		return arr
	}
	var objs []map[string]string
	if err := json.Unmarshal([]byte(raw), &objs); err == nil {
		for _, o := range objs {
			if o["step"] != "" {
				arr = append(arr, o["step"])
			} else if o["title"] != "" {
				arr = append(arr, o["title"])
			}
		}
	}
	return arr
}

func (s *Service) planStepsViaEngine(ctx context.Context, a *store.ManagedAgent, question string, callees []AgentRef) []string {
	var b strings.Builder
	b.WriteString("把下面任务拆成 4～8 个具体可执行步骤。\n")
	b.WriteString("要求：每步写清动作与对象；禁止「正在规划/正在执行/正在输出结果/理解需求/整理回复」等空话。\n")
	b.WriteString("只输出 JSON 字符串数组，例如 [\"查阅 xx 配置\",\"修改 yy 函数\",...]。\n")
	if len(callees) > 0 {
		b.WriteString("其中会协作其他同事：")
		for i, c := range callees {
			if i > 0 {
				b.WriteString("、")
			}
			b.WriteString(c.Name)
		}
		b.WriteString("。步骤里可包含「等待/整合某位同事结果」。\n")
	}
	b.WriteString("任务：\n")
	b.WriteString(question)

	sys := "你是任务拆分助手,只输出 JSON 数组。"
	out, _, err := s.runManagedEngine(ctx, a, sys, b.String(), "", nil, nil, 0, 0, true, nil)
	if err != nil {
		return nil
	}
	return parseStepsJSON(out)
}
