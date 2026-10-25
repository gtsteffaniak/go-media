package thumb

import (
	"bytes"
	"image"
	"image/jpeg"
)

// jpegDecode prefers YCbCr decode for faster downstream YCbCr scaling.
func jpegDecode(data []byte) (image.Image, string, error) {
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return image.Decode(bytes.NewReader(data))
	}
	return img, "jpeg", nil
}
