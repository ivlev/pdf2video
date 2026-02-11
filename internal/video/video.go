package video

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ivlev/pdf2video/internal/config"
)

type VideoEncoder interface {
	EncodeSegment(imagePath, videoPath string, params config.SegmentParams) error
	Concatenate(segmentPaths []string, finalPath string, tmpDir string, params config.Config) error
}

type FFmpegEncoder struct{}

func (e *FFmpegEncoder) EncodeSegment(imagePath, videoPath string, params config.SegmentParams) error {
	// EncodeSegment is currently handled within the engine's worker loop,
	// but can be moved here in the future for better encapsulation.
	return nil
}

func (e *FFmpegEncoder) Concatenate(segmentPaths []string, finalPath string, tmpDir string, params config.Config) error {
	if params.TransitionType == "" || params.TransitionType == "none" || len(segmentPaths) < 2 {
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
	filterGraph = strings.TrimSuffix(filterGraph, ";")

	args = append(args, "-filter_complex", filterGraph)
	args = append(args, "-map", lastOut)

	if audioIndex != -1 {
		args = append(args, "-map", fmt.Sprintf("%d:a", audioIndex), "-shortest")
	}

	args = append(args, "-c:v", "libx264", "-pix_fmt", "yuv420p", "-preset", "medium", finalPath)

	cmd := exec.Command("ffmpeg", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg xfade error: %v, output: %s", err, string(out))
	}
	return nil
}
