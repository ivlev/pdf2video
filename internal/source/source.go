package source

import (
	"image"

	"github.com/gen2brain/go-fitz"
)

type Source interface {
	PageCount() int
	GetPageDimensions(index int) (width, height float64, err error)
	RenderPage(index int, dpi int) (image.Image, error)
	Close() error
}

type FitzPDFSource struct {
	doc  *fitz.Document
	path string
}

func NewFitzPDFSource(path string) (*FitzPDFSource, error) {
	doc, err := fitz.New(path)
	if err != nil {
		return nil, err
	}
	return &FitzPDFSource{doc: doc, path: path}, nil
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
	workerDoc, err := fitz.New(f.path)
	if err != nil {
		return nil, err
	}
	defer workerDoc.Close()
	return workerDoc.ImageDPI(index, float64(dpi))
}

func (f *FitzPDFSource) Close() error {
	return f.doc.Close()
}
