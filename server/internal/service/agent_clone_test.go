package service

import (
	"errors"
	"strings"
	"testing"

	"colleague-avatar/server/internal/store"
)

func TestAgentStatusInitializing(t *testing.T) {
	if AgentStatusInitializing != "initializing" {
		t.Fatalf("got %s", AgentStatusInitializing)
	}
}

func TestOccupancyBlocksUserChat(t *testing.T) {
	if OccupancyBlocksUserChat(nil) {
		t.Fatal("empty")
	}
	if OccupancyBlocksUserChat(&store.Occupancy{SourceType: SourceDirect}) {
		t.Fatal("direct chat should replace")
	}
	if !OccupancyBlocksUserChat(&store.Occupancy{SourceType: SourceWorkflow}) {
		t.Fatal("workflow blocks")
	}
	if !OccupancyBlocksUserChat(&store.Occupancy{SourceType: SourceCloneInit}) {
		t.Fatal("clone init must block ask")
	}
}

func TestGuardAgentUsable(t *testing.T) {
	if err := GuardAgentUsable(&store.ManagedAgent{Status: "idle"}); err != nil {
		t.Fatal(err)
	}
	err := GuardAgentUsable(&store.ManagedAgent{Status: AgentStatusInitializing})
	if !errors.Is(err, ErrAgentInitializing) {
		t.Fatalf("got %v", err)
	}
}

func TestSourceReadyForSummary(t *testing.T) {
	if !SourceReadyForSummary("idle", nil) {
		t.Fatal("idle ready")
	}
	if SourceReadyForSummary("running", nil) {
		t.Fatal("running not ready")
	}
	if SourceReadyForSummary("idle", &store.Occupancy{SourceType: "WORKFLOW"}) {
		t.Fatal("occupied not ready")
	}
	if SourceReadyForSummary("compressing", nil) {
		t.Fatal("compressing not ready")
	}
}

func TestSourceHasEngineMemory(t *testing.T) {
	if SourceHasEngineMemory("sess-1", 0) != true {
		t.Fatal("session counts")
	}
	if SourceHasEngineMemory("", 2) != true {
		t.Fatal("messages count")
	}
	if SourceHasEngineMemory("  ", 0) {
		t.Fatal("empty")
	}
}

func TestNewCloneFromSource(t *testing.T) {
	se := true
	src := store.ManagedAgent{
		ID: 9, Name: "源", FolderID: 3, Engine: "claude", BinPath: "claude",
		RulesPrompt: "人设", AutoReview: true, TaskPlanEnabled: false,
		AllowWrite: true, AllowNetwork: false, AllowRm: true, AllowBrowser: true,
		WorkspacePath: "/tmp/ws", ScheduleEnabled: se, ScheduleCron: "* * * * *",
		ScheduleLabel: "每分", ConversationID: 88, Status: "running",
	}
	c := NewCloneFromSource(src, "新人", "https://img/a.png")
	if c.Name != "新人" || c.AvatarURL != "https://img/a.png" {
		t.Fatalf("name/avatar %+v", c)
	}
	if c.FolderID != 3 || c.WorkspacePath != "/tmp/ws" {
		t.Fatal("folder/workspace")
	}
	if c.ScheduleEnabled || c.ScheduleCron != "" {
		t.Fatal("schedule must be off")
	}
	if c.Status != AgentStatusInitializing || c.ConversationID != 0 {
		t.Fatal("status/conv")
	}
	if c.ClonedFromID != 9 || c.Engine != "claude" || c.TaskPlanEnabled || !c.AutoReview {
		t.Fatalf("flags %+v", c)
	}
	if !c.AllowRm || c.AllowNetwork {
		t.Fatal("perms")
	}
}

func TestDefaultCloneName(t *testing.T) {
	if DefaultCloneName("小王") != "小王 副本" {
		t.Fatal(DefaultCloneName("小王"))
	}
}

func TestTruncateRunes(t *testing.T) {
	if TruncateRunes("abcd", 3) != "abc" {
		t.Fatal(TruncateRunes("abcd", 3))
	}
	s := TruncateRunes("一二三四五", 2)
	if s != "一二" {
		t.Fatal(s)
	}
}

func TestBuildClonePrompts(t *testing.T) {
	sum := BuildCloneSummaryPrompt("源员", "/ws")
	if !strings.Contains(sum, "源员") || !strings.Contains(sum, "projects") {
		t.Fatalf("summary prompt: %s", sum)
	}
	init := BuildCloneInitPrompt("新人", "源员", "背景ABC")
	if !strings.Contains(init, "新人") || !strings.Contains(init, "背景ABC") || !strings.Contains(init, "隐藏初始化") {
		t.Fatalf("init prompt: %s", init)
	}
}

func TestChooseCloneSummary(t *testing.T) {
	if got := ChooseCloneSummary(false, "ignored"); got != DefaultCloneSummaryText {
		t.Fatal(got)
	}
	long := strings.Repeat("字", 1300)
	got := ChooseCloneSummary(true, long)
	if len([]rune(got)) != CloneSummaryMaxRunes {
		t.Fatalf("len=%d", len([]rune(got)))
	}
}

func TestShouldEmitTaskPlanSkipSilent(t *testing.T) {
	a := &store.ManagedAgent{TaskPlanEnabled: true}
	if shouldEmitTaskPlanAsk(a, true) {
		t.Fatal("silent must skip plan")
	}
	if !shouldEmitTaskPlanAsk(a, false) {
		t.Fatal("normal plan on")
	}
}

func TestIsInitializingConflict(t *testing.T) {
	if !IsInitializingConflict(ErrAgentInitializing) {
		t.Fatal("want true")
	}
}
