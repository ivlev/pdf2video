package source

import (
	"image"
	"sync"

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

func (f *FitzPDFSource) Close() error {
	return f.doc.Close()
}
