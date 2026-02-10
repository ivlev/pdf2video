package source

import (
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"sort"
)

type ImageSource struct {
	paths []string
}

func NewImageSource(path string) (*ImageSource, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	var paths []string
	if fi.IsDir() {
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				ext := filepath.Ext(entry.Name())
				if ext == ".jpg" || ext == ".jpeg" || ext == ".png" {
					paths = append(paths, filepath.Join(path, entry.Name()))
				}
			}
		}
		sort.Strings(paths)
	} else {
		paths = []string{path}
	}

	return &ImageSource{paths: paths}, nil
}

func (s *ImageSource) PageCount() int {
	return len(s.paths)
}

func (s *ImageSource) GetPageDimensions(index int) (float64, float64, error) {
	f, err := os.Open(s.paths[index])
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	img, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0, err
	}
	return float64(img.Width), float64(img.Height), nil
}

func (s *ImageSource) RenderPage(index int, dpi int) (image.Image, error) {
	f, err := os.Open(s.paths[index])
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	return img, nil
}

func (s *ImageSource) Close() error {
	return nil
}
