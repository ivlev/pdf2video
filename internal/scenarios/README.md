# Scenarios Directory

This directory stores generated YAML scenarios from the Smart Zoom analyzer.

## Usage

Scenarios are automatically saved here when using `-generate-scenario` flag:

```bash
go run cmd/pdf2video/main.go -input doc.pdf -generate-scenario
# Creates: internal/scenarios/scenario.yaml
```

## Format

Each scenario file contains:
- Detected regions of interest (blocks)
- Keyframes with camera positions
- Zoom levels and timing

You can manually edit these files before rendering.
