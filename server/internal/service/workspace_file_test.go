package service

import (
	"os"
	"path/filepath"
	"testing"

	"colleague-avatar/server/config"
	"colleague-avatar/server/internal/store"
)

func TestResolveAgentImageRelative(t *testing.T) {
	root := t.TempDir()
	img := filepath.Join(root, "out.png")
	if err := os.WriteFile(img, []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}, 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Service{Cfg: config.Config{WorkspaceRoot: root}}
	a := &store.ManagedAgent{WorkspacePath: root}
	abs, ct, err := s.ResolveAgentImage(a, "out.png")
	if err != nil {
		t.Fatal(err)
	}
	if abs != img {
		t.Fatalf("abs=%s want %s", abs, img)
	}
	if ct != "image/png" {
		t.Fatalf("ct=%s", ct)
	}
}

func TestResolveAgentImageRejectTraversal(t *testing.T) {
	root := t.TempDir()
	s := &Service{Cfg: config.Config{WorkspaceRoot: root}}
	a := &store.ManagedAgent{WorkspacePath: root}
	_, _, err := s.ResolveAgentImage(a, "../etc/passwd")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveAgentImageRejectNonImage(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "a.go")
	_ = os.WriteFile(p, []byte("package x"), 0o644)
	s := &Service{Cfg: config.Config{WorkspaceRoot: root}}
	a := &store.ManagedAgent{WorkspacePath: root}
	_, _, err := s.ResolveAgentImage(a, "a.go")
	if err == nil {
		t.Fatal("expected error")
	}
}
