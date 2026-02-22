package system

import (
	"crypto/sha256"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
)

// RenderCache предоставляет механизмы сохранения и загрузки отрендеренных страниц.
type RenderCache struct {
	Dir string
}

func NewRenderCache(dir string) *RenderCache {
	if dir == "" {
		dir = "cache/renders"
	}
	_ = os.MkdirAll(dir, 0755)
	return &RenderCache{Dir: dir}
}

// GetKey генерирует уникальный ключ для страницы на основе пути, времени изменения, индекса и DPI.
func (c *RenderCache) GetKey(path string, pageIndex int, dpi int) string {
	info, err := os.Stat(path)
	modTime := ""
	if err == nil {
		modTime = info.ModTime().String()
	}

	data := fmt.Sprintf("%s|%s|%d|%d", path, modTime, pageIndex, dpi)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x.png", hash)
}

// Get пытается загрузить изображение из кэша.
func (c *RenderCache) Get(key string) (*image.RGBA, bool) {
	filePath := filepath.Join(c.Dir, key)
	f, err := os.Open(filePath)
	if err != nil {
		return nil, false
	}
	defer f.Close()

	img, err := png.Decode(f)
	if err != nil {
		return nil, false
	}

	// Копируем в RGBA (используя наш пул, если возможно)
	bounds := img.Bounds()
	rgba := GetImage(bounds)
	draw.Draw(rgba, bounds, img, bounds.Min, draw.Src)

	return rgba, true
}

// Put сохраняет изображение в кэш.
func (c *RenderCache) Put(key string, img image.Image) error {
	filePath := filepath.Join(c.Dir, key)
	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	return png.Encode(f, img)
}
