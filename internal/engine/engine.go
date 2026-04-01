package engine

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"log"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ivlev/pdf2video/internal/analyzer"
	"github.com/ivlev/pdf2video/internal/config"
	"github.com/ivlev/pdf2video/internal/director"
	"github.com/ivlev/pdf2video/internal/effects"
	"github.com/ivlev/pdf2video/internal/source"
	"github.com/ivlev/pdf2video/internal/system"
	"github.com/ivlev/pdf2video/internal/video"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
	"golang.org/x/sync/errgroup"
)

type VideoProject struct {
	Config  *config.Config
	Source  source.Source
	Encoder video.VideoEncoder
	Effect  effects.Effect
	tempDir string
	ctx     context.Context
	cancel  context.CancelFunc
	cache   *system.RenderCache
	memory  *system.MemoryManager
}

func NewVideoProject(cfg *config.Config, src source.Source, ve video.VideoEncoder, eff effects.Effect) *VideoProject {
	ctx, cancel := context.WithCancel(context.Background())
	return &VideoProject{
		Config:  cfg,
		Source:  src,
		Encoder: ve,
		Effect:  eff,
		ctx:     ctx,
		cancel:  cancel,
		cache:   system.NewRenderCache("cache/renders"),
		memory:  system.NewMemoryManager(cfg.MaxMemoryMB),
	}
}

