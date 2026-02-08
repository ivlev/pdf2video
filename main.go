package main

import (
	"flag"
	"fmt"
	"image/png"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/gen2brain/go-fitz"
)

// Config содержит параметры генерации видео
type Config struct {
	InputPDF      string
	OutputVideo   string
	TotalDuration float64
	Width         int
	Height        int
	FPS           int
}

func main() {
	// 1. Определение флагов запуска
	inputPtr := flag.String("input", "input.pdf", "Путь к входному PDF файлу")
	outputPtr := flag.String("output", "animated_presentation.mp4", "Путь к готовому видео")
	durationPtr := flag.Float64("duration", 10.0, "Общая длительность видео в секундах")
	widthPtr := flag.Int("width", 1280, "Ширина видео")
	heightPtr := flag.Int("height", 720, "Высота видео")
	fpsPtr := flag.Int("fps", 30, "Кадров в секунду (для плавности анимации)")
	
	flag.Parse()

	conf := Config{
		InputPDF:      *inputPtr,
		OutputVideo:   *outputPtr,
		TotalDuration: *durationPtr,
		Width:         *widthPtr,
		Height:        *heightPtr,
		FPS:           *fpsPtr,
	}

	// Подготовка путей
	absInputPath, err := filepath.Abs(conf.InputPDF)
	if err != nil {
		log.Fatalf("[-] Ошибка пути: %v", err)
	}

	// 2. Инициализация PDF
	doc, err := fitz.New(absInputPath)
	if err != nil {
		log.Fatalf("[-] Ошибка открытия PDF: %v", err)
	}
	defer doc.Close()

	pageCount := doc.NumPage()
	pageDuration := conf.TotalDuration / float64(pageCount)

	fmt.Println("--- [SPRINT 3: ANIMATION MODE] ---")
	fmt.Printf("[*] Файл: %s\n", filepath.Base(absInputPath))
	fmt.Printf("[*] Страниц: %d | Длительность каждой: %.2fs\n", pageCount, pageDuration)
	fmt.Printf("[*] Разрешение: %dx%d @ %d FPS\n", conf.Width, conf.Height, conf.FPS)
	fmt.Println("----------------------------------")

	// 3. Создание временной папки
	tmpDir, err := os.MkdirTemp("", "pdf_anim_")
	if err != nil {
		log.Fatalf("[-] Ошибка создания temp-папки: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	concatFilePath := filepath.Join(tmpDir, "inputs.txt")
	concatFile, _ := os.Create(concatFilePath)

	// 4. Цикл обработки страниц с анимацией
	for i := 0; i < pageCount; i++ {
		fmt.Printf("[>] Рендеринг и анимация страницы %d/%d...\r", i+1, pageCount)

		// Рендерим страницу в PNG (высокое качество для зума)
		img, err := doc.Image(i)
		if err != nil {
			log.Printf("\n[!] Ошибка страницы %d: %v", i, err)
			continue
		}

		imgPath := filepath.Join(tmpDir, fmt.Sprintf("page_%d.png", i))
		imgFile, _ := os.Create(imgPath)
		png.Encode(imgFile, img)
		imgFile.Close()

		segmentPath := filepath.Join(tmpDir, fmt.Sprintf("seg_%d.mp4", i))
		
		// РАСЧЕТ АНИМАЦИИ (Ken Burns Effect)
		// d - общее кол-во кадров в сегменте
		totalFrames := int(pageDuration * float64(conf.FPS))
		
		// zoompan filter:
		// z - коэффициент увеличения. Начинаем с 1.0 и прибавляем 0.0008 каждый кадр (до 1.5x)
		// x, y - формулы центрирования, чтобы зум шел в середину листа
		// s - итоговый размер кадра
		zoomFilter := fmt.Sprintf(
			"zoompan=z='min(zoom+0.0008,1.5)':d=%d:s=%dx%d:x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)'",
			totalFrames, conf.Width, conf.Height,
		)

		// Запуск FFmpeg для создания анимированного сегмента
		cmd := exec.Command("ffmpeg", "-y",
			"-i", imgPath,
			"-vf", zoomFilter,
			"-t", fmt.Sprintf("%f", pageDuration),
			"-r", fmt.Sprintf("%d", conf.FPS),
			"-pix_fmt", "yuv420p",
			"-c:v", "libx264",
			"-preset", "medium", // Баланс между скоростью и качеством зума
			segmentPath,
		)

		if out, err := cmd.CombinedOutput(); err != nil {
			log.Fatalf("\n[-] Ошибка FFmpeg на странице %d: %v\nЛог: %s", i, err, string(out))
		}

		absSegPath, _ := filepath.Abs(segmentPath)
		fmt.Fprintf(concatFile, "file '%s'\n", absSegPath)
	}
	concatFile.Close()
	fmt.Println("\n[*] Все анимированные сегменты созданы.")

	// 5. Финальная склейка (мгновенно)
	fmt.Println("[*] Сборка финального фильма...")
	finalCmd := exec.Command("ffmpeg", "-y",
		"-f", "concat", "-safe", "0", "-i", concatFilePath,
		"-c", "copy", 
		conf.OutputVideo,
	)

	if out, err := finalCmd.CombinedOutput(); err != nil {
		log.Fatalf("[-] Ошибка склейки: %v\nЛог: %s", err, string(out))
	}

	fmt.Printf("[+++] Успех! Видео с анимацией: %s\n", conf.OutputVideo)
}