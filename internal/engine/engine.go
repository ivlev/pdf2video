package engine

import (
	"fmt"
	"image/png"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/ivlev/pdf2video/internal/config"
	"github.com/ivlev/pdf2video/internal/effects"
	"github.com/ivlev/pdf2video/internal/source"
	"github.com/ivlev/pdf2video/internal/video"
)

type VideoProject struct {
	Config  *config.Config
	Source  source.Source
	Encoder video.VideoEncoder
	Effect  effects.Effect
	tempDir string
}

func NewVideoProject(cfg *config.Config, src source.Source, ve video.VideoEncoder, eff effects.Effect) *VideoProject {
	return &VideoProject{
		Config:  cfg,
		Source:  src,
		Encoder: ve,
		Effect:  eff,
	}
}

func (p *VideoProject) Run() error {
	var err error
	p.tempDir, err = os.MkdirTemp("", "pdf2video_")
	if err != nil {
		return err
	}
	defer os.RemoveAll(p.tempDir)

	pageCount := p.Source.PageCount()
	if pageCount == 0 {
		return fmt.Errorf("источник не содержит страниц/кадров")
	}

	// Рассчитываем рандомизированные длительности
	p.calculateDurations(pageCount)

	// Проверка корректности переходов относительно минимальной длительности
	minDur := p.Config.PageDurations[0]
	for _, d := range p.Config.PageDurations {
		if d < minDur {
			minDur = d
		}
	}

	if p.Config.FadeDuration >= minDur {
		p.Config.FadeDuration = minDur / 2.0
		fmt.Printf("[!] Переход уменьшен до %.2fs из-за короткого клипа\n", p.Config.FadeDuration)
		// Пересчитываем длительности с учетом нового FadeDuration, чтобы сохранить общую длину
		p.calculateDurations(pageCount)
	}

	if p.Config.Width == 1280 && p.Config.Height == 720 {
		srcW, srcH, err := p.Source.GetPageDimensions(0)
		if err == nil {
			p.Config.Width = int(float64(p.Config.Height) * (srcW / srcH))
			if p.Config.Width%2 != 0 {
				p.Config.Width++
			}
		}
	}

	fmt.Println("--- [PROJECT: MODULAR ENGINE] ---")
	fmt.Printf("[*] Источник: %s | Кадров/Страниц: %d\n", p.Config.InputPath, pageCount)
	fmt.Printf("[*] Разрешение: %dx%d @ %d FPS | DPI: %d\n", p.Config.Width, p.Config.Height, p.Config.FPS, p.Config.DPI)
	fmt.Println("-----------------------------")

	jobs := make(chan int, pageCount)
	results := make([]string, pageCount)
	var wg sync.WaitGroup

	numWorkers := p.Config.Workers
	if numWorkers > pageCount {
		numWorkers = pageCount
	}

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				img, err := p.Source.RenderPage(i, p.Config.DPI)
				if err != nil {
					log.Printf("[!] Error page %d: %v", i, err)
					continue
				}

				imgPath := filepath.Join(p.tempDir, fmt.Sprintf("p%d.png", i))
				imgFile, _ := os.Create(imgPath)
				png.Encode(imgFile, img)
				imgFile.Close()

				segPath := filepath.Join(p.tempDir, fmt.Sprintf("s%d.mp4", i))
				duration := p.Config.PageDurations[i]

				params := config.SegmentParams{
					Width:        p.Config.Width,
					Height:       p.Config.Height,
					FPS:          p.Config.FPS,
					Duration:     duration,
					ZoomMode:     p.Config.ZoomMode,
					ZoomSpeed:    p.Config.ZoomSpeed,
					FadeDuration: p.Config.FadeDuration,
					PageIndex:    i,
				}

				filter := p.Effect.GenerateFilter(params)

				cmd := exec.Command("ffmpeg", "-y",
					"-i", imgPath,
					"-vf", filter,
					"-t", fmt.Sprintf("%f", duration),
					"-r", fmt.Sprintf("%d", p.Config.FPS),
					"-pix_fmt", "yuv420p",
					"-c:v", "libx264",
					"-preset", "medium",
					segPath,
				)

				if out, err := cmd.CombinedOutput(); err != nil {
					log.Printf("[!] FFmpeg error page %d: %v\nLog: %s", i, err, string(out))
					continue
				}

				results[i] = segPath
				fmt.Printf("[>] Ready: %d/%d\n", i+1, pageCount)
			}
		}()
	}

	for i := 0; i < pageCount; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	fmt.Println("[*] Сборка финального видео (с эффектами переходов)...")
	return p.Encoder.Concatenate(results, p.Config.OutputVideo, p.tempDir, *p.Config)
}

func (p *VideoProject) calculateDurations(pageCount int) {
	// Общая визуальная длительность (продолжительность аудио)
	A := p.Config.TotalDuration
	// Длительность перехода
	F := p.Config.FadeDuration
	// Количество переходов
	numFades := float64(pageCount - 1)
	if numFades < 0 {
		numFades = 0
	}

	// Общая длительность всех клипов (A + numFades*F)
	// Это потому что каждый переход "съедает" F секунд общей длительности
	totalClipsDuration := A + numFades*F

	// Базовая длительность одного клипа (если была бы равномерной)
	Dbase := totalClipsDuration / float64(pageCount)

	durations := make([]float64, pageCount)
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	// Первая страница: отклонение от Dbase в диапазоне [-15%, +15%]
	variation := (r.Float64()*0.3 - 0.15) // [-0.15, 0.15]
	durations[0] = Dbase * (1 + variation)

	// Последующие страницы: отклонение от предыдущей в диапазоне [-15%, +15%]
	for i := 1; i < pageCount; i++ {
		variation := (r.Float64()*0.3 - 0.15)
		durations[i] = durations[i-1] * (1 + variation)
		// Ограничение: клип не может быть короче перехода (с запасом)
		if durations[i] < F*1.1 {
			durations[i] = F * 1.1
		}
	}

	// Масштабируем, чтобы сумма была в точности totalClipsDuration
	sum := 0.0
	for _, d := range durations {
		sum += d
	}

	scale := totalClipsDuration / sum
	for i := range durations {
		durations[i] *= scale
	}

	p.Config.PageDurations = durations
}
