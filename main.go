package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gen2brain/go-fitz"
)

//---------------------------------------------------------
// 1. Интерфейсы (Абстракция)
//---------------------------------------------------------

type PDFSource interface {
	PageCount() int
	GetPageDimensions(index int) (width, height float64, err error)
	RenderPage(index int, dpi int) (image.Image, error)
	Close() error
}

type VideoEncoder interface {
	EncodeSegment(imagePath, videoPath string, params SegmentParams) error
	Concatenate(segmentPaths []string, finalPath string) error
}

type Effect interface {
	GenerateFilter(params SegmentParams) string
}

//---------------------------------------------------------
// 2. Модели и Конфигурация
//---------------------------------------------------------

type Config struct {
	InputPDF       string
	OutputVideo    string
	TotalDuration  float64
	Width          int
	Height         int
	FPS            int
	Workers        int
	FadeDuration   float64
	TransitionType string
	ZoomMode       string
	ZoomSpeed      float64
	DPI            int
	AudioPath      string
	Preset         string
}

type SegmentParams struct {
	Width, Height int
	FPS           int
	Duration      float64
	ZoomMode      string
	ZoomSpeed     float64
	FadeDuration  float64
	PageIndex     int
}

//---------------------------------------------------------
// 3. Реализации
//---------------------------------------------------------

// FitzPDFSource реализует PDFSource через go-fitz
type FitzPDFSource struct {
	doc  *fitz.Document
	path string
}

func NewFitzPDFSource(path string) (*FitzPDFSource, error) {
	doc, err := fitz.New(path)
	if err != nil {
		return nil, err
	}
	return &FitzPDFSource{doc: doc, path: path}, nil
}

func (f *FitzPDFSource) PageCount() int {
	return f.doc.NumPage()
}

func (f *FitzPDFSource) GetPageDimensions(index int) (float64, float64, error) {
	rect, err := f.doc.Bound(index)
	if err != nil {
		return 0, 0, err
	}
	return float64(rect.Dx()), float64(rect.Dy()), nil
}

func (f *FitzPDFSource) RenderPage(index int, dpi int) (image.Image, error) {
	// Для параллельной работы нужно открывать новый документ или использовать Mutex
	// Здесь мы открываем новый, чтобы не блокировать воркеров
	workerDoc, err := fitz.New(f.path)
	if err != nil {
		return nil, err
	}
	defer workerDoc.Close()
	return workerDoc.ImageDPI(index, float64(dpi))
}

func (f *FitzPDFSource) Close() error {
	return f.doc.Close()
}

// FFmpegEncoder реализует VideoEncoder через системный FFmpeg
type FFmpegEncoder struct{}

func (e *FFmpegEncoder) EncodeSegment(imagePath, videoPath string, params SegmentParams) error {
	// Составляем цепочку фильтров из параметров (в реальном коде сюда можно передать Effect)
	// Для простоты пока оставим формирование фильтра внутри, имитируя поведение Effect

	// Это заглушка, фильтр будем брать из интерфейса Effect в VideoProject
	return nil // Метод используется выше в VideoProject
}

func (e *FFmpegEncoder) Concatenate(segmentPaths []string, finalPath string, tmpDir string, params Config) error {
	if params.TransitionType == "" || params.TransitionType == "none" || len(segmentPaths) < 2 {
		// Стандартная быстрая склейка без эффектов
		concatFilePath := filepath.Join(tmpDir, "inputs.txt")
		f, err := os.Create(concatFilePath)
		if err != nil {
			return err
		}
		for _, p := range segmentPaths {
			absPath, _ := filepath.Abs(p)
			fmt.Fprintf(f, "file '%s'\n", absPath)
		}
		f.Close()

		cmd := exec.Command("ffmpeg", "-y",
			"-f", "concat", "-safe", "0", "-i", concatFilePath,
			"-c", "copy", finalPath,
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("ffmpeg concat error: %v, output: %s", err, string(out))
		}
		return nil
	}

	// Сложная склейка с xfade
	pageDuration := params.TotalDuration / float64(len(segmentPaths))
	fadeDuration := params.FadeDuration

	args := []string{"-y"}
	for _, p := range segmentPaths {
		args = append(args, "-i", p)
	}

	// Добавляем аудио в список входов, если оно есть
	audioIndex := -1
	if params.AudioPath != "" {
		audioIndex = len(segmentPaths)
		args = append(args, "-i", params.AudioPath)
	}

	filterGraph := ""
	lastOut := "[0:v]"
	offset := pageDuration - fadeDuration

	for i := 1; i < len(segmentPaths); i++ {
		nextIn := fmt.Sprintf("[%d:v]", i)
		outName := fmt.Sprintf("[v%d]", i)
		filterGraph += fmt.Sprintf("%s%sxfade=transition=%s:duration=%f:offset=%f%s;",
			lastOut, nextIn, params.TransitionType, fadeDuration, offset, outName)
		lastOut = outName
		offset += pageDuration - fadeDuration
	}
	// Удаляем последнюю точку с запятой и добавляем финальный маппинг
	filterGraph = strings.TrimSuffix(filterGraph, ";")

	args = append(args, "-filter_complex", filterGraph)
	args = append(args, "-map", lastOut)

	if audioIndex != -1 {
		// Мапим аудио по его индексу в списке входов
		args = append(args, "-map", fmt.Sprintf("%d:a", audioIndex), "-shortest")
	}

	args = append(args, "-c:v", "libx264", "-pix_fmt", "yuv420p", "-preset", "medium", finalPath)

	cmd := exec.Command("ffmpeg", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg xfade error: %v, output: %s", err, string(out))
	}
	return nil
}

