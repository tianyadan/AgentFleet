package service

import (
	"context"
	"strings"

	"colleague-avatar/server/internal/agent"
	"colleague-avatar/server/internal/store"
)

const maxTerminalRecover = 2

// activityFromSink 把引擎 Activity 转成 SSE。
func activityFromSink(sink AskEventSink) agent.ActivityFn {
	if sink == nil {
		return nil
	}
	return func(a agent.Activity) {
		emitAsk(sink, map[string]any{
			"type": "activity", "tool": a.Tool, "summary": a.Summary,
		})
	}
}

// isTerminalKillErr 进程被超时/信号杀掉。
func isTerminalKillErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "signal: killed") ||
		strings.Contains(msg, "killed") ||
		strings.Contains(msg, "deadline exceeded") ||
		strings.Contains(msg, "context deadline")
}

// runEngineWithRecover 终端被杀后自检并续跑(有限次)。
func (s *Service) runEngineWithRecover(
	ctx context.Context,
	a *store.ManagedAgent,
	convID int64,
	sys, question, history string,
	onChunk func(string),
	onEvent AskEventSink,
	userTask string,
) (out string, ms int, err error) {
	out, ms, err = s.runManagedEngine(ctx, a, sys, question, history, onChunk, activityFromSink(onEvent), convID, a.ID, false, onEvent)
	if err == nil || !isTerminalKillErr(err) || ctx.Err() != nil {
		return out, ms, err
	}

	partial := strings.TrimSpace(out)
	for attempt := 1; attempt <= maxTerminalRecover; attempt++ {
		msg := "⚠️ 智能体任务终端（进程被中断），正在自检任务状态…（失败重试 " + itoa(attempt) + "/" + itoa(maxTerminalRecover) + "）"
		emitAsk(onEvent, map[string]any{"type": "system_note", "content": msg})
		_, _ = s.Store.InsertMessage(context.WithoutCancel(ctx), &store.Message{
			ConversationID: convID, Role: "system", Content: msg, Status: "ok",
		})
		if onChunk != nil {
			onChunk("\n\n" + msg + "\n\n")
		}

		checkQ := buildRecoverPrompt(userTask, partial, err.Error())
		out2, ms2, err2 := s.runManagedEngine(ctx, a, sys, checkQ, history, onChunk, activityFromSink(onEvent), convID, a.ID, false, onEvent)
		ms += ms2
		if err2 == nil {
			joined := strings.TrimSpace(partial)
			if joined != "" && out2 != "" {
				joined += "\n\n"
			}
			joined += strings.TrimSpace(out2)
			return joined, ms, nil
		}
		if !isTerminalKillErr(err2) {
			return strings.TrimSpace(partial + "\n\n" + out2), ms, err2
		}
		partial = strings.TrimSpace(partial + "\n\n" + out2)
		err = err2
	}
	return partial, ms, err
}

func buildRecoverPrompt(userTask, partial, killErr string) string {
	var b strings.Builder
	b.WriteString("【系统：任务终端自检】上一次执行被中断（")
	b.WriteString(truncate(killErr, 120))
	b.WriteString("）。请先检查工作区与已有改动，判断用户任务是否已经完成。\n")
	b.WriteString("- 若已完成：简要说明完成证据，然后正常结束，不要重复无意义操作。\n")
	b.WriteString("- 若未完成：从中断处继续完成剩余工作，不要从头无差别重做。\n\n")
	b.WriteString("用户原任务：\n")
	b.WriteString(userTask)
	if strings.TrimSpace(partial) != "" {
		b.WriteString("\n\n中断前已有输出摘要：\n")
		b.WriteString(truncate(partial, 2000))
	}
	return b.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d [12]byte
	i := len(d)
	for n > 0 {
		i--
		d[i] = byte('0' + n%10)
		n /= 10
	}
	return string(d[i:])
}
