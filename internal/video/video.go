package video

import (
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
)

type VideoEncoder interface {
	EncodeSegment(ctx context.Context, img image.Image, videoPath string, params config.SegmentParams, encoderName string, quality int) error
	Concatenate(ctx context.Context, segmentPaths []string, finalPath string, tmpDir string, params config.Config) error
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

	args := e.buildFFmpegArgs(inputW, inputH, videoPath, params, encoderName, quality)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe error: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ffmpeg start error: %w", err)
	}

	// Запись raw RGBA данных
	if err := e.writeRawRGBA(stdin, img); err != nil {
		stdin.Close()
		return fmt.Errorf("write raw error: %w", err)
	}
	stdin.Close()

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("ffmpeg wait error: %w", err)
	}

	return nil
}

func (e *FFmpegEncoder) buildFFmpegArgs(
	inputW, inputH int,
	videoPath string,
	params config.SegmentParams,
	encoderName string,
	quality int,
) []string {
	args := []string{
		"-y",
		"-f", "rawvideo",
		"-pixel_format", "rgba",
		"-video_size", fmt.Sprintf("%dx%d", inputW, inputH),
		"-i", "-",
		"-vf", params.Filter,
		"-t", fmt.Sprintf("%f", params.Duration),
		"-r", fmt.Sprintf("%d", params.FPS),
		"-pix_fmt", "yuv420p",
		"-c:v", encoderName,
	}

	// Качество в зависимости от энкодера
	switch encoderName {
	case "h264_videotoolbox":
		bitrate := quality * 100
		args = append(args, "-b:v", fmt.Sprintf("%dk", bitrate))
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
		rgba = image.NewRGBA(bounds)
		draw.Draw(rgba, bounds, img, bounds.Min, draw.Src)
	}
	_, err := w.Write(rgba.Pix)
	return err
}

func (e *FFmpegEncoder) Concatenate(ctx context.Context, segmentPaths []string, finalPath string, tmpDir string, params config.Config) error {
	// Используем сложный фильтр (filter_complex), если:
	// 1. Нужен переход (xfade)
	// 2. Есть фоновое аудио для микширования
	// 3. Есть основное аудио, которое нужно наложить на видеоряд
	useComplex := (params.TransitionType != "" && params.TransitionType != "none" && len(segmentPaths) > 1) ||
		params.BackgroundAudio != "" ||
		params.AudioPath != ""

	if !useComplex {
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

		cmd := exec.CommandContext(ctx, "ffmpeg", "-y",
			"-f", "concat", "-safe", "0", "-i", concatFilePath,
			"-c", "copy", finalPath,
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("ffmpeg concat error: %v, output: %s", err, string(out))
		}
		return nil
	}

	fadeDuration := params.FadeDuration

	args := []string{"-y"}
	for _, p := range segmentPaths {
		args = append(args, "-i", p)
	}

	audioIndex := -1
	if params.AudioPath != "" {
		audioIndex = len(segmentPaths)
		args = append(args, "-i", params.AudioPath)
	}

	filterGraph := ""
	lastOut := "[0:v]"
	currentOffset := 0.0

	// 1. Видео фильтры (xfade)
	if params.TransitionType != "" && params.TransitionType != "none" && len(segmentPaths) > 1 {
		for i := 1; i < len(segmentPaths); i++ {
			duration := params.TotalDuration / float64(len(segmentPaths))
			if i-1 < len(params.PageDurations) {
				duration = params.PageDurations[i-1]
			}
			currentOffset += duration - fadeDuration

			nextIn := fmt.Sprintf("[%d:v]", i)
			outName := fmt.Sprintf("[v%d]", i)
			filterGraph += fmt.Sprintf("%s%sxfade=transition=%s:duration=%f:offset=%f%s;",
				lastOut, nextIn, params.TransitionType, fadeDuration, currentOffset, outName)
			lastOut = outName
		}
	} else if len(segmentPaths) > 1 {
		// Если переходов нет, но сегментов много — используем concat filter
		concatInputs := ""
		for i := 0; i < len(segmentPaths); i++ {
			concatInputs += fmt.Sprintf("[%d:v]", i)
		}
		filterGraph += fmt.Sprintf("%sconcat=n=%d:v=1:a=0[vconcat];", concatInputs, len(segmentPaths))
		lastOut = "[vconcat]"
	}

	// 2. Аудио фильтры
	audioOut := ""
	if audioIndex != -1 {
		if params.BackgroundAudio != "" {
			bgIndex := audioIndex + 1
			args = append(args, "-stream_loop", "-1", "-i", params.BackgroundAudio)

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

			filterGraph += fmt.Sprintf("[%d:a]%s[bg_a];[%d:a]volume=1.0[main_a];[main_a][bg_a]amix=inputs=2:duration=first:dropout_transition=3[aout];",
				bgIndex, bgVolExpr, audioIndex)
			audioOut = "[aout]"
		} else {
			// Только основное аудио (прокидываем через фильтр для единообразия или просто мапим)
			audioOut = fmt.Sprintf("%d:a", audioIndex)
		}
	}

	filterGraph = strings.TrimSuffix(filterGraph, ";")
	if filterGraph != "" {
		args = append(args, "-filter_complex", filterGraph)
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
		qualityArgs = append(qualityArgs, "-b:v", fmt.Sprintf("%dk", bitrate))
	case "h264_nvenc":
		qualityArgs = append(qualityArgs, "-cq", fmt.Sprintf("%d", params.Quality))
	default: // libx264
		qualityArgs = append(qualityArgs, "-crf", fmt.Sprintf("%d", params.Quality), "-preset", "medium")
	}

	args = append(args, "-c:v", params.VideoEncoder, "-pix_fmt", "yuv420p")
	args = append(args, qualityArgs...)
	args = append(args, finalPath)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg xfade error: %v, output: %s", err, string(out))
	}
	return nil
}