// DefaultEffect реализует стандартный набор эффектов проекта
type DefaultEffect struct{}

func (e *DefaultEffect) GenerateFilter(p SegmentParams) string {
	// 1. Логика выбора направления зума
	mode := strings.ToLower(p.ZoomMode)
	if mode == "random" || mode == "out-random" {
		modes := []string{"center", "top-left", "top-right", "bottom-left", "bottom-right"}
		r := rand.New(rand.NewSource(time.Now().UnixNano() + int64(p.PageIndex*99)))
		mode = modes[r.Intn(len(modes))]
	}

	var zoomX, zoomY string
	switch mode {
	case "top-left":
		zoomX, zoomY = "0", "0"
	case "top-right":
		zoomX, zoomY = "iw-(iw/zoom)", "0"
	case "bottom-left":
		zoomX, zoomY = "0", "ih-(ih/zoom)"
	case "bottom-right":
		zoomX, zoomY = "iw-(iw/zoom)", "ih-(ih/zoom)"
	default: // center
		zoomX, zoomY = "iw/2-(iw/zoom/2)", "ih/2-(ih/zoom/2)"
	}

	// 2. Рассчитываем тайминги в кадрах (для стабильной анимации PNG используем 'on')
	fFPS := float64(p.FPS)
	fTotal := p.Duration * fFPS
	fFade := p.FadeDuration * fFPS
	fActive := fTotal - fFade
	if fActive <= 0 {
		fActive = fTotal
	}

	// Точка смены направления (peak)
	// zoom_diff = 0.5 (от 1.0 до 1.5). frames_to_hit = 0.5 / ZoomSpeed
	zSpeed := p.ZoomSpeed
	if zSpeed <= 0 {
		zSpeed = 0.001
	}

	onPeak := 0.5 / zSpeed
	// Ограничиваем пик половиной активного времени, чтобы успеть вернуться
	if onPeak > fActive/2 {
		onPeak = fActive / 2
	}

	actualPeak := 1.0 + (zSpeed * onPeak)
	if actualPeak > 1.5 {
		actualPeak = 1.5
		onPeak = 0.5 / zSpeed
	}

	// Формула "Дыхания": плавно до peak, затем плавно возврат к 1.0 до начала xfade
	// FFmpeg zoompan formula:
	zFormula := fmt.Sprintf("if(lte(on,%f), 1.0+(%f*on), if(lte(on,%f), %f-(%f-1.0)*(on-%f)/(%f-%f), 1.0))",
		onPeak, zSpeed, fActive, actualPeak, actualPeak, onPeak, fActive, onPeak)

	aspectFilter := fmt.Sprintf(
		"scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2",
		p.Width*2, p.Height*2, p.Width*2, p.Height*2,
	)

	zoomFilter := fmt.Sprintf(
		"zoompan=z='%s':d=%d:s=%dx%d:x='%s':y='%s'",
		zFormula, int(fTotal), p.Width, p.Height, zoomX, zoomY,
	)

	return fmt.Sprintf("%s,%s,scale=%d:%d", aspectFilter, zoomFilter, p.Width, p.Height)
}

//---------------------------------------------------------
// 4. Оркестратор
//---------------------------------------------------------

type VideoProject struct {
	Config  *Config
	PDF     PDFSource
	Encoder *FFmpegEncoder
	Effect  Effect
	tempDir string
}

