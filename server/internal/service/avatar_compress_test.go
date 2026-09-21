package service

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestCompressAvatarJPEG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1200, 800))
	for y := 0; y < 800; y++ {
		for x := 0; x < 1200; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		t.Fatal(err)
	}
	out, err := CompressAvatarImage(pngBuf.Bytes(), 512)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width > 512 || cfg.Height > 512 {
		t.Fatalf("size %dx%d", cfg.Width, cfg.Height)
	}
	if max(cfg.Width, cfg.Height) != 512 {
		t.Fatalf("want max edge 512, got %dx%d", cfg.Width, cfg.Height)
	}
}

func TestCompressAvatarRejectsNonImage(t *testing.T) {
	_, err := CompressAvatarImage([]byte("not-an-image"), 512)
	if err == nil {
		t.Fatal("want error")
	}
}

func TestBuildAvatarObjectURL(t *testing.T) {
	u := BuildAvatarObjectURL("https://cdn.example.com/", "avatars/", "agent-1-9.jpg")
	if u != "https://cdn.example.com/avatars/agent-1-9.jpg" {
		t.Fatal(u)
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
