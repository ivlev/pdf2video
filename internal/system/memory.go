package system

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	"github.com/shirou/gopsutil/v3/mem"
	"golang.org/x/sync/semaphore"
)

// MemoryManager контролирует бюджет оперативной памяти, чтобы предотвратить OOM.
type MemoryManager struct {
	totalLimit int64 // в байтах
	used       int64
	mu         sync.Mutex
	sem        *semaphore.Weighted
}

func NewMemoryManager(maxMB int) *MemoryManager {
	limit := int64(maxMB) * 1024 * 1024
	if limit <= 0 {
		// Автоопределение: используем 75% доступной памяти
		v, err := mem.VirtualMemory()
		if err == nil {
			limit = int64(float64(v.Available) * 0.75)
		} else {
			// Fallback: 2GB
			limit = 2 * 1024 * 1024 * 1024
		}
	}

	m := &MemoryManager{
		totalLimit: limit,
		sem:        semaphore.NewWeighted(limit),
	}

	fmt.Printf("[*] Memory Budget initialized: %d MB\n", limit/1024/1024)
	return m
}

// Acquire блокирует выполнение, пока не освободится достаточное количество памяти.
func (m *MemoryManager) Acquire(ctx context.Context, bytes int64) error {
	if err := m.sem.Acquire(ctx, bytes); err != nil {
		return err
	}

	m.mu.Lock()
	m.used += bytes
	m.mu.Unlock()
	return nil
}

// Release освобождает забронированный объем памяти.
func (m *MemoryManager) Release(bytes int64) {
	m.sem.Release(bytes)

	m.mu.Lock()
	m.used -= bytes
	if m.used < 0 {
		m.used = 0
	}
	m.mu.Unlock()
}

// GetFrameSize рассчитывает примерный размер одного кадра RGBA в байтах.
func GetFrameSize(width, height int) int64 {
	return int64(width) * int64(height) * 4
}

// GetRecommendedWorkers возвращает рекомендуемое кол-во воркеров на основе RAM.
func (m *MemoryManager) GetRecommendedWorkers(frameSize int64, maxCPUs int) int {
	// Оставляем запас: считаем, что каждый воркер потребляет минимум 2 кадра (рендер + очередь кодирования)
	limitWorkers := int(m.totalLimit / (frameSize * 2))
	if limitWorkers < 1 {
		limitWorkers = 1
	}
	if limitWorkers > maxCPUs {
		return maxCPUs
	}
	return limitWorkers
}

func GetSystemTotalMemory() uint64 {
	v, err := mem.VirtualMemory()
	if err != nil {
		return 0
	}
	return v.Total
}

func GetRuntimeMemory() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Alloc
}
