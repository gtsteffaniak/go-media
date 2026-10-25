package thumb

import (
	"bytes"
	"image"
	"image/png"
)

// RasterPNG encodes the scaled decode path as PNG (lossless).
func RasterPNG(r interface{ ReadAt([]byte, int64) (int, error) }, size int64, opt Options) ([]byte, string, error) {
	normalizeOptions(&opt)
	img, src, err := sourceImage(r, size, opt)
	if err != nil {
		return nil, "", err
	}
	fl := filterFromOptions(opt)
	img = scaleDown(img, opt.MaxWidth, opt.MaxHeight, fl)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), src, nil
}

// Raster returns the scaled image before lossy encode.
func Raster(r interface{ ReadAt([]byte, int64) (int, error) }, size int64, opt Options) (image.Image, string, error) {
	data, src, err := RasterPNG(r, size, opt)
	if err != nil {
		return nil, "", err
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", err
	}
	return img, src, nil
}
