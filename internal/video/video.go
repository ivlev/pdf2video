package video

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"image"
	"image/draw"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ivlev/pdf2video/internal/config"
	"github.com/ivlev/pdf2video/internal/system"
)

type ProgressFunc func(current, total float64)

type VideoEncoder interface {
	EncodeSegment(ctx context.Context, img image.Image, videoPath string, params config.SegmentParams, encoderName string, quality int) error
	Concatenate(ctx context.Context, segments []config.VideoSegment, finalPath string, tmpDir string, params config.Config, audioDelayMs int, progress ProgressFunc) error
}

type FFmpegEncoder struct{}

func (e *FFmpegEncoder) EncodeSegment(
	ctx context.Context,
	img image.Image,
	videoPath string,
	params config.SegmentParams,
	encoderName string,
	quality int,
) error {
	inputW, inputH := img.Bounds().Dx(), img.Bounds().Dy()

	// Создаем временный файл для фильтра, чтобы избежать лимитов на размер аргументов командной строки
	filterFile, err := os.CreateTemp("", "ffmpeg_filter_*.txt")
	if err != nil {
		return fmt.Errorf("failed to create temp filter file: %w", err)
	}
	defer os.Remove(filterFile.Name())

	if _, err := filterFile.WriteString(params.Filter); err != nil {
		filterFile.Close()
		return fmt.Errorf("failed to write filter: %w", err)
	}
	filterFile.Close()

	args := e.buildFFmpegArgs(inputW, inputH, videoPath, params, encoderName, quality, filterFile.Name())

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe error: %w, stderr: %s", err, stderr.String())
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ffmpeg start error: %w, stderr: %s", err, stderr.String())
	}

	// Запись raw RGBA данных
	if err := e.writeRawRGBA(stdin, img); err != nil {
		stdin.Close()
		_ = cmd.Wait() // Clean up process
		return fmt.Errorf("write raw error: %w, stderr: %s", err, stderr.String())
	}
	stdin.Close()

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("ffmpeg wait error: %w, stderr: %s", err, stderr.String())
	}

	return nil
}

func (e *FFmpegEncoder) buildFFmpegArgs(
	inputW, inputH int,
	videoPath string,
	params config.SegmentParams,
	encoderName string,
	quality int,
	filterPath string,
) []string {
	args := []string{
		"-y",
		"-f", "rawvideo",
		"-pixel_format", "rgba",
		"-video_size", fmt.Sprintf("%dx%d", inputW, inputH),
		"-i", "-",
		"-filter_script:v", filterPath,
		"-t", fmt.Sprintf("%f", params.Duration),
		"-r", fmt.Sprintf("%d", params.FPS),
		"-pix_fmt", "yuv420p",
		"-c:v", encoderName,
	}

	// Качество в зависимости от энкодера
	switch encoderName {
	case "h264_videotoolbox":
		bitrate := quality * 100
		args = append(args, "-b:v", fmt.Sprintf("%dk", bitrate), "-pix_fmt", "yuv420p", "-realtime", "true")
	case "h264_nvenc":
		args = append(args, "-cq", fmt.Sprintf("%d", quality))
	default: // libx264
		args = append(args, "-crf", fmt.Sprintf("%d", quality), "-preset", "medium")
	}

	args = append(args, videoPath)
	return args
}

func (e *FFmpegEncoder) writeRawRGBA(w io.Writer, img image.Image) error {
	bounds := img.Bounds()
	rgba, ok := img.(*image.RGBA)
	if !ok || rgba.Stride != bounds.Dx()*4 || rgba.Rect.Min.X != 0 || rgba.Rect.Min.Y != 0 {
		rgba = system.GetImage(bounds)
		defer system.PutImage(rgba)
		draw.Draw(rgba, bounds, img, bounds.Min, draw.Src)
	}
	_, err := w.Write(rgba.Pix)
	return err
}

