package engine

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"io"
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
					log.Printf("[!] Error rendering page %d: %v", i, err)
					continue
				}

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

				inputW, inputH := img.Bounds().Dx(), img.Bounds().Dy()
				qualityArgs := []string{}
				switch p.Config.VideoEncoder {
				case "h264_videotoolbox":
					// VideoToolbox часто не поддерживает -q:v напрямую на всех версиях. Используем битрейт.
					bitrate := p.Config.Quality * 100 // кбит/с. 75 -> 7.5Мбит/с
					qualityArgs = append(qualityArgs, "-b:v", fmt.Sprintf("%dk", bitrate))
				case "h264_nvenc":
					qualityArgs = append(qualityArgs, "-cq", fmt.Sprintf("%d", p.Config.Quality))
				default: // libx264
					qualityArgs = append(qualityArgs, "-crf", fmt.Sprintf("%d", p.Config.Quality), "-preset", "medium")
				}

				// Используем rawvideo через stdin для исключения I/O на диск
				ffmpegArgs := []string{
					"-y",
					"-f", "rawvideo",
					"-pixel_format", "rgba",
					"-video_size", fmt.Sprintf("%dx%d", inputW, inputH),
					"-i", "-",
					"-vf", filter,
					"-t", fmt.Sprintf("%f", duration),
					"-r", fmt.Sprintf("%d", p.Config.FPS),
					"-pix_fmt", "yuv420p",
					"-c:v", p.Config.VideoEncoder,
				}
				ffmpegArgs = append(ffmpegArgs, qualityArgs...)
				ffmpegArgs = append(ffmpegArgs, segPath)

				cmd := exec.Command("ffmpeg", ffmpegArgs...)
				var out bytes.Buffer
				cmd.Stdout = &out
				cmd.Stderr = &out

				stdin, err := cmd.StdinPipe()
				if err != nil {
					log.Printf("[!] StdinPipe error page %d: %v", i, err)
					continue
				}

				if err := cmd.Start(); err != nil {
					log.Printf("[!] FFmpeg start error page %d: %v", i, err)
					continue
				}

				// Передаем один кадр raw-данных. zoompan с d=N размножит его.
				if err := p.writeRawRGBA(stdin, img); err != nil {
					log.Printf("[!] Write raw error page %d: %v", i, err)
					stdin.Close()
					continue
				}
				stdin.Close()

				if err := cmd.Wait(); err != nil {
					log.Printf("[!] FFmpeg wait error page %d: %v\nLog: %s", i, err, out.String())
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
func (p *VideoProject) writeRawRGBA(w io.Writer, img image.Image) error {
	bounds := img.Bounds()
	rgba, ok := img.(*image.RGBA)
	// Проверяем, является ли изображение уже RGBA и имеет ли стандартный шаг (stride)
	if !ok || rgba.Stride != bounds.Dx()*4 || rgba.Rect.Min.X != 0 || rgba.Rect.Min.Y != 0 {
		rgba = image.NewRGBA(bounds)
		draw.Draw(rgba, bounds, img, bounds.Min, draw.Src)
	}
	_, err := w.Write(rgba.Pix)
	return err
}
