# pdf2video Functional Specification

**Version:** 0.9.2 (Progress Indicator & Stats Fix)
**Date:** 2026-03-31

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
- **Smart Zoom (Auto-Plan):** Automatic scenario generation based on slide content analysis. "Auto" mode selects between OCR (for text-rich PDFs) and Contrast analysis.
- **Transitions:** Support for all `xfade` effects (fade, wipe, slide, pixelize, etc.).
- **Synchronization:** Video length perfectly matches the audio track duration.
- **Outro Zoom-out:** Guaranteed camera return to 1:1 scale before transitions.
- **Safe Audio Mixing:** 100% stability when using background music of any duration. The algorithm limits `apad` to the video duration, preventing infinite rendering loops.
- **Native Mac Support (Compatibility):** Videos are optimized for QuickTime Player and macOS system frameworks using the `yuv420p` standard and the `faststart` flag (moving `moov` metadata to the beginning of the file).
- **Graceful Shutdown:** Clean FFmpeg termination and temporary file cleanup on Ctrl+C.
- **Modular Architecture:** Encoder logic isolated in `video` package for easier maintenance.
- **Frame-Boundary Alignment:** All timing calculations are aligned to exact frame boundaries (FPS) to eliminate jitter.

## 3. Architecture & Optimizations

### 3.1. Hardware Acceleration
The application automatically selects the best available encoder:
- **macOS:** `h264_videotoolbox` (Apple Silicon / Intel). Forces `yuv420p` standard for maximum compatibility.
- **NVIDIA:** `h264_nvenc`.
- **CPU:** `libx264` (Fallback).

### 3.2. Pipelining & Memory Pipes
- **Render Pool (CPU):** Renders PDF pages to images in parallel.
- **Encode Pool (GPU):** Encodes video segments using hardware acceleration.
- **Zero-Disk I/O:** Images are transferred directly to FFmpeg via `stdin pipe` in `rawvideo` format.
- **PDF Document Pooling:** Uses `sync.Pool` to reuse open PDF documents, eliminating redundant I/O during parallel rendering.
- **Buffer Pooling:** Centralized pool for `image.RGBA` buffer reuse. Reduces Garbage Collector pressure and prevents memory fragmentation.
- **Render Caching:** Persistent on-disk caching of rendered pages. Skips the rendering phase for unchanged PDF files.
- **Memory Budgeting:** Automatic management of worker count and RAM consumption to prevent Out-Of-Memory errors.
- **Adaptive DPI:** Automatic calculation of the minimum required pixel density (DPI) for the target video resolution with a 50% margin for zoom. Reduces CPU load by 20-40%.

### 3.3. Smart Zoom & Scenario Rendering
- **Analyze:** ROI detection via `EnhancedDetector` (edge density) or `OCRDetector` (structural text from MuPDF with dynamic DPI scaling).
- **Score:** Semantic ranking of blocks via `SemanticScorer` based on content type (headers, charts) and vertical position.
- **Plan:** Generation of an optimized camera trajectory via `TrajectoryOptimizer`, balancing block importance and travel distance to avoid erratic jumps.
- **Scale:** Scenario timings are automatically scaled to match the total audio duration.
- **Render:** `ScenarioEffect` transforms YAML keyframes into complex piecewise `zoompan` expressions.

## 4. Technical Excellence & Reliability

### 4.1. Scenario Scalability
- **$O(N)$ Expression Optimization:** Complex pan filters are mathematically optimized, allowing scenarios with hundreds of keyframes without performance degradation or FFmpeg buffer overflows.
- **Filter Scripts:** Uses `-filter_script` to bypass OS command-line length limits, ensuring stability for long, complex projects.

### 4.2. Visual Debugging
- **Debug Overlay:** Integrated visualizer to see region-of-interest bounding boxes directly in the generated video for easier scenario fine-tuning.
- **Go-side Drawing:** Critical debug elements (camera tracking) are rendered in the Go engine, guaranteed to show even if FFmpeg filters are restricted.

### 4.3. Compatibility & Standards
- **Faststart Optimization:** Using `-movflags +faststart` allows videos to start playback immediately after opening (or starting to download in a browser) without waiting for the entire file to load.
- **Color Space Standard:** Forcing `yuv420p` in all pipelines (including hardware) ensures correct color representation in QuickTime and on mobile devices.

## 5. Configuration Reference (Flags)

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
| `-fade` | Transition effect duration (sec) | `0.5` |
| `-outro-duration` | Time to return to 1:1 before transition | `1.0` |
| `-black-screen-duration` | Black screen duration (intro/outro) | `2.0` |
| `-black-screen-transition`| Transition type for black screen | same as `-transition` |
| `-bg-audio` | Path to background track | auto |
| `-bg-volume` | Background track volume (0.0 - 1.0) | `0.3` |

### Quality & Performance
| Flag | Description | Default |
|------|-------------|---------|
| `-dpi` | PDF rendering quality | 300 |
| `-quality` | Quality (CRF for x264, Bitrate for GPU) | `auto` (x264: 23, VT: 75) |
| `-stats` | Output performance metrics | `false` |
| `-debug` | Debug mode: draw camera paths and stats | `false` |
| `-trace` | Trace mode: draw camera movement direction and stop points | `false` |
| `-trace-color` | Color of coordinate text in trace mode (HEX) | `#FFFFFF` |
| `-workers` | Number of render threads | All CPU cores |

### Smart Zoom (Analysis & Scenarios)
| Flag | Description | Default |
|------|-------------|---------|
| `-analyze-mode` | Analysis mode (`auto`, `contrast`, `ocr`, `enhanced`) | `auto` |
| `-min-block-area` | Minimum block area (pixels²) | 500 |
| `-edge-threshold` | Edge detection sensitivity threshold | 30.0 |
| `-generate-scenario` | Generate YAML scenario | `false` |
| `-scenario-output` | Path to save scenario | `internal/scenarios/scenario_YYYY-MM-DD_HH-MM-SS.yaml` |
| `-scenario` | Path to scenario for rendering | Latest from `internal/scenarios/` |

## 6. Usage Scenarios

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

## 16. Quality Assurance & Reliability
**PLANNED.**
- **Unit Testing:** 80%+ coverage for `internal/system`, `internal/video`, and `internal/engine`.
- **Mocking Strategy:** Isolation of external dependencies (FFmpeg, Fitz) for deterministic testing.
- **CI/CD:** Automatic quality control via GitHub Actions.

## 17. Smart Directing 2.0 (Advanced Camera Logic)
**ACTIVE PHASE:**
- **Content-Aware Analysis:** Evaluation of block importance via Edge Density and Color Variance. Helps differentiate between text and graphics.
- **Semantic Scoring:** Content type prioritization. Headers and charts receive higher weight (up to 1.0) than body text or footers.
- **Trajectory Optimizer:** Intelligent ROI sorting using a greedy algorithm that combines block importance (Priority) and physical proximity (Distance Weight) to minimize redundant camera travel and prevent erratic jumps.

**PLANNED:**
- **Adaptive Dwell Time (PLANNED):** Dynamic adjustment of travel speeds and dwell times based on block significance.
- **Cinematic Smoothing (PLANNED):** Smooth camera trajectories (Splines) and movement inertia physics.
