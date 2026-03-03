package config

import (
	"flag"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ivlev/pdf2video/internal/system"
)

// Builder инкапсулирует логику сбора конфигурации из флагов и внешних условий
type Builder struct {
	flags *flag.FlagSet
	cfg   *Config

	// Временные переменные для флагов
	inputPtr            *string
	outputPtr           *string
	durationPtr         *float64
	pageDurationPtr     *float64
	widthPtr            *int
	heightPtr           *int
	fpsPtr              *int
	workersPtr          *int
	fadePtr             *float64
	transitionPtr       *string
	zoomPtr             *string
	zoomSpeedPtr        *float64
	dpiPtr              *int
	audioPtr            *string
	audioSyncPtr        *bool
	presetPtr           *string
	qualityPtr          *int
	statsPtr            *bool
	analyzeModePtr      *string
	minBlockAreaPtr     *int
	edgeThresholdPtr    *float64
	generateScenarioPtr *bool
	scenarioOutputPtr   *string
	scenarioInputPtr    *string
	outroDurationPtr    *float64
	bgAudioPtr          *string
	bgVolumePtr         *float64
	debugPtr            *bool
	blackScreenDurPtr   *float64
	blackScreenTransPtr *string
	tracePtr            *bool
	traceColorPtr       *string
	version             string
}

// NewBuilder создает новый экземпляр Builder
func NewBuilder(version string) *Builder {
	b := &Builder{
		flags:   flag.NewFlagSet("pdf2video", flag.ExitOnError),
		cfg:     &Config{},
		version: version,
	}
	b.defineFlags()
	return b
}

func (b *Builder) defineFlags() {
	b.inputPtr = b.flags.String("input", "", "Путь к PDF или папке с изображениями (по умолчанию: самый свежий файл в input/pdf/)")
	b.outputPtr = b.flags.String("output", "", "Путь к видео (если пусто, генерируется автоматически в output/)")
	b.durationPtr = b.flags.Float64("duration", 0, "Общая длительность видео (если 0, рассчитывается из -page-duration)")
	b.pageDurationPtr = b.flags.Float64("page-duration", 0.3, "Длительность показа одной страницы/изображения в секундах")
	b.widthPtr = b.flags.Int("width", 1280, "Ширина")
	b.heightPtr = b.flags.Int("height", 720, "Высота")
	b.fpsPtr = b.flags.Int("fps", 30, "FPS")
	b.workersPtr = b.flags.Int("workers", runtime.NumCPU(), "Потоки")
	b.fadePtr = b.flags.Float64("fade", 0.5, "Длительность перехода (сек)")
	b.transitionPtr = b.flags.String("transition", "fade", "Тип перехода xfade: fade, wipeleft, slideup, pixelize, circlecrop, dissolve, none")
	b.zoomPtr = b.flags.String("zoom-mode", "center", "Зум: center, top-left, top-right, bottom-left, bottom-right, random, out-center, out-random")
	b.zoomSpeedPtr = b.flags.Float64("zoom-speed", 0.001, "Скорость зума (например, 0.001)")
	b.dpiPtr = b.flags.Int("dpi", 300, "DPI")
	b.audioPtr = b.flags.String("audio", "", "Путь к аудио (по умолчанию: самый свежий файл в input/audio/)")
	b.audioSyncPtr = b.flags.Bool("audio-sync", true, "Синхронизировать длительность видео с аудио")
	b.presetPtr = b.flags.String("preset", "", "Пресет формата: 16:9, 9:16 (Shorts/TikTok), 4:5 (Instagram)")
	b.qualityPtr = b.flags.Int("quality", 0, "Качество видео (0 - авто, x264: CRF 1-51, VideoToolbox: битрейт = Q*100кбит/с)")
	b.statsPtr = b.flags.Bool("stats", false, "Вывести статистику производительности и записать в benchmark.log")
	b.analyzeModePtr = b.flags.String("analyze-mode", "contrast", "Режим анализа изображения: contrast (поиск границ), ocr (поиск текста)")
	b.minBlockAreaPtr = b.flags.Int("min-block-area", 500, "Минимальная площадь блока для детекции (в пикселях²)")
	b.edgeThresholdPtr = b.flags.Float64("edge-threshold", 30.0, "Порог чувствительности детектора границ (Sobel)")
	b.generateScenarioPtr = b.flags.Bool("generate-scenario", false, "Анализировать PDF и сгенерировать YAML-сценарий вместо видео")
	b.scenarioOutputPtr = b.flags.String("scenario-output", "", "Путь для сохранения сгенерированного сценария")
	b.scenarioInputPtr = b.flags.String("scenario", "", "Путь к YAML-сценарию для рендеринга видео с точным управлением камерой")
	b.outroDurationPtr = b.flags.Float64("outro-duration", 1.0, "Длительность возврата камеры к зуму 1:1 перед переходом (сек)")
	b.bgAudioPtr = b.flags.String("bg-audio", "", "Путь к фоновому аудио (по умолчанию: самый свежий файл в input/background/)")
	b.bgVolumePtr = b.flags.Float64("bg-volume", 0.3, "Громкость фонового аудио (0.0 - 1.0, по умолчанию 0.3)")
	b.debugPtr = b.flags.Bool("debug", false, "Режим отладки: показывать рамки отслеживания камеры")
	b.blackScreenDurPtr = b.flags.Float64("black-screen-duration", 2.0, "Длительность черного экрана в начале и в конце видео (сек)")
	b.blackScreenTransPtr = b.flags.String("black-screen-transition", "", "Переход для черного экрана (по умолчанию совпадает с -transition)")
	b.tracePtr = b.flags.Bool("trace", false, "Режим трассировки: показывать направление движения камеры и точки остановок")
	b.traceColorPtr = b.flags.String("trace-color", "#FFFFFF", "Цвет текста координат в режиме трассировки (HEX: #FFFFFF, #00FF00)")
}

