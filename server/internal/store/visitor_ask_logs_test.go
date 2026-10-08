package store

import "testing"

func TestMaskIP(t *testing.T) {
	if got := MaskIP("117.136.22.82"); got != "117.136.***.82" {
		t.Fatalf("got %q", got)
	}
	if got := MaskIP(""); got != "" {
		t.Fatalf("empty got %q", got)
	}
}

func TestAskStatusConsts(t *testing.T) {
	if AskStatusProcessing != "processing" || AskStatusSuccess != "success" {
		t.Fatal("status consts")
	}
}
