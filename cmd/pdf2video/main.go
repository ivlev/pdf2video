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

	"github.com/ivlev/pdf2video/internal/config"
	"github.com/ivlev/pdf2video/internal/effects"
	"github.com/ivlev/pdf2video/internal/engine"
	"github.com/ivlev/pdf2video/internal/source"
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

	inputPtr := flag.String("input", "", "Путь к PDF или папке с изображениями (по умолчанию: самый свежий файл в input/pdf/)")
	outputPtr := flag.String("output", "", "Путь к видео (если пусто, генерируется автоматически в output/)")
	durationPtr := flag.Float64("duration", 0, "Общая длительность видео (если 0, рассчитывается из -page-duration)")
	pageDurationPtr := flag.Float64("page-duration", 0.3, "Длительность показа одной страницы/изображения в секундах")
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
	qualityPtr := flag.Int("quality", 0, "Качество видео (0 - авто, x264: CRF 1-51, VideoToolbox: битрейт = Q*100кбит/с)")

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

	var src source.Source
	var err error

	if strings.HasSuffix(strings.ToLower(inputPath), ".pdf") {
		src, err = source.NewFitzPDFSource(inputPath)
	} else {
		src, err = source.NewImageSource(inputPath)
	}

	if err != nil {
		log.Fatalf("[-] Ошибка инициализации источника: %v", err)
	}
	defer src.Close()

	pageCount := src.PageCount()
	if pageCount == 0 {
		log.Fatalf("[-] Ошибка: в источнике нет страниц или изображений")
	}

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
		var nameSource string
		if strings.HasSuffix(strings.ToLower(inputPath), ".pdf") {
			nameSource = inputPath
		} else {
			if audioPath != "" {
				nameSource = audioPath
			} else {
				// Пытаемся найти самое свежее изображение для имени файла
				latestImg, err := system.FindLatestImage(inputPath)
				if err == nil {
					nameSource = latestImg
				} else {
					nameSource = inputPath
				}
			}
		}

		baseName := filepath.Base(nameSource)
		ext := filepath.Ext(baseName)
		nameOnly := strings.TrimSuffix(baseName, ext)
		cleanName := strings.ReplaceAll(nameOnly, " ", "_")
		timestamp := time.Now().Format("2006-01-02_15-04-05")
		finalOutput = filepath.Join("output", fmt.Sprintf("%s_%s.mp4", cleanName, timestamp))
	}

	encoderName, _ := system.GetBestH264Encoder()
	if encoderName != "libx264" {
		fmt.Printf("[*] Обнаружено аппаратное ускорение: %s\n", encoderName)
	}

	quality := *qualityPtr
	if quality == 0 {
		switch encoderName {
		case "h264_videotoolbox":
			quality = 75 // Хорошее качество для VideoToolbox
		case "h264_nvenc":
			quality = 28 // Эквивалент CRF для NVENC
		default:
			quality = 23 // Стандартный CRF для x264
		}
	}

	cfg := &config.Config{
		InputPath:      inputPath,
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
		VideoEncoder:   encoderName,
		Quality:        quality,
	}

	// Инициализируем зависимости
	ve := &video.FFmpegEncoder{}
	eff := &effects.DefaultEffect{}

	project := engine.NewVideoProject(cfg, src, ve, eff)
	if err := project.Run(); err != nil {
		log.Fatalf("[-] Ошибка проекта: %v", err)
	}

	fmt.Printf("[+++] Успех! Результат: %s\n", cfg.OutputVideo)
}
