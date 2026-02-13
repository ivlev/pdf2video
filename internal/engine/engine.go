package engine

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"io"
	"log"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/ivlev/pdf2video/internal/analyzer"
	"github.com/ivlev/pdf2video/internal/config"
	"github.com/ivlev/pdf2video/internal/director"
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
	startTime := time.Now()
	var renderStart, renderEnd, encodeStart, encodeEnd, concatStart time.Time

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

	// Обработка сценариев
	if p.Config.GenerateScenario {
		return p.handleGenerateScenario(pageCount)
	}

	if p.Config.ScenarioInput != "" {
		scenario, err := director.ReadScenario(p.Config.ScenarioInput)
		if err != nil {
			return fmt.Errorf("ошибка чтения сценария: %v", err)
		}
		p.Effect = effects.NewScenarioEffect(scenario)
		fmt.Printf("[*] Используется сценарий: %s\n", p.Config.ScenarioInput)

		// Если сценарий загружен, длительности берем из него
		if len(scenario.Slides) > 0 {
			durations := make([]float64, pageCount)
			scenarioTotalClipsDur := 0.0
			for i := 0; i < pageCount && i < len(scenario.Slides); i++ {
				durations[i] = scenario.Slides[i].Duration
				scenarioTotalClipsDur += durations[i]
			}

			// Если в конфиге уже задана общая длительность (например, из аудио),
			// масштабируем длительности слайдов из сценария
			if p.Config.TotalDuration > 0 {
				targetTotalClipsDur := p.Config.TotalDuration
				if pageCount > 1 {
					targetTotalClipsDur += float64(pageCount-1) * p.Config.FadeDuration
				}

				if scenarioTotalClipsDur > 0 {
					scale := targetTotalClipsDur / scenarioTotalClipsDur
					fmt.Printf("[*] Сценарий масштабирован под аудио (x%.3f): общая длительность сегментов %.2fs\n", scale, targetTotalClipsDur)
					for i := range durations {
						durations[i] *= scale
						// Выравниваем по кадрам для стабильности xfade
						durations[i] = math.Round(durations[i]*float64(p.Config.FPS)) / float64(p.Config.FPS)
					}
				}
			} else {
				// Если общая длительность не задана, рассчитываем её по сценарию
				total := scenarioTotalClipsDur
				if pageCount > 1 {
					total -= float64(pageCount-1) * p.Config.FadeDuration
				}
				p.Config.TotalDuration = total
				for i := range durations {
					// Выравниваем по кадрам
					durations[i] = math.Round(durations[i]*float64(p.Config.FPS)) / float64(p.Config.FPS)
				}
			}
			p.Config.PageDurations = durations
		}
	} else {
		// Рассчитываем рандомизированные длительности (стандартный режим)
		p.calculateDurations(pageCount)
	}

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

	// Каналы для пайплайна
	// jobs -> renderPool -> renderResults -> encodePool -> results
	jobs := make(chan int, pageCount)
	renderResults := make(chan *RenderResult, pageCount)
	// Финализируем общую длительность по сумме сегментов (после выравнивания)
	sumDur := 0.0
	for _, d := range p.Config.PageDurations {
		sumDur += d
	}
	if pageCount > 1 {
		sumDur -= float64(pageCount-1) * p.Config.FadeDuration
	}
	p.Config.TotalDuration = sumDur

	results := make([]string, pageCount)

	var wgRender sync.WaitGroup
	var wgEncode sync.WaitGroup

	// 1. Render Pool (CPU bound)
	// Используем все доступные ядра для рендеринга PDF
	numRenderWorkers := p.Config.Workers
	if numRenderWorkers > pageCount {
		numRenderWorkers = pageCount
	}

	for w := 0; w < numRenderWorkers; w++ {
		wgRender.Add(1)
		go func() {
			defer wgRender.Done()
			for i := range jobs {
				img, err := p.Source.RenderPage(i, p.Config.DPI)
				if err != nil {
					log.Printf("[!] Error rendering page %d: %v", i, err)
					continue
				}
				// Отправляем результат рендеринга в канал кодирования
				renderResults <- &RenderResult{Index: i, Image: img}
			}
		}()
	}

	// 2. Encode Pool (GPU/Encoder bound)
	// Ограничиваем количество параллельных энкодеров, чтобы не перегрузить GPU/VRAM
	// 4 - разумный компромисс для большинства GPU (NVENC/VideoToolbox)
	numEncodeWorkers := 4
	if numEncodeWorkers > pageCount {
		numEncodeWorkers = pageCount
	}

	for w := 0; w < numEncodeWorkers; w++ {
		wgEncode.Add(1)
		go func() {
			defer wgEncode.Done()
			for res := range renderResults {
				i := res.Index
				img := res.Image

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
					// Don't continue silently. If a segment fails, the whole video will be broken.
					// We should probably set a failure flag.
					continue
				}

				results[i] = segPath
				fmt.Printf("[>] Ready: %d/%d\n", i+1, pageCount)
			}
		}()
	}

	// Запускаем задачи рендеринга
	renderStart = time.Now()
	for i := 0; i < pageCount; i++ {
		jobs <- i
	}
	close(jobs)

	// Ждем завершения рендеринга
	wgRender.Wait()
	renderEnd = time.Now()
	close(renderResults)

	// Ждем завершения кодирования
	encodeStart = renderStart // Encode по факту стартует почти сразу с Render
	wgEncode.Wait()
	encodeEnd = time.Now()

	// Проверяем, все ли сегменты готовы
	for i, r := range results {
		if r == "" {
			return fmt.Errorf("сегмент %d не был создан. Проверьте логи FFmpeg", i)
		}
	}

	fmt.Println("[*] Сборка финального видео (с эффектами переходов)...")
	concatStart = time.Now()
	err = p.Encoder.Concatenate(results, p.Config.OutputVideo, p.tempDir, *p.Config)
	if err != nil {
		return fmt.Errorf("ошибка сборки финального видео: %v", err)
	}

	totalTime := time.Since(startTime)
	renderTime := renderEnd.Sub(renderStart)
	encodeTime := encodeEnd.Sub(encodeStart)
	concatTime := time.Since(concatStart)
	fps := float64(pageCount) / totalTime.Seconds()

	if p.Config.ShowStats {
		report := fmt.Sprintf(
			"--- [PERFORMANCE REPORT] ---\n"+
				"Build: %s\n"+
				"Total Time: %.2fs\n"+
				"Rendering (CPU): %.2fs\n"+
				"Encoding (GPU/CPU): %.2fs\n"+
				"Concatenation: %.2fs\n"+
				"Effective FPS: %.2f\n"+
				"----------------------------\n",
			p.Config.BuildVersion, totalTime.Seconds(), renderTime.Seconds(), encodeTime.Seconds(), concatTime.Seconds(), fps,
		)
		fmt.Print(report)

		// Логирование в файл
		logEntry := fmt.Sprintf("[%s] Build: %s | Input: %s | Pages: %d | Total: %.2fs | Render: %.2fs | Encode: %.2fs | FPS: %.2f\n",
			time.Now().Format("2006-01-02 15:04:05"),
			p.Config.BuildVersion,
			filepath.Base(p.Config.InputPath),
			pageCount,
			totalTime.Seconds(),
			renderTime.Seconds(),
			encodeTime.Seconds(),
			fps,
		)

		f, err := os.OpenFile("benchmark.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			f.WriteString(logEntry)
			f.Close()
		} else {
			fmt.Printf("[!] Не удалось записать benchmark.log: %v\n", err)
		}
	}

	return nil
}

