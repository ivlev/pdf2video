package system

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func InitResourceLimits() {
	var rLimit syscall.Rlimit
	err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit)
	if err != nil {
		log.Printf("[!] Не удалось получить лимит файлов: %v", err)
		return
	}

	rLimit.Cur = 2048
	if rLimit.Cur > rLimit.Max {
		rLimit.Cur = rLimit.Max
	}

	err = syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit)
	if err != nil {
		log.Printf("[!] Не удалось установить лимит файлов: %v", err)
	} else {
		fmt.Printf("[*] Системный лимит открытых файлов увеличен до %d\n", rLimit.Cur)
	}
}

func FindLatestPDF(dir string) (string, error) {
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

func FindLatestAudio(dir string) (string, error) {
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

func GetAudioDuration(path string) (float64, error) {
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

func FindLatestImage(path string) (string, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", err
	}

	var searchDir string
	if fi.IsDir() {
		searchDir = path
	} else {
		// Если это файл, берем его директорию, чтобы найти самый свежий в ней (согласно логике ТЗ)
		// Хотя если указан конкретный файл, возможно стоит вернуть его.
		// Но ТЗ говорит "имя файла изображения с самой поздней датой", если используются изображения.
		searchDir = filepath.Dir(path)
	}

	files, err := os.ReadDir(searchDir)
	if err != nil {
		return "", err
	}

	extensions := []string{".jpg", ".jpeg", ".png"}
	var latestFile string
	var latestTime time.Time

	for _, f := range files {
		if f.IsDir() {
			continue
		}
		isImage := false
		for _, ext := range extensions {
			if strings.HasSuffix(strings.ToLower(f.Name()), ext) {
				isImage = true
				break
			}
		}
		if isImage {
			info, err := f.Info()
			if err != nil {
				continue
			}
			if info.ModTime().After(latestTime) {
				latestTime = info.ModTime()
				latestFile = filepath.Join(searchDir, f.Name())
			}
		}
	}

	if latestFile == "" {
		return "", fmt.Errorf("в папке %s не найдено изображений", searchDir)
	}

	return latestFile, nil
}
func GetBestH264Encoder() (string, string) {
	// Приоритеты:
	// 1. MacOS (VideoToolbox)
	// 2. NVIDIA (NVENC)
	// 3. Intel/Linux (VAAPI - требует доп. настройки, пока пропустим или добавим позже)
	// 4. Software (libx264)

	encoders := []struct {
		name string
		args string
	}{
		{"h264_videotoolbox", ""},
		{"h264_nvenc", ""},
	}

	for _, enc := range encoders {
		cmd := exec.Command("ffmpeg", "-encoders")
		out, err := cmd.CombinedOutput()
		if err == nil && strings.Contains(string(out), enc.name) {
			return enc.name, enc.args
		}
	}

	return "libx264", ""
}
