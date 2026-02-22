# Proposal: Advanced Camera Trajectories & Smart Planning

This document outlines improvements for the camera "flight" and "zoom" logic to achieve cinematic quality and more natural movement.

## 1. Path Smoothing (Spline Interpolation)
**Current:** Linear (X, Y) interpolation with cubic easing. This creates sharp changes in direction at keyframes.
**Proposal:** Implement **Catmull-Rom splines** for the (X, Y) path. 
- **Benefit:** Smooth, curved trajectories that feel like a real handheld or crane-operated camera.
- **Implementation:** Replace `lerp` for X/Y in `interpolator.go` with a spline calculation when moving between multiple points.

## 2. Adaptive Dwell & Travel Timing
**Current:** Equal time allocated to every block regardless of content or distance.
**Proposal:** 
- **Reading Time Estimation:** If OCR is enabled, calculate duration based on word count (e.g., 200 wpm). Otherwise, use block area as a proxy for content density.
- **Velocity-Weighted Travel:** Instead of fixed travel time, define a "comfortable camera speed". Travel time between ROIs should be proportional to the distance `sqrt(dx^2 + dy^2)`.

## 3. Shot Clustering & Hierarchy
**Current:** Visits every detected block individually.
**Proposal:** 
- **Grouping:** If multiple ROIs are within N% of the viewport at a certain zoom level, treat them as a single "group shot" to avoid unnecessary micro-movements.
- **Semantic Priority:** Rank blocks (Title > Image > Text). High-priority blocks get closer zooms and longer dwell times.

## 4. Cinematic Directing Styles (Presets)
Introduce presets in `Config` to change the "personality" of the camera:
- **"The Professor" (Narrative):** Top-to-bottom, focus on text, slow pans, long reading times.
- **"The Advertiser" (Dynamic):** Sharp zooms onto images/charts, faster travel, shorter dwell times.
- **"The Artist" (Organic):** Uses curved paths, slight "breathing" even when stationary, focuses on visual balance.

## 5. Composition Awareness
**Proposal:** 
- **Rule of Thirds:** Instead of perfectly centering every block, offset the camera slightly based on the block's content (e.g., focus on faces or text starts).
- **Dynamic Padding:** Adjust the "zoom-in" margin based on the slide's blank space to avoid "cramped" shots.

## 6. Inertia and Physics
- **Proposal:** Add subtle "overshoot" and "settling" effects at the end of a flight to simulate camera weight and inertia.
