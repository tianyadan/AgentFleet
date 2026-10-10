package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"atolla/server/internal/store"
)

func TestFinalizeVisitorAskAuditNilSafe(t *testing.T) {
	s := &Service{}
	s.finalizeVisitorAskAudit(context.Background(), nil, nil, nil)
}

func TestToInt64Any(t *testing.T) {
	if toInt64Any(float64(12)) != 12 {
		t.Fatal("float64")
	}
	if toInt64Any(int64(7)) != 7 {
		t.Fatal("int64")
	}
	if toInt64Any("x") != 0 {
		t.Fatal("unknown")
	}
}

func TestVisitorAskAuditStatusCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	audit := &visitorAskAudit{logID: 0, started: time.Now()}
	// logID=0 → no DB write; ensure no panic with cancelled ctx
	s := &Service{}
	s.finalizeVisitorAskAudit(ctx, audit, &store.Message{ID: 1, Status: "ok"}, errors.New("x"))
}