func NewVideoProject(cfg *Config, pdf PDFSource) *VideoProject {
	return &VideoProject{
		Config:  cfg,
		PDF:     pdf,
		Encoder: &FFmpegEncoder{},
		Effect:  &DefaultEffect{},
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

	// Коррекция FadeDuration для коротких слайдов
	if p.Config.FadeDuration >= pageDuration {
		p.Config.FadeDuration = pageDuration / 3.0
		fmt.Printf("[!] Переход уменьшен до %.2fs для соответствия слайдам\n", p.Config.FadeDuration)
	}

	// Авто-пропорции
	if p.Config.Width == 1280 && p.Config.Height == 720 {
		pdfW, pdfH, err := p.PDF.GetPageDimensions(0)
		if err == nil {
			p.Config.Width = int(float64(p.Config.Height) * (pdfW / pdfH))
			if p.Config.Width%2 != 0 {
				p.Config.Width++
			}
		}
	}

	fmt.Println("--- [PROJECT: OOP ENGINE] ---")
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

				params := SegmentParams{
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

// initResourceLimits пытается увеличить лимит открытых файлов
func initResourceLimits() {
	var rLimit syscall.Rlimit
	err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit)
	if err != nil {
		log.Printf("[!] Не удалось получить лимит файлов: %v", err)
		return
	}

	// Попробуем поставить 2048 или максимум, разрешенный системой
	rLimit.Cur = 2048
	if rLimit.Cur > rLimit.Max {
		rLimit.Cur = rLimit.Max
	}

	err = syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit)
	if err != nil {
		log.Printf("[!] Не удалось установить лимит файлов: %v (это может вызвать ошибку на больших PDF)", err)
	} else {
		fmt.Printf("[*] Системный лимит открытых файлов увеличен до %d\n", rLimit.Cur)
	}
}

// findLatestPDF ищет самый свежий PDF-файл в указанной директории
func findLatestPDF(dir string) (string, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}

	var latestFile string
	var latestTime time.Time

	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(strings.ToLower(f.Name()), ".pdf") {
			info, err := f.Info()
			if err != nil {
				continue
			}
			if info.ModTime().After(latestTime) {
				latestTime = info.ModTime()
				latestFile = filepath.Join(dir, f.Name())
			}
		}
	}

	if latestFile == "" {
		return "", fmt.Errorf("в папке %s не найдено PDF-файлов", dir)
	}

	return latestFile, nil
}

// findLatestAudio ищет самый свежий аудио-файл в указанной директории
func findLatestAudio(dir string) (string, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}

	extensions := []string{".mp3", ".wav", ".m4a", ".ogg", ".aac", ".flac"}
	var latestFile string
	var latestTime time.Time

	for _, f := range files {
		if f.IsDir() {
			continue
		}
		isAudio := false
		for _, ext := range extensions {
			if strings.HasSuffix(strings.ToLower(f.Name()), ext) {
				isAudio = true
				break
			}
		}
		if isAudio {
			info, err := f.Info()
			if err != nil {
				continue
			}
			if info.ModTime().After(latestTime) {
				latestTime = info.ModTime()
				latestFile = filepath.Join(dir, f.Name())
			}
		}
	}

	if latestFile == "" {
		return "", fmt.Errorf("в папке %s не найдено аудио-файлов", dir)
	}

	return latestFile, nil
}

// getAudioDuration получает длительность аудио через ffprobe
func getAudioDuration(path string) (float64, error) {
	cmd := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, err
	}

	var duration float64
	_, err = fmt.Sscanf(strings.TrimSpace(string(out)), "%f", &duration)
	if err != nil {
		return 0, err
	}

	return duration, nil
}

//---------------------------------------------------------
// 5. Main
//---------------------------------------------------------