type RenderResult struct {
	Index int
	Image image.Image
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

func (p *VideoProject) handleGenerateScenario(pageCount int) error {
	fmt.Println("[*] Режим генерации сценария...")

	// Используем Director для генерации путей камеры
	dir := director.NewDirector(p.Config.Width, p.Config.Height)

	// Инициализируем детектор (по умолчанию ContrastDetector)
	var det analyzer.Detector
	cdet := analyzer.NewContrastDetector()
	cdet.MinBlockArea = p.Config.MinBlockArea
	cdet.EdgeThreshold = p.Config.EdgeThreshold
	det = cdet

	var slides []director.Slide
	for i := 0; i < pageCount; i++ {
		fmt.Printf("[*] Анализ страницы %d/%d...\n", i+1, pageCount)

		img, err := p.Source.RenderPage(i, p.Config.DPI)
		if err != nil {
			log.Printf("[!] Ошибка рендеринга страницы %d для анализа: %v", i, err)
			continue
		}

		// Поиск блоков на изображении
		blocks, err := det.Detect(img)
		if err != nil {
			log.Printf("[!] Ошибка анализа страницы %d: %v", i, err)
			// Продолжаем с пустым списком блоков
		}

		// Генерация сценария для конкретной страницы (слайда)
		slideDuration := 5.0
		if len(p.Config.PageDurations) > i {
			slideDuration = p.Config.PageDurations[i]
		}

		slideScenario, err := dir.GenerateScenario(blocks, fmt.Sprintf("slide_%d.png", i+1), slideDuration)
		if err != nil || len(slideScenario.Slides) == 0 {
			// Если анализ не удался, создаем пустой слайд
			slides = append(slides, director.Slide{
				ID:       i + 1,
				Input:    fmt.Sprintf("slide_%d.png", i+1),
				Duration: slideDuration,
				Keyframes: []director.Keyframe{
					{
						Time:  0,
						Focus: "full_view",
						Rect:  director.Rectangle{X: 0, Y: 0, W: p.Config.Width, H: p.Config.Height},
						Zoom:  1.0,
					},
				},
			})
			continue
		}

		slide := slideScenario.Slides[0]
		slide.ID = i + 1
		slides = append(slides, slide)
	}

	scenario := &director.Scenario{
		Version: "1.0",
		Slides:  slides,
	}

	outputPath := p.Config.ScenarioOutput
	if outputPath == "" {
		timestamp := time.Now().Format("2006-01-02_15-04-05")
		outputPath = filepath.Join("internal/scenarios", fmt.Sprintf("scenario_%s.yaml", timestamp))
	}

	// Убеждаемся, что директория существует
	os.MkdirAll(filepath.Dir(outputPath), 0755)

	err := director.WriteScenario(scenario, outputPath)
	if err != nil {
		return err
	}

	fmt.Printf("[+++] Успех! Сценарий сохранен: %s\n", outputPath)
	return nil
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
