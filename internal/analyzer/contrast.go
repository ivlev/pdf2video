package analyzer

import (
	"image"
	"image/color"
	"math"
)

// ContrastDetector implements edge-based region detection using Sobel operator
type ContrastDetector struct {
	MinBlockArea  int     // Minimum area in pixels²
	EdgeThreshold float64 // Gradient magnitude threshold
}

// NewContrastDetector creates a new contrast-based detector with default settings
func NewContrastDetector() *ContrastDetector {
	return &ContrastDetector{
		MinBlockArea:  500,  // ~22x22 pixels minimum
		EdgeThreshold: 30.0, // Moderate sensitivity
	}
}

// Detect finds regions of interest using edge detection and morphology
func (d *ContrastDetector) Detect(img image.Image) ([]Block, error) {
	// Step 1: Convert to grayscale
	gray := toGrayscale(img)

	// Step 2: Apply Sobel edge detection
	edges := sobelEdgeDetection(gray, d.EdgeThreshold)

	// Step 3: Morphological dilation to connect nearby edges
	dilated := dilate(edges, 5, 2)

	// Step 4: Find connected components (contours)
	contours := findContours(dilated)

	// Step 5: Filter by minimum area and create Blocks
	blocks := []Block{}
	for _, rect := range contours {
		area := rect.Dx() * rect.Dy()
		if area >= d.MinBlockArea {
			blocks = append(blocks, Block{
				Rect:       rect,
				Type:       "unknown", // Could be refined with aspect ratio analysis
				Confidence: 0.7,       // Moderate confidence for edge-based detection
			})
		}
	}

	return blocks, nil
}

// toGrayscale converts an image to grayscale
func toGrayscale(img image.Image) *image.Gray {
	bounds := img.Bounds()
	gray := image.NewGray(bounds)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			gray.Set(x, y, color.GrayModel.Convert(img.At(x, y)))
		}
	}

	return gray
}

// sobelEdgeDetection applies Sobel operator to detect edges
func sobelEdgeDetection(gray *image.Gray, threshold float64) *image.Gray {
	bounds := gray.Bounds()
	edges := image.NewGray(bounds)

	// Sobel kernels
	gx := [][]int{
		{-1, 0, 1},
		{-2, 0, 2},
		{-1, 0, 1},
	}
	gy := [][]int{
		{-1, -2, -1},
		{0, 0, 0},
		{1, 2, 1},
	}

	for y := bounds.Min.Y + 1; y < bounds.Max.Y-1; y++ {
		for x := bounds.Min.X + 1; x < bounds.Max.X-1; x++ {
			var sumX, sumY float64

			// Apply convolution
			for ky := -1; ky <= 1; ky++ {
				for kx := -1; kx <= 1; kx++ {
					pixel := float64(gray.GrayAt(x+kx, y+ky).Y)
					sumX += pixel * float64(gx[ky+1][kx+1])
					sumY += pixel * float64(gy[ky+1][kx+1])
				}
			}

			// Gradient magnitude
			magnitude := math.Sqrt(sumX*sumX + sumY*sumY)

			// Threshold
			if magnitude > threshold {
				edges.SetGray(x, y, color.Gray{Y: 255})
			} else {
				edges.SetGray(x, y, color.Gray{Y: 0})
			}
		}
	}

	return edges
}

// dilate performs morphological dilation to connect nearby edges
func dilate(img *image.Gray, kernelSize, iterations int) *image.Gray {
	bounds := img.Bounds()
	result := image.NewGray(bounds)

	// Copy original
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			result.SetGray(x, y, img.GrayAt(x, y))
		}
	}

	half := kernelSize / 2

	for iter := 0; iter < iterations; iter++ {
		temp := image.NewGray(bounds)

		for y := bounds.Min.Y + half; y < bounds.Max.Y-half; y++ {
			for x := bounds.Min.X + half; x < bounds.Max.X-half; x++ {
				maxVal := uint8(0)

				// Check kernel neighborhood
				for ky := -half; ky <= half; ky++ {
					for kx := -half; kx <= half; kx++ {
						val := result.GrayAt(x+kx, y+ky).Y
						if val > maxVal {
							maxVal = val
						}
					}
				}

				temp.SetGray(x, y, color.Gray{Y: maxVal})
			}
		}

		result = temp
	}

	return result
}

// findContours finds bounding rectangles of connected white regions
func findContours(img *image.Gray) []image.Rectangle {
	bounds := img.Bounds()
	visited := make([][]bool, bounds.Dy())
	for i := range visited {
		visited[i] = make([]bool, bounds.Dx())
	}

	contours := []image.Rectangle{}

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if img.GrayAt(x, y).Y > 128 && !visited[y-bounds.Min.Y][x-bounds.Min.X] {
				// Found a new component, flood fill to find bounds
				rect := floodFill(img, visited, x, y)
				contours = append(contours, rect)
			}
		}
	}

	return contours
}

// floodFill performs flood fill and returns bounding rectangle
func floodFill(img *image.Gray, visited [][]bool, startX, startY int) image.Rectangle {
	bounds := img.Bounds()
	minX, minY := startX, startY
	maxX, maxY := startX, startY

	stack := []image.Point{{X: startX, Y: startY}}

	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		x, y := p.X, p.Y

		if x < bounds.Min.X || x >= bounds.Max.X || y < bounds.Min.Y || y >= bounds.Max.Y {
			continue
		}

		if visited[y-bounds.Min.Y][x-bounds.Min.X] || img.GrayAt(x, y).Y <= 128 {
			continue
		}

		visited[y-bounds.Min.Y][x-bounds.Min.X] = true

		// Update bounds
		if x < minX {
			minX = x
		}
		if x > maxX {
			maxX = x
		}
		if y < minY {
			minY = y
		}
		if y > maxY {
			maxY = y
		}

		// Add neighbors
		stack = append(stack,
			image.Point{X: x + 1, Y: y},
			image.Point{X: x - 1, Y: y},
			image.Point{X: x, Y: y + 1},
			image.Point{X: x, Y: y - 1},
		)
	}

	return image.Rect(minX, minY, maxX+1, maxY+1)
}
