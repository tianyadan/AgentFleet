package service

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/gif"
	_ "image/png"
	"strings"

	"golang.org/x/image/draw"
)

const avatarMaxEdge = 512
const avatarJPEGQuality = 82

// CompressAvatarImage 将任意图片压成最长边 ≤ maxEdge 的 JPEG。
func CompressAvatarImage(raw []byte, maxEdge int) ([]byte, error) {
	if maxEdge <= 0 {
		maxEdge = avatarMaxEdge
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty image")
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("invalid image: %w", err)
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 1 || h < 1 {
		return nil, fmt.Errorf("invalid image size")
	}
	nw, nh := w, h
	if w >= h {
		if w > maxEdge {
			nw = maxEdge
			nh = h * maxEdge / w
		}
	} else if h > maxEdge {
		nh = maxEdge
		nw = w * maxEdge / h
	}
	if nh < 1 {
		nh = 1
	}
	if nw < 1 {
		nw = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: avatarJPEGQuality}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// BuildAvatarObjectURL 拼接公网访问 URL。
func BuildAvatarObjectURL(publicBase, prefix, objectName string) string {
	base := strings.TrimRight(strings.TrimSpace(publicBase), "/")
	p := strings.Trim(strings.TrimSpace(prefix), "/")
	name := strings.TrimLeft(strings.TrimSpace(objectName), "/")
	if p == "" {
		return base + "/" + name
	}
	return base + "/" + p + "/" + name
}
