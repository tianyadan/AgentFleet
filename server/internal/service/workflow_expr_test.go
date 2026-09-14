package service

import (
	"testing"
)

func TestEvalConditionStatusPass(t *testing.T) {
	ok, err := EvalCondition(`status == "PASS"`, map[string]any{"status": "PASS"})
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestEvalConditionContainsAnd(t *testing.T) {
	ok, err := EvalCondition(`contains(summary, "通过") and progress >= 80`, map[string]any{
		"summary": "测试通过", "progress": 90,
	})
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestEvalConditionOr(t *testing.T) {
	ok, err := EvalCondition(`status == "FAIL" or status == "PASS"`, map[string]any{"status": "PASS"})
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}