func main() {
	// Увеличиваем лимиты системы (для macOS/Linux)
	initResourceLimits()

	// Создаем нужные директории, если их нет
	dirs := []string{"input/audio", "input/pdf", "output"}
	for _, d := range dirs {
		os.MkdirAll(d, 0755)
	}

	inputPtr := flag.String("input", "", "Путь к PDF (по умолчанию: самый свежий файл в input/pdf/)")
	outputPtr := flag.String("output", "", "Путь к видео (если пусто, генерируется автоматически в output/)")
	durationPtr := flag.Float64("duration", 0, "Общая длительность видео (если 0, рассчитывается из -page-duration)")
	pageDurationPtr := flag.Float64("page-duration", 0.3, "Длительность показа одной страницы в секундах")
	widthPtr := flag.Int("width", 1280, "Ширина")
	heightPtr := flag.Int("height", 720, "Высота")
	fpsPtr := flag.Int("fps", 30, "FPS")
	workersPtr := flag.Int("workers", runtime.NumCPU(), "Потоки")
	fadePtr := flag.Float64("fade", 0.5, "Длительность перехода (сек)")
	transitionPtr := flag.String("transition", "fade", "Тип перехода xfade: fade, wipeleft, slideup, pixelize, circlecrop, dissolve, none")
	zoomPtr := flag.String("zoom-mode", "center", "Зум: center, top-left, top-right, bottom-left, bottom-right, random, out-center, out-random")
	zoomSpeedPtr := flag.Float64("zoom-speed", 0.001, "Скорость зума (например, 0.001)")
	dpiPtr := flag.Int("dpi", 300, "DPI")
	audioPtr := flag.String("audio", "", "Путь к аудио (по умолчанию: самый свежий файл в input/audio/)")
	audioSyncPtr := flag.Bool("audio-sync", true, "Синхронизировать длительность видео с аудио")
	presetPtr := flag.String("preset", "", "Пресет формата: 16:9, 9:16 (Shorts/TikTok), 4:5 (Instagram)")

	flag.Parse()

	width, height := *widthPtr, *heightPtr
	switch *presetPtr {
	case "16:9":
		width, height = 1280, 720
	case "9:16":
		width, height = 720, 1280
	case "4:5":
		width, height = 1080, 1350
	}

	inputPath := *inputPtr
	if inputPath == "" {
		latest, err := findLatestPDF("input/pdf")
		if err != nil {
			log.Fatalf("[-] Ошибка: %v. Положите PDF в input/pdf/", err)
		}
		inputPath = latest
		fmt.Printf("[*] Выбран файл: %s\n", inputPath)
	}

	// Инициализируем PDF для подсчета страниц
	pdfDoc, err := fitz.New(inputPath)
	if err != nil {
		log.Fatalf("[-] Ошибка открытия PDF: %v", err)
	}
	pageCount := pdfDoc.NumPage()
	pdfDoc.Close()

	totalDuration := *durationPtr

	// Обработка аудио
	audioPath := *audioPtr
	if audioPath == "" {
		// Пытаемся найти последнее аудио, но не валимся, если его нет
		latest, err := findLatestAudio("input/audio")
		if err == nil {
			audioPath = latest
			fmt.Printf("[*] Выбрано аудио: %s\n", audioPath)
		}
	}

	if audioPath != "" && *audioSyncPtr {
		audioDur, err := getAudioDuration(audioPath)
		if err == nil {
			totalDuration = audioDur
			fmt.Printf("[*] Длительность видео установлена по аудио: %.2fs\n", totalDuration)
		} else {
			log.Printf("[!] Не удалось получить длительность аудио: %v", err)
		}
	}

	if totalDuration <= 0 {
		totalDuration = float64(pageCount) * (*pageDurationPtr)
	}

	// Генерация имени выходного файла, если не задано
	finalOutput := *outputPtr
	if finalOutput == "" {
		baseName := filepath.Base(inputPath)
		ext := filepath.Ext(baseName)
		nameOnly := strings.TrimSuffix(baseName, ext)

		// Замена пробелов на подчеркивания
		cleanName := strings.ReplaceAll(nameOnly, " ", "_")

		// Добавление даты и времени
		timestamp := time.Now().Format("2006-01-02_15-04-05")
		finalOutput = filepath.Join("output", fmt.Sprintf("%s_%s.mp4", cleanName, timestamp))
	}

	cfg := &Config{
		InputPDF:       inputPath,
		OutputVideo:    finalOutput,
		TotalDuration:  totalDuration,
		Width:          width,
		Height:         height,
		FPS:            *fpsPtr,
		Workers:        *workersPtr,
		FadeDuration:   *fadePtr,
		TransitionType: *transitionPtr,
		ZoomMode:       *zoomPtr,
		ZoomSpeed:      *zoomSpeedPtr,
		DPI:            *dpiPtr,
		AudioPath:      audioPath,
		Preset:         *presetPtr,
	}

	pdf, err := NewFitzPDFSource(cfg.InputPDF)
	if err != nil {
		log.Fatalf("[-] Ошибка открытия PDF: %v", err)
	}
	defer pdf.Close()

	project := NewVideoProject(cfg, pdf)
	if err := project.Run(); err != nil {
		log.Fatalf("[-] Ошибка проекта: %v", err)
	}

	fmt.Printf("[+++] Успех! Результат: %s\n", cfg.OutputVideo)
}
