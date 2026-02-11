# pdf2video Functional Specification

**Version:** 0.5 (Pipelined)
**Date:** 2026-02-11

## 1. Product Overview
`pdf2video` is a high-performance CLI utility for automatically creating dynamic video presentations from static PDF files or image sets. The program transforms static slides into cinematic video sequences with effects overlay ("Zoom Drift", transitions), audio synchronization, and hardware acceleration.

## 2. Key Features

### 2.1. Input Data
- **PDF Documents:** High-resolution page rendering (configurable DPI).
- **Images:** Folders containing `.jpg`, `.jpeg`, `.png` files. Sorting and filtering supported.
- **Audio:** Automatic soundtrack selection from `input/audio` folder or specific file path.

### 2.2. Video Generation
- **Presets:** Ready-made formats for social media:
    - `16:9` (YouTube)
    - `9:16` (TikTok, Reels, Shorts)
    - `4:5` (Instagram Feed)
- **Zoom Drift:** Intelligent camera "breathing" effect (smooth zoom in and return).
- **Transitions:** Support for `xfade` (fade, wipe, slide, pixelize, etc.).
- **Synchronization:** Video duration automatically adjusts to audio track length (if enabled).
- **Randomization:** Slide duration varies by ±15% from average to create a lively rhythm.

## 3. Architecture & Optimizations

### 3.1. Hardware Acceleration
The application automatically selects the best available encoder:
- **macOS:** `h264_videotoolbox` (Apple Silicon / Intel). Uses bitrate control (`-b:v`).
- **NVIDIA:** `h264_nvenc` (NVENC).
- **CPU:** `libx264` (Fallback).

### 3.2. Pipelining (Stream Processing)
Architecture is split into two independent worker pools:
1.  **Render Pool (CPU):** Renders PDF pages to images. Scales across all cores.
2.  **Encode Pool (GPU):** Encodes video segments. Limited to 4 threads to protect GPU memory.
Data is transferred via buffered channels, ensuring 100% resource utilization without idle time.

### 3.3. Memory Pipes (Zero-Disk I/O)
Images are transferred from Render Pool to FFmpeg directly through RAM (stdin pipe) in `rawvideo` format. Intermediate PNG files are not created, significantly reducing SSD wear and speeding up operation.

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

### Quality & Performance
| Flag | Description | Default |
|------|-------------|---------|
| `-dpi` | PDF rendering quality | 300 |
| `-quality` | Quality (CRF for x264, Bitrate for GPU) | `auto` (x264: 23, VT: 75) |
| `-stats` | Output performance metrics | `false` |
| `-workers` | Number of render threads | All CPU cores |

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