// Build парсит флаги и собирает итоговую конфигурацию
func (b *Builder) Build(args []string) (*Config, error) {
	if err := b.flags.Parse(args); err != nil {
		return nil, err
	}

	c := b.cfg
	c.BuildVersion = b.version

	// Preset handling
	c.Width, c.Height = *b.widthPtr, *b.heightPtr
	switch *b.presetPtr {
	case "16:9":
		c.Width, c.Height = 1280, 720
	case "9:16":
		c.Width, c.Height = 720, 1280
	case "4:5":
		c.Width, c.Height = 1080, 1350
	}
	c.Preset = *b.presetPtr

	// Input handling
	c.InputPath = *b.inputPtr
	if c.InputPath == "" {
		latest, err := system.FindLatestPDF("input/pdf")
		if err != nil {
			return nil, fmt.Errorf("ошибка: %v. Положите PDF в input/pdf/", err)
		}
		c.InputPath = latest
	}

	// Audio handling
	c.AudioPath = *b.audioPtr
	if c.AudioPath == "" {
		latest, err := system.FindLatestAudio("input/audio")
		if err == nil {
			c.AudioPath = latest
		}
	}

	c.TotalDuration = *b.durationPtr
	if c.AudioPath != "" && *b.audioSyncPtr {
		audioDur, err := system.GetAudioDuration(c.AudioPath)
		if err == nil {
			c.TotalDuration = audioDur
		}
	}

	// Background audio
	c.BackgroundAudio = *b.bgAudioPtr
	if c.BackgroundAudio == "" {
		latest, err := system.FindLatestAudio("input/background")
		if err == nil {
			c.BackgroundAudio = latest
		}
	}
	c.BackgroundVolume = *b.bgVolumePtr

	// Encoder & Quality
	encoderName, _ := system.GetBestH264Encoder()
	c.VideoEncoder = encoderName
	c.Quality = *b.qualityPtr
	if c.Quality == 0 {
		switch encoderName {
		case "h264_videotoolbox":
			c.Quality = 75
		case "h264_nvenc":
			c.Quality = 28
		default:
			c.Quality = 23
		}
	}

	// Output handling
	c.OutputVideo = *b.outputPtr
	if c.OutputVideo == "" {
		c.OutputVideo = b.generateDefaultOutputPath(c)
	}

	// Misc
	c.FPS = *b.fpsPtr
	c.Workers = *b.workersPtr
	c.FadeDuration = *b.fadePtr
	c.TransitionType = *b.transitionPtr
	c.ZoomMode = *b.zoomPtr
	c.ZoomSpeed = *b.zoomSpeedPtr
	c.DPI = *b.dpiPtr
	c.ShowStats = *b.statsPtr
	c.AnalyzeMode = *b.analyzeModePtr
	c.MinBlockArea = *b.minBlockAreaPtr
	c.EdgeThreshold = *b.edgeThresholdPtr
	c.GenerateScenario = *b.generateScenarioPtr
	c.ScenarioOutput = *b.scenarioOutputPtr
	c.ScenarioInput = *b.scenarioInputPtr
	c.OutroDuration = *b.outroDurationPtr
	c.Debug = *b.debugPtr
	c.BlackScreenDuration = *b.blackScreenDurPtr
	c.BlackScreenTransition = *b.blackScreenTransPtr
	if c.BlackScreenTransition == "" {
		c.BlackScreenTransition = c.TransitionType
	}
	c.Trace = *b.tracePtr
	c.TraceColor = *b.traceColorPtr

	// Validation
	if err := c.Validate(); err != nil {
		return nil, err
	}

	return c, nil
}

func (b *Builder) generateDefaultOutputPath(c *Config) string {
	var nameSource string
	if strings.HasSuffix(strings.ToLower(c.InputPath), ".pdf") {
		nameSource = c.InputPath
	} else {
		if c.AudioPath != "" {
			nameSource = c.AudioPath
		} else {
			latestImg, err := system.FindLatestImage(c.InputPath)
			if err == nil {
				nameSource = latestImg
			} else {
				nameSource = c.InputPath
			}
		}
	}

	baseName := filepath.Base(nameSource)
	ext := filepath.Ext(baseName)
	nameOnly := strings.TrimSuffix(baseName, ext)
	cleanName := strings.ReplaceAll(nameOnly, " ", "_")
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	return filepath.Join("output", fmt.Sprintf("%s_%s.mp4", cleanName, timestamp))
}