func (p *VideoProject) Run(ctx context.Context) error {
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
		select {
		case <-p.ctx.Done():
			return p.ctx.Err()
		default:
		}
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

	// Использование errgroup для управления пайплайном
	g, gCtx := errgroup.WithContext(ctx)

	// 1. Unified Pipeline (Balanced CPU/GPU)
	// Мы объединяем рендеринг и кодирование в один воркер, чтобы обеспечить
	// непрерывную загрузку системы и избежать простоев CPU, пока GPU занят.
	frameSize := system.GetFrameSize(p.Config.Width, p.Config.Height)
	numWorkers := p.memory.GetRecommendedWorkers(frameSize, p.Config.Workers)

	if numWorkers > pageCount {
		numWorkers = pageCount
	}

	// Trace scenario collection
	traceScenario := &director.Scenario{
		Version: "1.0-trace",
		Slides:  make([]director.Slide, pageCount),
	}
	var traceMu sync.Mutex

	renderStart = time.Now()
	encodeStart = renderStart

	bar := system.NewProgressBar(100, "[*] Generating Video")
	renderedCount := 0
	mu := sync.Mutex{}

	for w := 0; w < numWorkers; w++ {
		g.Go(func() error {
			for i := range jobs {
				select {
				case <-gCtx.Done():
					return gCtx.Err()
				default:
				}

				// --- STAGE 1: RENDER (CPU) ---
				if err := p.memory.Acquire(gCtx, frameSize); err != nil {
					return err
				}

				dpi := p.calculateOptimalDPI(i)
				pageHash, hashErr := p.Source.GetPageHash(i)
				if hashErr != nil {
					p.memory.Release(frameSize)
					return fmt.Errorf("error hashing page %d: %w", i, hashErr)
				}

				cacheKey := p.cache.GetKey(pageHash, i, dpi)
				var img image.Image
				var err error

				if cachedImg, found := p.cache.Get(cacheKey); found {
					img = cachedImg
				} else {
					img, err = p.Source.RenderPage(i, dpi)
					if err == nil {
						_ = p.cache.Put(cacheKey, img)
					}
				}

				if err != nil {
					p.memory.Release(frameSize)
					return fmt.Errorf("error rendering page %d: %w", i, err)
				}

				// --- STAGE 2: ENCODE (GPU/CPU) ---
				segPath := filepath.Join(p.tempDir, fmt.Sprintf("s%d.mp4", i))
				duration := p.Config.PageDurations[i]
				params := config.SegmentParams{
					Width:         p.Config.Width,
					Height:        p.Config.Height,
					FPS:           p.Config.FPS,
					Duration:      duration,
					ZoomMode:      p.Config.ZoomMode,
					ZoomSpeed:     p.Config.ZoomSpeed,
					FadeDuration:  p.Config.FadeDuration,
					OutroDuration: p.Config.OutroDuration,
					PageIndex:     i,
					Debug:         p.Config.Debug,
					Trace:         p.Config.Trace,
					TraceColor:    p.Config.TraceColor,
				}
				params.Filter = p.Effect.GenerateFilter(params)

				// Debug/Trace drawing
				if p.Config.Debug || p.Config.Trace {
					keyframes := p.Effect.GenerateKeyframes(params)
					traceMu.Lock()
					traceScenario.Slides[i] = director.Slide{
						ID:        i + 1,
						Duration:  duration,
						Keyframes: keyframes,
					}
					traceMu.Unlock()

					if se, ok := p.Effect.(*effects.ScenarioEffect); ok {
						newImg := p.debugDrawScenario(img, se, i, p.Config.Debug, p.Config.Trace)
						if newImg != img {
							if oldRgba, ok := img.(*image.RGBA); ok {
								system.PutImage(oldRgba)
							}
							img = newImg
						}
					}
				}

				encErr := p.Encoder.EncodeSegment(gCtx, img, segPath, params, p.Config.VideoEncoder, p.Config.Quality)

				// Сразу освобождаем память после кодирования
				if rgba, ok := img.(*image.RGBA); ok {
					system.PutImage(rgba)
					p.memory.Release(frameSize)
				}

				if encErr != nil {
					return fmt.Errorf("encode error page %d: %w", i, encErr)
				}

				results[i] = segPath
				mu.Lock()
				renderedCount++
				// Phase 1: 0-70%
				progress := (float64(renderedCount) / float64(pageCount)) * 70.0
				bar.Update(int(progress))
				mu.Unlock()
			}
			return nil
		})
	}

	// Запускаем задачи рендеринга
	g.Go(func() error {
		defer close(jobs)
		for i := 0; i < pageCount; i++ {
			select {
			case jobs <- i:
			case <-gCtx.Done():
				return gCtx.Err()
			}
		}
		return nil
	})

	// Ждем завершения всех воркеров через errgroup
	if err := g.Wait(); err != nil {
		return err
	}
	renderEnd = time.Now() // На самом деле это конец всего пайплайна
	encodeEnd = renderEnd

	// Сохранение лога трассировки камеры, если включен Debug или Trace
	if p.Config.Debug || p.Config.Trace {
		timestamp := time.Now().Format("2006-01-02_15-04-05")
		tracePath := filepath.Join("internal/scenarios", fmt.Sprintf("trace_%s.yaml", timestamp))
		os.MkdirAll(filepath.Dir(tracePath), 0755)
		if err := director.WriteScenario(traceScenario, tracePath); err == nil {
			fmt.Printf("[*] Лог трассировки камеры сохранен: %s\n", tracePath)
		} else {
			fmt.Printf("[!] Ошибка сохранения лога трассировки: %v\n", err)
		}
	}

	// Проверяем, был ли процесс прерван
	select {
	case <-p.ctx.Done():
		return fmt.Errorf("процесс прерван пользователем")
	default:
	}

	// Проверяем, все ли сегменты готовы
	for i, r := range results {
		if r == "" {
			return fmt.Errorf("сегмент %d не был создан. Проверьте логи FFmpeg", i)
		}
	}

	var finalSegments []config.VideoSegment
	audioDelayMs := 0

	if p.Config.BlackScreenDuration > 0 {
		introPath := filepath.Join(p.tempDir, "intro_black.mp4")
		introDur := p.Config.BlackScreenDuration + p.Config.FadeDuration
		fmt.Printf("[*] Создание интро (черный экран, %.2fs)\n", introDur)
		if err := p.generateBlackSegment(p.ctx, introDur, introPath); err != nil {
			return fmt.Errorf("ошибка создания интро: %v", err)
		}

		finalSegments = append(finalSegments, config.VideoSegment{
			Path:           introPath,
			Duration:       introDur,
			TransitionType: "none",
			FadeDuration:   0,
		})

		audioDelayMs = int(p.Config.BlackScreenDuration * 1000)
	}

	for i, r := range results {
		transType := p.Config.TransitionType
		fadeDur := p.Config.FadeDuration

		if i == 0 && p.Config.BlackScreenDuration > 0 {
			transType = p.Config.BlackScreenTransition
		} else if i == 0 {
			transType = "none"
			fadeDur = 0
		}

		finalSegments = append(finalSegments, config.VideoSegment{
			Path:           r,
			Duration:       p.Config.PageDurations[i],
			TransitionType: transType,
			FadeDuration:   fadeDur,
		})
	}

	if p.Config.BlackScreenDuration > 0 {
		outroPath := filepath.Join(p.tempDir, "outro_black.mp4")
		outroDur := p.Config.BlackScreenDuration + p.Config.FadeDuration
		fmt.Printf("[*] Создание аутро (черный экран, %.2fs)\n", outroDur)
		if err := p.generateBlackSegment(p.ctx, outroDur, outroPath); err != nil {
			return fmt.Errorf("ошибка создания аутро: %v", err)
		}

		finalSegments = append(finalSegments, config.VideoSegment{
			Path:           outroPath,
			Duration:       outroDur,
			TransitionType: p.Config.BlackScreenTransition,
			FadeDuration:   p.Config.FadeDuration,
		})
	}

	if p.Config.QREnabled && p.Config.QRURL != "" {
		fmt.Printf("[*] Создание сквозного QR-кода: %s\n", p.Config.QRURL)
		qrPath, err := effects.GenerateQRCode(p.Config.QRURL, p.Config.QRSize, p.tempDir)
		if err != nil {
			return fmt.Errorf("ошибка создания qr-кода: %v", err)
		}
		p.Config.QRCodePath = qrPath
	}

	fmt.Println("[*] Сборка финального видео (с эффектами переходов)...")

	concatStart = time.Now()
	err = p.Encoder.Concatenate(p.ctx, finalSegments, p.Config.OutputVideo, p.tempDir, *p.Config, audioDelayMs, func(current, total float64) {
		p1 := 70.0
		p2 := 30.0
		progress := p1 + (current/total)*p2
		bar.Update(int(progress + 0.5))
	})
	if err != nil {
		select {
		case <-p.ctx.Done():
			return fmt.Errorf("сборка прервана пользователем")
		default:
		}
		return fmt.Errorf("ошибка сборки финального видео: %v", err)
	}

	totalTime := time.Since(startTime)
	renderTime := renderEnd.Sub(renderStart)
	encodeTime := encodeEnd.Sub(encodeStart)
	concatTime := time.Since(concatStart)
	fps := float64(pageCount) / totalTime.Seconds()

	if p.Config.ShowStats {
		report := fmt.Sprintf(
			"\n--- [PERFORMANCE REPORT] ---\n"+
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
		logEntry := fmt.Sprintf("[%s] Build: %s | Input: %s | Pages: %d | Total: %.2fs | Render: %.2fs | Encode: %.2fs | Concat: %.2fs | FPS: %.2f\n",
			time.Now().Format("2006-01-02 15:04:05"),
			p.Config.BuildVersion,
			filepath.Base(p.Config.InputPath),
			pageCount,
			totalTime.Seconds(),
			renderTime.Seconds(),
			encodeTime.Seconds(),
			concatTime.Seconds(),
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

func (p *VideoProject) generateBlackSegment(ctx context.Context, duration float64, outputPath string) error {
	img := system.GetImage(image.Rect(0, 0, p.Config.Width, p.Config.Height))
	defer system.PutImage(img)
	draw.Draw(img, img.Bounds(), &image.Uniform{color.Black}, image.Point{}, draw.Src)

	// Для простого черного фона (raw image -> encoder) не нужно использовать zoompan, если мы
	// передаем один кадр в ffmpeg? А нет, EncodeSegment генерирует видео из 1 raw-кадра:
	// -t duration и -filter_script. Мы можем использовать базовый null-фильтр или просто
	// простую копию. Но `zoompan` тоже подойдет, чтобы кадр клонировался до нужной длины.
	filter := fmt.Sprintf("zoompan=z=1.0:x=0:y=0:d=%d:s=%dx%d:fps=%d", int(duration*float64(p.Config.FPS)), p.Config.Width, p.Config.Height, p.Config.FPS)

	params := config.SegmentParams{
		Width:         p.Config.Width,
		Height:        p.Config.Height,
		FPS:           p.Config.FPS,
		Duration:      duration,
		ZoomMode:      "center",
		ZoomSpeed:     0,
		FadeDuration:  0,
		OutroDuration: 0,
		Filter:        filter,
	}

	return p.Encoder.EncodeSegment(ctx, img, outputPath, params, p.Config.VideoEncoder, p.Config.Quality)
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
	p.Source.SetDPI(p.Config.DPI)

	// Используем Director для генерации путей камеры
	dir := director.NewDirector(p.Config.Width, p.Config.Height)

	// Smart Analysis Logic
	hasText := p.hasTextLayer()
	finalMode := p.Config.AnalyzeMode

	if finalMode == "auto" {
		if hasText {
			finalMode = "ocr"
			fmt.Println("[*] Автоопределение: найден слой текста, используется режим \"ocr\"")
		} else {
			finalMode = "contrast"
			fmt.Println("[*] Автоопределение: слой текста не найден, используется режим \"contrast\"")
		}
	} else if finalMode == "ocr" && !hasText {
		fmt.Println("[!] Предупреждение: слой текста не найден. Переключение в режим \"contrast\".")
		finalMode = "contrast"
	} else if finalMode == "contrast" && hasText {
		fmt.Println("[!] Предупреждение: в PDF найден слой текста. Возможно, режим \"ocr\" даст лучший результат.")
	}

	// Инициализируем детектор на основе выбранного режима
	det, err := analyzer.NewDetector(finalMode)
	if err != nil {
		return fmt.Errorf("ошибка инициализации детектора (%s): %v", finalMode, err)
	}

	// Настройка параметров, если детектор их поддерживает
	if cdet, ok := det.(*analyzer.ContrastDetector); ok {
		cdet.MinBlockArea = p.Config.MinBlockArea
		cdet.EdgeThreshold = p.Config.EdgeThreshold
	} else if edet, ok := det.(*analyzer.EnhancedDetector); ok {
		edet.MinBlockArea = p.Config.MinBlockArea
		edet.EdgeThreshold = p.Config.EdgeThreshold
	}

	var slides []director.Slide
	for i := 0; i < pageCount; i++ {
		select {
		case <-p.ctx.Done():
			return p.ctx.Err()
		default:
		}
		fmt.Printf("[*] Анализ страницы %d/%d...\n", i+1, pageCount)

		var img image.Image
		var renderErr error

		// Для анализа используем DPI из конфига (или адаптивный, если захотим)
		dpi := p.Config.DPI
		pageHash, _ := p.Source.GetPageHash(i)
		frameSize := system.GetFrameSize(p.Config.Width, p.Config.Height)
		cacheKey := p.cache.GetKey(pageHash, i, dpi)

		// Бронируем память (даже для кэша, т.к. Get() выделяет из пула)
		if errAcq := p.memory.Acquire(p.ctx, frameSize); errAcq != nil {
			return errAcq
		}

		if cachedImg, found := p.cache.Get(cacheKey); found {
			img = cachedImg
		} else {
			img, renderErr = p.Source.RenderPage(i, dpi)
			if renderErr == nil {
				_ = p.cache.Put(cacheKey, img)
			}
		}

		// Если это OCR-детектор, обновляем контекст (источник и страницу)
		if odet, ok := det.(*analyzer.OCRDetector); ok {
			odet.Source = p.Source
			odet.PageIndex = i
		}

		// Поиск блоков на изображении
		blocks, err := det.Detect(img)
		if rgba, ok := img.(*image.RGBA); ok {
			system.PutImage(rgba)
			p.memory.Release(frameSize)
		} else if img != nil {
			// Если вдруг не RGBA, все равно освобождаем бюджет
			p.memory.Release(frameSize)
		}

		if err != nil {
			log.Printf("[!] Ошибка анализа страницы %d: %v", i, err)
			// Продолжаем с пустым списком блоков
		}

		// Генерация сценария для конкретной страницы (слайда)
		slideDuration := 5.0
		if len(p.Config.PageDurations) > i {
			slideDuration = p.Config.PageDurations[i]
		}

		slideScenario, err := dir.GenerateScenario(blocks, fmt.Sprintf("slide_%d.png", i+1), slideDuration, p.Config.FadeDuration, p.Config.OutroDuration)
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

	err = director.WriteScenario(scenario, outputPath)
	if err != nil {
		return err
	}

	fmt.Printf("[+++] Успех! Сценарий сохранен: %s\n", outputPath)
	return nil
}

func (p *VideoProject) debugDrawScenario(img image.Image, se *effects.ScenarioEffect, pageIndex int, drawDebug, drawTrace bool) image.Image {
	if pageIndex >= len(se.Scenario.Slides) {
		return img
	}
	slide := se.Scenario.Slides[pageIndex]

	// Create a writable copy from the buffer pool
	bounds := img.Bounds()
	rgba := system.GetImage(bounds)
	draw.Draw(rgba, bounds, img, bounds.Min, draw.Src)

	// Draw rectangles for each keyframe
	red := color.RGBA{255, 0, 0, 255}
	if drawDebug {
		for _, kf := range slide.Keyframes {
			p.drawHollowRect(rgba, kf.Rect, red)
		}
	}

	if drawTrace {
		p.drawTracePath(rgba, slide.Keyframes, p.Config.TraceColor)
	}

	return rgba
}

func (p *VideoProject) drawTracePath(img *image.RGBA, keyframes []director.Keyframe, traceColorStr string) {
	if len(keyframes) < 2 {
		return
	}

	traceColor := color.RGBA{0, 255, 0, 255} // Green for trace path
	dotColor := color.RGBA{0, 0, 255, 255}   // Blue for stop points
	textColor, err := system.ParseHexColor(traceColorStr)
	if err != nil {
		textColor = color.RGBA{255, 255, 255, 255} // Fallback to White
	}
	dotRadius := 8

	// Draw lines between centers of keyframes
	for i := 0; i < len(keyframes)-1; i++ {
		start := p.getCenter(keyframes[i])
		end := p.getCenter(keyframes[i+1])
		p.drawLine(img, start, end, traceColor, 3)
	}

	// Prepare font drawer
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(textColor),
		Face: basicfont.Face7x13,
	}

	// Draw dots for each stop point (keyframe center)
	for _, kf := range keyframes {
		center := p.getCenter(kf)
		p.drawCircle(img, center, dotRadius, dotColor)

		// Draw coordinates near the dot
		label := fmt.Sprintf("(%d, %d)", center.X, center.Y)
		d.Dot = fixed.Point26_6{
			X: fixed.I(center.X + 10),
			Y: fixed.I(center.Y - 10),
		}
		d.DrawString(label)
	}
}

func (p *VideoProject) getCenter(kf director.Keyframe) image.Point {
	return image.Point{
		X: int(kf.Rect.X + kf.Rect.W/2),
		Y: int(kf.Rect.Y + kf.Rect.H/2),
	}
}

// Simple Bresenham line drawing
func (p *VideoProject) drawLine(img *image.RGBA, start, end image.Point, c color.Color, thickness int) {
	x0, y0 := start.X, start.Y
	x1, y1 := end.X, end.Y

	dx := x1 - x0
	if dx < 0 {
		dx = -dx
	}
	dy := y1 - y0
	if dy < 0 {
		dy = -dy
	}

	sx, sy := 1, 1
	if x0 >= x1 {
		sx = -1
	}
	if y0 >= y1 {
		sy = -1
	}

	err := dx - dy

	for {
		// Draw thick point
		for i := -thickness / 2; i <= thickness/2; i++ {
			for j := -thickness / 2; j <= thickness/2; j++ {
				img.Set(x0+i, y0+j, c)
			}
		}

		if x0 == x1 && y0 == y1 {
			break
		}

		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x0 += sx
		}
		if e2 < dx {
			err += dx
			y0 += sy
		}
	}
}

// Simple circle drawing (filled)
func (p *VideoProject) drawCircle(img *image.RGBA, center image.Point, radius int, c color.Color) {
	cx, cy := center.X, center.Y
	for y := -radius; y <= radius; y++ {
		for x := -radius; x <= radius; x++ {
			if x*x+y*y <= radius*radius {
				img.Set(cx+x, cy+y, c)
			}
		}
	}
}

func (p *VideoProject) drawHollowRect(img *image.RGBA, r director.Rectangle, c color.Color) {
	thickness := 4
	x := int(r.X)
	y := int(r.Y)
	w := int(r.W)
	h := int(r.H)

	// Top
	draw.Draw(img, image.Rect(x, y, x+w, y+thickness), &image.Uniform{c}, image.Point{}, draw.Src)
	// Bottom
	draw.Draw(img, image.Rect(x, y+h-thickness, x+w, y+h), &image.Uniform{c}, image.Point{}, draw.Src)
	// Left
	draw.Draw(img, image.Rect(x, y, x+thickness, y+h), &image.Uniform{c}, image.Point{}, draw.Src)
	// Right
	draw.Draw(img, image.Rect(x+w-thickness, y, x+w, y+h), &image.Uniform{c}, image.Point{}, draw.Src)
}
func (p *VideoProject) calculateOptimalDPI(index int) int {
	srcW, srcH, err := p.Source.GetPageDimensions(index)
	if err != nil {
		return p.Config.DPI
	}

	// Вычисляем необходимый DPI для целевого разрешения
	// PDF points (1/72 inch).
	// targetPixels = (sourcePoints / 72) * requiredDPI
	// requiredDPI = (targetPixels * 72) / sourcePoints

	targetW := float64(p.Config.Width)
	targetH := float64(p.Config.Height)

	// Учитываем, что страница может быть повернута или иметь другие пропорции.
	// Берем максимум из требуемого DPI по ширине и высоте.
	requiredDPI_W := (targetW * 72.0) / srcW
	requiredDPI_H := (targetH * 72.0) / srcH

	requiredDPI := requiredDPI_W
	if requiredDPI_H > requiredDPI {
		requiredDPI = requiredDPI_H
	}

	// Если есть зум, нам нужно больше пикселей для сохранения четкости.
	// Учитываем максимальный зум в 2x (стандарт для Ken Burns в нашем проекте)
	requiredDPI *= 1.5 // Коэффициент запаса для зума

	// Ограничиваем диапазон: минимум 72 (экран), максимум 300 (типичный конфиг пользователя)
	// но не больше, чем задал пользователь в конфиге, если он хочет меньше.
	minDPI := 150.0
	maxDPI := float64(p.Config.DPI)
	if maxDPI <= 0 {
		maxDPI = 1200 // Absolute reasonable maximum for auto-DPI
	}
	if maxDPI < minDPI {
		minDPI = maxDPI
	}

	result := int(math.Ceil(requiredDPI))
	if float64(result) < minDPI {
		return int(minDPI)
	}
	if float64(result) > maxDPI {
		return int(maxDPI)
	}

	return result
}

func (p *VideoProject) hasTextLayer() bool {
	// Проверяем первые несколько страниц на наличие текста (для экономии времени)
	checkPages := p.Source.PageCount()
	if checkPages > 3 {
		checkPages = 3
	}

	for i := 0; i < checkPages; i++ {
		if p.Source.HasTextLayer(i) {
			return true
		}
	}
	return false
}
