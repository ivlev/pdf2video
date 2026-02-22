# Epic: Performance Optimization & Resource Efficiency

## Description
This issue tracks the implementation of high-priority performance optimizations for `pdf2video`. The goal is to reduce rendering time, optimize memory usage, and improve overall system stability during long video generation sessions.

## Optimization Patterns

### 1. [P0] Adaptive DPI Rendering
- **Goal**: Dynamically calculate the DPI required to match the target video resolution.
- **Benefit**: 20-40% speed increase by avoiding over-rendering PDF pages (e.g., rendering 150 DPI instead of 300 DPI for 1080p output).
- **Task**: Implement `calculateOptimalDPI` in `internal/engine/engine.go`.

### 2. [P1] Buffer Pooling (sync.Pool)
- **Goal**: Reuse `image.RGBA` buffers during the rendering and encoding process.
- **Benefit**: Significantly reduces Garbage Collector (GC) pressure and prevents memory fragmentation.
- **Task**: Implement a centralized `BufferPool` in `internal/system/`.

### 3. [P2] Persistent Render Caching
- **Goal**: Store rendered pages as PNGs in a temporary/cache directory.
- **Benefit**: Instantaneous re-runs for unchanged PDF content.
- **Task**: Implement file-based caching with hash-based keys (hash of PDF content + page + DPI).

### 4. [P3] Memory Budgeting & Worker Scaling
- **Goal**: Limit simultaneous operations based on available system RAM.
- **Benefit**: Prevents Out-Of-Memory (OOM) crashes on resource-constrained environments.
- **Task**: Implement `MemoryManager` to track allocated bytes and throttle workers.

## Definition of Done
- [ ] Benchmarks show measurable improvement in FPS.
- [ ] Memory profile remains stable during 100+ page processing.
- [ ] Unit tests for new optimization components.

---
*Based on expert project analysis v0.8. Refer to [pdf2video_roadmap_and_optimization.md](https://github.com/ivlev/pdf2video/blob/main/project_meta/internal/pdf2video_roadmap_and_optimization.md) for full details.*
