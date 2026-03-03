package source

import (
	"crypto/sha256"
	"fmt"
	"image"
	"regexp"
	"strconv"
	"sync"

	"github.com/gen2brain/go-fitz"
	"github.com/ivlev/pdf2video/internal/analyzer"
)

type Source interface {
	PageCount() int
	GetPageDimensions(index int) (width, height float64, err error)
	RenderPage(index int, dpi int) (image.Image, error)
	GetTextBlocks(index int) ([]analyzer.Block, error)
	GetPageHash(index int) (string, error)
	HasTextLayer(index int) bool
	SetDPI(dpi int)
	Close() error
}

type FitzPDFSource struct {
	doc  *fitz.Document
	path string
	pool sync.Pool
	dpi  int
}

func NewFitzPDFSource(path string) (*FitzPDFSource, error) {
	doc, err := fitz.New(path)
	if err != nil {
		return nil, err
	}

	f := &FitzPDFSource{
		doc:  doc,
		path: path,
		dpi:  300, // Default
	}

	f.pool.New = func() interface{} {
		d, err := fitz.New(f.path)
		if err != nil {
			return nil
		}
		return d
	}

	return f, nil
}

func (f *FitzPDFSource) PageCount() int {
	return f.doc.NumPage()
}

func (f *FitzPDFSource) GetPageDimensions(index int) (float64, float64, error) {
	rect, err := f.doc.Bound(index)
	if err != nil {
		return 0, 0, err
	}
	return float64(rect.Dx()), float64(rect.Dy()), nil
}

func (f *FitzPDFSource) RenderPage(index int, dpi int) (image.Image, error) {
	docObj := f.pool.Get()
	if docObj == nil {
		return nil, image.ErrFormat // Or a more appropriate error
	}
	workerDoc := docObj.(*fitz.Document)
	defer f.pool.Put(workerDoc)

	return workerDoc.ImageDPI(index, float64(dpi))
}

func (f *FitzPDFSource) GetTextBlocks(index int) ([]analyzer.Block, error) {
	// MuPDF's Text() method returns structured text including block information.
	// The format is generally block-based, often with coordinates in the output
	// or structured in a way that fitz-go can potentially expose.
	// Since go-fitz's Text() returns a string, we'll use it to detect if text exists,
	// but for REAL block coordinates, we should ideally use a more advanced method
	// if go-fitz supports it. Checking go-fitz, it has a Blocks() method in newer versions.

	// Let's try to use PlainText or similar if available, or fallback to a simpler approach.
	// However, the user reported that camera doesn't move.
	// This is because our HTML regex failed to find ANY blocks.

	// FIX: Use go-fitz's structured text if possible.
	// In the absence of a direct "Blocks" method in this version of go-fitz,
	// we will use the text layer to at least identify that there IS content.
	// BUT to fix the "no movement" issue, we need valid coordinates.

	// Re-evaluating HTML parsing: the issue was the regex being too strict.
	// Let's use a MUCH simpler regex that just finds ANY 'left:pt' etc.
	html, err := f.doc.HTML(index, false)
	if err != nil {
		return nil, err
	}

	// MuPDF HTML style attribute is notoriously inconsistent.
	// We'll search for both <div> (containers/blocks) and <p> (lines) tags.
	reTags := regexp.MustCompile(`(?i)<(?:div|p)[^>]+style="([^"]+)"`)
	matches := reTags.FindAllStringSubmatch(html, -1)

	var blocks []analyzer.Block
	dpi := f.dpi
	if dpi <= 0 {
		dpi = 300
	}
	scale := float64(dpi) / 72.0

	for _, m := range matches {
		style := m[1]

		var left, top, width, height float64
		var foundL, foundT bool
		var foundW, foundH bool

		// Extract values. Some tags only have left/top (like <p>),
		// some have all four (like <div>).
		parts := regexp.MustCompile(`([a-z-]+):\s*(-?[\d.]+)pt`).FindAllStringSubmatch(style, -1)
		for _, p := range parts {
			val, _ := strconv.ParseFloat(p[2], 64)
			switch p[1] {
			case "left":
				left = val
				foundL = true
			case "top":
				top = val
				foundT = true
			case "width":
				width = val
				foundW = true
			case "height":
				height = val
				foundH = true
			}
		}

		if foundL && foundT {
			// If it's a <p> tag or similar, it might only have left/top.
			// We'll estimate a reasonable width/height for a line if missing.
			if !foundW {
				width = 200 // reasonable minimum line width
			}
			if !foundH {
				height = 15 // reasonable line height
			}

			// Skip page-level div (usually fills most of the page)
			if width > 500 && height > 500 {
				continue
			}

			// Ignore noise
			if width < 3 || height < 3 {
				continue
			}

			blocks = append(blocks, analyzer.Block{
				Rect: image.Rect(
					int(left*scale),
					int(top*scale),
					int((left+width)*scale),
					int((top+height)*scale),
				),
				Type:       analyzer.BlockTypeText,
				Confidence: 1.0,
				Score:      1.0,
			})
		}
	}

	// If we still found nothing but THERE IS text, we create one big block for the page
	// to at least allow some movement or centering.
	if len(blocks) == 0 && f.HasTextLayer(index) {
		w, h, _ := f.GetPageDimensions(index)
		blocks = append(blocks, analyzer.Block{
			Rect: image.Rect(
				int(w*0.1*scale),
				int(h*0.1*scale),
				int(w*0.9*scale),
				int(h*0.9*scale),
			),
			Type: analyzer.BlockTypeText,
		})
	}

	return blocks, nil
}

func (f *FitzPDFSource) SetDPI(dpi int) {
	f.dpi = dpi
}

func (f *FitzPDFSource) HasTextLayer(index int) bool {
	text, err := f.doc.Text(index)
	return err == nil && len(text) > 0
}

func (f *FitzPDFSource) GetPageHash(index int) (string, error) {
	// Мы используем HTML-представление страницы как прокси для её содержимого.
	// Это включает текст, структуру и ссылки, что достаточно для детекции изменений.
	html, err := f.doc.HTML(index, false)
	if err != nil {
		return "", err
	}

	// Также добавляем путь к файлу и индекс, чтобы избежать коллизий между разными PDF
	// с одинаковым контентом одной страницы (если нужно)
	data := fmt.Sprintf("%s|%d|%s", f.path, index, html)
	h := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", h), nil
}

func (f *FitzPDFSource) Close() error {
	return f.doc.Close()
}
