package engine

import (
	"fmt"
	"image/png"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/ivlev/pdf2video/internal/config"
	"github.com/ivlev/pdf2video/internal/effects"
	"github.com/ivlev/pdf2video/internal/pdf"
	"github.com/ivlev/pdf2video/internal/video"
)

type VideoProject struct {
	Config  *config.Config
	PDF     pdf.PDFSource
	Encoder video.VideoEncoder
	Effect  effects.Effect
	tempDir string
}

func NewVideoProject(cfg *config.Config, pdfDoc pdf.PDFSource, ve video.VideoEncoder, eff effects.Effect) *VideoProject {
	return &VideoProject{
		Config:  cfg,
		PDF:     pdfDoc,
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

	pageCount := p.PDF.PageCount()
	pageDuration := p.Config.TotalDuration / float64(pageCount)

	if p.Config.FadeDuration >= pageDuration {
		p.Config.FadeDuration = pageDuration / 3.0
		fmt.Printf("[!] Переход уменьшен до %.2fs для соответствия слайдам\n", p.Config.FadeDuration)
	}

	if p.Config.Width == 1280 && p.Config.Height == 720 {
		pdfW, pdfH, err := p.PDF.GetPageDimensions(0)
		if err == nil {
			p.Config.Width = int(float64(p.Config.Height) * (pdfW / pdfH))
			if p.Config.Width%2 != 0 {
				p.Config.Width++
			}
		}
	}

	fmt.Println("--- [PROJECT: MODULAR ENGINE] ---")
	fmt.Printf("[*] Файл: %s | Страниц: %d\n", p.Config.InputPDF, pageCount)
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
				img, err := p.PDF.RenderPage(i, p.Config.DPI)
				if err != nil {
					log.Printf("[!] Error page %d: %v", i, err)
					continue
				}

				imgPath := filepath.Join(p.tempDir, fmt.Sprintf("p%d.png", i))
				imgFile, _ := os.Create(imgPath)
				png.Encode(imgFile, img)
				imgFile.Close()

				segPath := filepath.Join(p.tempDir, fmt.Sprintf("s%d.mp4", i))

				params := config.SegmentParams{
					Width:        p.Config.Width,
					Height:       p.Config.Height,
					FPS:          p.Config.FPS,
					Duration:     pageDuration,
					ZoomMode:     p.Config.ZoomMode,
					ZoomSpeed:    p.Config.ZoomSpeed,
					FadeDuration: p.Config.FadeDuration,
					PageIndex:    i,
				}

				filter := p.Effect.GenerateFilter(params)

				cmd := exec.Command("ffmpeg", "-y",
					"-i", imgPath,
					"-vf", filter,
					"-t", fmt.Sprintf("%f", pageDuration),
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
