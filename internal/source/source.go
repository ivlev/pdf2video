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
	Close() error
}

type FitzPDFSource struct {
	doc  *fitz.Document
	path string
	pool sync.Pool
}

func NewFitzPDFSource(path string) (*FitzPDFSource, error) {
	doc, err := fitz.New(path)
	if err != nil {
		return nil, err
	}

	f := &FitzPDFSource{
		doc:  doc,
		path: path,
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
	html, err := f.doc.HTML(index, false)
	if err != nil {
		return nil, err
	}

	// MuPDF HTML format for blocks usually looks like:
	// <div class="block" style="left:70.866pt;top:70.866pt;width:453.54pt;height:67.165pt">
	// Or similar. We'll use a regex to extract these bounding boxes.
	// Note: dimensions are in points (1/72 inch). We need to convert to pixels if necessary,
	// but the Director/Scenarios usually expect points/percentages relative to the source.
	// However, the internal coordinate system for CV is pixels.
	// Default DPI is 72 for these coordinates in Fitz HTML.

	re := regexp.MustCompile(`left:([\d.]+)pt;top:([\d.]+)pt;width:([\d.]+)pt;height:([\d.]+)pt`)
	matches := re.FindAllStringSubmatch(html, -1)

	var blocks []analyzer.Block
	for _, m := range matches {
		left, _ := strconv.ParseFloat(m[1], 64)
		top, _ := strconv.ParseFloat(m[2], 64)
		width, _ := strconv.ParseFloat(m[3], 64)
		height, _ := strconv.ParseFloat(m[4], 64)

		blocks = append(blocks, analyzer.Block{
			Rect: image.Rect(
				int(left),
				int(top),
				int(left+width),
				int(top+height),
			),
			Type:       analyzer.BlockTypeText,
			Confidence: 1.0,
			Score:      1.0,
		})
	}

	return blocks, nil
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
