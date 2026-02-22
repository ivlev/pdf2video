package system

import (
	"image"
	"sync"
)

// ImagePool предоставляет механизмы повторного использования image.RGBA
// для снижения нагрузки на Garbage Collector (GC).
type ImagePool struct {
	pools map[string]*sync.Pool
	mu    sync.RWMutex
}

var globalPool = &ImagePool{
	pools: make(map[string]*sync.Pool),
}

// Get возвращает экземпляр *image.RGBA из пула или создает новый,
// если в пуле нет подходящего по размеру объекта.
func GetImage(rect image.Rectangle) *image.RGBA {
	return globalPool.Get(rect)
}

// Put возвращает экземпляр *image.RGBA в пул для повторного использования.
func PutImage(img *image.RGBA) {
	globalPool.Put(img)
}

func (p *ImagePool) Get(rect image.Rectangle) *image.RGBA {
	key := rect.String()
	p.mu.RLock()
	pool, exists := p.pools[key]
	p.mu.RUnlock()

	if !exists {
		p.mu.Lock()
		// Double check
		pool, exists = p.pools[key]
		if !exists {
			pool = &sync.Pool{
				New: func() interface{} {
					return image.NewRGBA(rect)
				},
			}
			p.pools[key] = pool
		}
		p.mu.Unlock()
	}

	return pool.Get().(*image.RGBA)
}

func (p *ImagePool) Put(img *image.RGBA) {
	if img == nil {
		return
	}
	key := img.Rect.String()
	p.mu.RLock()
	pool, exists := p.pools[key]
	p.mu.RUnlock()

	if exists {
		pool.Put(img)
	}
}
