package service

import (
	"testing"

	"colleague-avatar/server/internal/store"
)

func TestShouldEmitTaskPlan(t *testing.T) {
	if !shouldEmitTaskPlan(nil) {
		t.Fatal("nil agent defaults on")
	}
	if !shouldEmitTaskPlan(&store.ManagedAgent{TaskPlanEnabled: true}) {
		t.Fatal("want on")
	}
	if shouldEmitTaskPlan(&store.ManagedAgent{TaskPlanEnabled: false}) {
		t.Fatal("want off")
	}
}
