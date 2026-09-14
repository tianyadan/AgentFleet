package service

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// EvalCondition 表达式判定。支持:
//   status == "PASS"
//   progress >= 80
//   contains(summary, "通过")
//   A and B / A or B
// ctx 为上游 output 扁平字段(string/number/bool)。
func EvalCondition(expr string, ctx map[string]any) (bool, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return true, nil
	}
	// 按 or 拆（最低优先级）
	parts := splitTop(expr, " or ")
	if len(parts) > 1 {
		for _, p := range parts {
			ok, err := EvalCondition(p, ctx)
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}
		return false, nil
	}
	parts = splitTop(expr, " and ")
	if len(parts) > 1 {
		for _, p := range parts {
			ok, err := EvalCondition(p, ctx)
			if err != nil {
				return false, err
			}
			if !ok {
				return false, nil
			}
		}
		return true, nil
	}
	return evalAtom(expr, ctx)
}

func splitTop(s, sep string) []string {
	low := strings.ToLower(s)
	sepLow := strings.ToLower(sep)
	var out []string
	start := 0
	for {
		i := strings.Index(low[start:], sepLow)
		if i < 0 {
			out = append(out, strings.TrimSpace(s[start:]))
			break
		}
		out = append(out, strings.TrimSpace(s[start:start+i]))
		start = start + i + len(sep)
	}
	return out
}

var (
	reCmp      = regexp.MustCompile(`(?i)^([a-zA-Z_][\w.]*)\s*(==|!=|>=|<=|>|<)\s*(.+)$`)
	reContains = regexp.MustCompile(`(?i)^contains\(\s*([a-zA-Z_][\w.]*)\s*,\s*(.+)\s*\)$`)
)

func evalAtom(expr string, ctx map[string]any) (bool, error) {
	expr = strings.TrimSpace(expr)
	if m := reContains.FindStringSubmatch(expr); m != nil {
		field := m[1]
		needle, err := parseLiteral(strings.TrimSpace(m[2]))
		if err != nil {
			return false, err
		}
		hay := fmt.Sprint(lookup(ctx, field))
		return strings.Contains(hay, fmt.Sprint(needle)), nil
	}
	if m := reCmp.FindStringSubmatch(expr); m != nil {
		field, op, raw := m[1], m[2], strings.TrimSpace(m[3])
		left := lookup(ctx, field)
		right, err := parseLiteral(raw)
		if err != nil {
			return false, err
		}
		return compare(left, op, right)
	}
	return false, fmt.Errorf("unsupported expr: %s", expr)
}

func lookup(ctx map[string]any, path string) any {
	if ctx == nil {
		return nil
	}
	parts := strings.Split(path, ".")
	var cur any = ctx
	for _, p := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			// also try string-keyed from nested
			return nil
		}
		cur, ok = m[p]
		if !ok {
			return nil
		}
	}
	return cur
}

func parseLiteral(raw string) (any, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 2 {
		if (raw[0] == '"' && raw[len(raw)-1] == '"') || (raw[0] == '\'' && raw[len(raw)-1] == '\'') {
			return raw[1 : len(raw)-1], nil
		}
	}
	low := strings.ToLower(raw)
	if low == "true" {
		return true, nil
	}
	if low == "false" {
		return false, nil
	}
	if n, err := strconv.ParseFloat(raw, 64); err == nil {
		return n, nil
	}
	return raw, nil
}

func compare(left any, op string, right any) (bool, error) {
	// 数值优先
	lf, lok := toFloat(left)
	rf, rok := toFloat(right)
	if lok && rok {
		switch op {
		case "==":
			return lf == rf, nil
		case "!=":
			return lf != rf, nil
		case ">":
			return lf > rf, nil
		case ">=":
			return lf >= rf, nil
		case "<":
			return lf < rf, nil
		case "<=":
			return lf <= rf, nil
		}
	}
	ls, rs := fmt.Sprint(left), fmt.Sprint(right)
	switch op {
	case "==":
		return ls == rs, nil
	case "!=":
		return ls != rs, nil
	default:
		return false, fmt.Errorf("op %s needs numbers", op)
	}
}

func toFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case string:
		n, err := strconv.ParseFloat(t, 64)
		return n, err == nil
	case bool:
		if t {
			return 1, true
		}
		return 0, true
	default:
		return 0, false
	}
}
