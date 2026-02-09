package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gen2brain/go-fitz"
	"github.com/ivlev/pdf2video/internal/config"
	"github.com/ivlev/pdf2video/internal/effects"
	"github.com/ivlev/pdf2video/internal/engine"
	"github.com/ivlev/pdf2video/internal/pdf"
	"github.com/ivlev/pdf2video/internal/system"
	"github.com/ivlev/pdf2video/internal/video"
)

func main() {
	// Увеличиваем лимиты системы (для macOS/Linux)
	system.InitResourceLimits()

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
		latest, err := system.FindLatestPDF("input/pdf")
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
		latest, err := system.FindLatestAudio("input/audio")
		if err == nil {
			audioPath = latest
			fmt.Printf("[*] Выбрано аудио: %s\n", audioPath)
		}
	}

	if audioPath != "" && *audioSyncPtr {
		audioDur, err := system.GetAudioDuration(audioPath)
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

	finalOutput := *outputPtr
	if finalOutput == "" {
		baseName := filepath.Base(inputPath)
		ext := filepath.Ext(baseName)
		nameOnly := strings.TrimSuffix(baseName, ext)
		cleanName := strings.ReplaceAll(nameOnly, " ", "_")
		timestamp := time.Now().Format("2006-01-02_15-04-05")
		finalOutput = filepath.Join("output", fmt.Sprintf("%s_%s.mp4", cleanName, timestamp))
	}

	cfg := &config.Config{
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

	pdfSource, err := pdf.NewFitzPDFSource(cfg.InputPDF)
	if err != nil {
		log.Fatalf("[-] Ошибка открытия PDF: %v", err)
	}
	defer pdfSource.Close()

	// Инициализируем зависимости
	ve := &video.FFmpegEncoder{}
	eff := &effects.DefaultEffect{}

	project := engine.NewVideoProject(cfg, pdfSource, ve, eff)
	if err := project.Run(); err != nil {
		log.Fatalf("[-] Ошибка проекта: %v", err)
	}

	fmt.Printf("[+++] Успех! Результат: %s\n", cfg.OutputVideo)
}
