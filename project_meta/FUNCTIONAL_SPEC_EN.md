# pdf2video Functional Specification

**Version:** 0.8 (Outro Duration Control)
**Date:** 2026-02-14

## 1. Product Overview
`pdf2video` is a high-performance CLI utility for automatically creating dynamic video presentations from static PDF files or image sets. The program transforms static slides into cinematic video sequences with precise camera control via YAML scenarios, audio synchronization, and hardware acceleration.

## 2. Key Features

### 2.1. Input Data
- **PDF Documents:** High-resolution page rendering (configurable DPI).
- **Images:** Folders containing `.jpg`, `.jpeg`, `.png` files. Sorting and filtering supported.
- **Audio:** Automatic soundtrack selection from `input/audio` folder or specific file path.
- **Scenarios (YAML):** Keyframe descriptions for precise zoom and pan control.

### 2.2. Video Generation
- **Presets:** YouTube (16:9), TikTok/Shorts (9:16), Instagram (4:5).
- **Scenario Rendering:** High-precision camera movement based on provided coordinates and timing.
- **Smart Zoom (Auto-Plan):** Automatic scenario generation based on slide content analysis.
- **Transitions:** Support for all `xfade` effects (fade, wipe, slide, pixelize, etc.).
- **Synchronization:** Video length perfectly matches the audio track duration.
- **Outro Zoom-out:** Guaranteed camera return to 1:1 scale before each clip transition.
- **Frame-Boundary Alignment:** All timing calculations are aligned to exact frame boundaries (FPS) to eliminate jitter and concatenation artifacts.

## 3. Architecture & Optimizations

### 3.1. Hardware Acceleration
The application automatically selects the best available encoder:
- **macOS:** `h264_videotoolbox` (Apple Silicon / Intel).
- **NVIDIA:** `h264_nvenc`.
- **CPU:** `libx264` (Fallback).

### 3.2. Pipelining & Memory Pipes
- **Render Pool (CPU):** Renders PDF pages to images in parallel.
- **Encode Pool (GPU):** Encodes video segments using hardware acceleration.
- **Zero-Disk I/O:** Images are transferred directly to FFmpeg via `stdin pipe` in `rawvideo` format.

### 3.3. Smart Zoom & Scenario Rendering
- **Analyze:** ROI detection (headers, text) via `ContrastDetector`.
- **Plan:** Automatic YAML scenario generation.
- **Scale:** Scenario timings are automatically scaled to match the total audio duration.
- **Render:** `ScenarioEffect` transforms YAML keyframes into complex piecewise `zoompan` expressions.

## 4. Configuration Reference (Flags)

### Basics
| Flag | Description | Default |
|------|-------------|---------|
| `-input` | Path to PDF or image folder | Fresh file in `input/pdf/` |
| `-output` | Path to output file | Auto-generated in `output/` |
| `-preset` | Format preset (`16:9`, `9:16`, `4:5`) | - |

### Video & Audio
| Flag | Description | Default |
|------|-------------|---------|
| `-width`, `-height` | Explicit resolution | 1280x720 |
| `-fps` | Frame rate | 30 |
| `-duration` | Total video duration (sec) | 0 (auto by audio) |
| `-page-duration` | Average slide duration (sec) | 0.3 |
| `-audio` | Path to audio file | Fresh file in `input/audio/` |
| `-audio-sync` | Sync video to audio length | `true` |

### Effects
| Flag | Description | Default |
|------|-------------|---------|
| `-zoom-mode` | Zoom type (`center`, `random`, etc.) | `center` |
| `-zoom-speed` | Zoom speed | `0.001` |
| `-transition` | Transition type (`fade`, `wipeleft`...) | `fade` |
| `-fade` | Transition duration (sec) | `0.5` |
| `-outro-duration` | Time to return to 1:1 before transition | `1.0` |

### Quality & Performance
| Flag | Description | Default |
|------|-------------|---------|
| `-dpi` | PDF rendering quality | 300 |
| `-quality` | Quality (CRF for x264, Bitrate for GPU) | `auto` (x264: 23, VT: 75) |
| `-stats` | Output performance metrics | `false` |
| `-workers` | Number of render threads | All CPU cores |

### Smart Zoom (Analysis & Scenarios)
| Flag | Description | Default |
|------|-------------|---------|
| `-analyze-mode` | Analysis mode (`contrast`, `ocr`, `ai`) | `contrast` |
| `-min-block-area` | Minimum block area (pixels²) | 500 |
| `-edge-threshold` | Edge detection sensitivity threshold | 30.0 |
| `-generate-scenario` | Generate YAML scenario | `false` |
| `-scenario-output` | Path to save scenario | `internal/scenarios/scenario_YYYY-MM-DD_HH-MM-SS.yaml` |
| `-scenario` | Path to scenario for rendering | Latest from `internal/scenarios/` |

## 5. Usage Scenarios

### Quick Test
```bash
./pdf2video -stats
```
Takes the first available PDF and audio, creates a video, outputs statistics.

### Creating Shorts for TikTok
```bash
./pdf2video -input presentation.pdf -preset 9:16 -duration 60 -zoom-mode random
```
Creates a vertical video exactly 60 seconds long with random camera movements.

### Maximum Quality (4K)
```bash
./pdf2video -input high_res.pdf -width 3840 -height 2160 -quality 90 -dpi 600
```
Uses high bitrate and DPI for crystal clear image.

### Smart Zoom Scenario Generation
```bash
./pdf2video -input presentation.pdf -generate-scenario
```
Analyzes slides, detects key blocks, and creates a YAML scenario in `internal/scenarios/scenario_2026-02-13_01-42-26.yaml`.