func (e *FFmpegEncoder) Concatenate(ctx context.Context, segments []config.VideoSegment, finalPath string, tmpDir string, params config.Config, audioDelayMs int, progress ProgressFunc) error {
	// Используем сложный фильтр (filter_complex), если:
	// 1. Нужен переход (xfade) хотя бы в одном сегменте
	// 2. Есть фоновое аудио для микширования
	// 3. Есть основное аудио (особенно с задержкой)
	hasTransition := false
	for i := 1; i < len(segments); i++ {
		if segments[i].TransitionType != "" && segments[i].TransitionType != "none" {
			hasTransition = true
			break
		}
	}

	useComplex := hasTransition || params.BackgroundAudio != "" || params.AudioPath != ""

	if !useComplex {
		concatFilePath := filepath.Join(tmpDir, "inputs.txt")
		f, err := os.Create(concatFilePath)
		if err != nil {
			return err
		}
		for _, s := range segments {
			absPath, _ := filepath.Abs(s.Path)
			fmt.Fprintf(f, "file '%s'\n", absPath)
		}
		f.Close()

		cmd := exec.CommandContext(ctx, "ffmpeg", "-y",
			"-f", "concat", "-safe", "0", "-i", concatFilePath,
			"-c", "copy", finalPath,
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("ffmpeg concat error: %v, output: %s", err, string(out))
		}
		if progress != nil {
			progress(params.TotalDuration, params.TotalDuration)
		}
		return nil
	}

	args := []string{"-y"}
	for _, s := range segments {
		args = append(args, "-i", s.Path)
	}

	nextInputIdx := len(segments)

	audioIndex := -1
	if params.AudioPath != "" {
		audioIndex = nextInputIdx
		args = append(args, "-i", params.AudioPath)
		nextInputIdx++
	}

	qrIndex := -1
	if params.QRCodePath != "" {
		qrIndex = nextInputIdx
		args = append(args, "-i", params.QRCodePath)
		nextInputIdx++
	}

	filterGraph := ""
	lastOut := "[0:v]"
	currentOffset := 0.0

	// 1. Видео фильтры (xfade)
	if hasTransition {
		for i := 1; i < len(segments); i++ {
			fadeDur := segments[i].FadeDuration
			transType := segments[i].TransitionType
			if transType == "" || transType == "none" || fadeDur <= 0 {
				transType = "fade"
				fadeDur = 0.05 // Минимальный фейд для корректной работы xfade
			}

			// offset: время предыдущего сегмента минус текущий фейд
			currentOffset += segments[i-1].Duration - fadeDur

			nextIn := fmt.Sprintf("[%d:v]", i)
			outName := fmt.Sprintf("[v%d]", i)
			filterGraph += fmt.Sprintf("%s%sxfade=transition=%s:duration=%f:offset=%f%s;",
				lastOut, nextIn, transType, fadeDur, currentOffset, outName)
			lastOut = outName
		}
	} else if len(segments) > 1 {
		// Если переходов нет, но сегментов много — используем concat filter
		concatInputs := ""
		for i := 0; i < len(segments); i++ {
			concatInputs += fmt.Sprintf("[%d:v]", i)
		}
		filterGraph += fmt.Sprintf("%sconcat=n=%d:v=1:a=0[vconcat];", concatInputs, len(segments))
		lastOut = "[vconcat]"
	}

	// 1.5 QR Code Overlay (Persistent from audio start)
	if qrIndex != -1 {
		startTime := float64(audioDelayMs) / 1000.0
		outName := "[vqr]"
		filterGraph += fmt.Sprintf("%s[%d:v]overlay=x=main_w-overlay_w-%d:y=main_h-overlay_h-%d:enable='between(t,%f,99999)'%s;",
			lastOut, qrIndex, params.QRMarginRight, params.QRMarginBottom, startTime, outName)
		lastOut = outName
	}

	// 2. Аудио фильтры
	audioOut := ""
	if audioIndex != -1 {
		mainAudioFilter := ""
		if audioDelayMs > 0 {
			// Pad with silence to match total video duration to avoid infinite loops
			filterGraph += fmt.Sprintf("[%d:a]adelay=%d|%d,apad=whole_len=%f[padded_a];", audioIndex, audioDelayMs, audioDelayMs, params.TotalDuration)
			mainAudioFilter = "[padded_a]"
			audioOut = "[padded_a]"
		} else {
			// Pad with silence to match total video duration
			filterGraph += fmt.Sprintf("[%d:a]apad=whole_len=%f[padded_a];", audioIndex, params.TotalDuration)
			mainAudioFilter = "[padded_a]"
			audioOut = "[padded_a]"
		}

		if params.BackgroundAudio != "" {
			bgIndex := nextInputIdx
			args = append(args, "-stream_loop", "-1", "-i", params.BackgroundAudio)
			nextInputIdx++ // redundant but good for consistency

			bgVol := params.BackgroundVolume
			fadeInDur := 5.0
			fadeOutDur := 5.0
			totalDur := params.TotalDuration
			if totalDur < fadeInDur+fadeOutDur {
				fadeInDur = totalDur * 0.1
				fadeOutDur = totalDur * 0.1
			}

			bgVolExpr := fmt.Sprintf("volume='%f*(if(lte(t,%f), 0.1 + 0.9*(t/%f), if(gte(t, %f), (%f-t)/%f, 1.0)))':eval=frame",
				bgVol, fadeInDur, fadeInDur, totalDur-fadeOutDur, totalDur, fadeOutDur)

			filterGraph += fmt.Sprintf("[%d:a]%s[bg_a];%svolume=1.0[main_a];[main_a][bg_a]amix=inputs=2:duration=first:dropout_transition=3[aout];",
				bgIndex, bgVolExpr, mainAudioFilter)
			audioOut = "[aout]"
		}
	}

	filterGraph = strings.TrimSuffix(filterGraph, ";")
	if filterGraph != "" {
		// Создаем временный файл для сложного фильтра
		filterFile, err := os.CreateTemp("", "ffmpeg_complex_*.txt")
		if err != nil {
			return fmt.Errorf("failed to create temp complex filter file: %w", err)
		}
		defer os.Remove(filterFile.Name())

		if _, err := filterFile.WriteString(filterGraph); err != nil {
			filterFile.Close()
			return fmt.Errorf("failed to write complex filter: %w", err)
		}
		filterFile.Close()

		args = append(args, "-filter_complex_script", filterFile.Name())
	}

	// Настройка маппинга
	args = append(args, "-map", lastOut)
	if audioOut != "" {
		args = append(args, "-map", audioOut)
		args = append(args, "-shortest")
	}

	qualityArgs := []string{}
	switch params.VideoEncoder {
	case "h264_videotoolbox":
		// VideoToolbox часто не поддерживает -q:v напрямую на всех версиях. Используем битрейт.
		bitrate := params.Quality * 100 // кбит/с. 75 -> 7.5Мбит/с
		qualityArgs = append(qualityArgs, "-b:v", fmt.Sprintf("%dk", bitrate), "-realtime", "true")
	case "h264_nvenc":
		qualityArgs = append(qualityArgs, "-cq", fmt.Sprintf("%d", params.Quality))
	default: // libx264
		qualityArgs = append(qualityArgs, "-crf", fmt.Sprintf("%d", params.Quality), "-preset", "medium")
	}

	args = append(args, "-c:v", params.VideoEncoder, "-pix_fmt", "yuv420p")
	args = append(args, qualityArgs...)
	args = append(args, "-progress", "pipe:1", "-movflags", "+faststart")
	args = append(args, finalPath)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ffmpeg concat error: %w, stderr: %s", err, stderr.String())
	}

	if progress != nil {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "out_time_us=") {
				var us int64
				fmt.Sscanf(line, "out_time_us=%d", &us)
				seconds := float64(us) / 1000000.0
				progress(seconds, params.TotalDuration)
			} else if line == "progress=end" {
				progress(params.TotalDuration, params.TotalDuration)
			}
		}
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("ffmpeg concat error: %w, stderr: %s", err, stderr.String())
	}
	return nil
}
