package vc1

// Picture is a decoded 4:2:0 I-picture.
type Picture struct {
	Y, Cb, Cr        []byte
	YStride, CStride int
	Width, Height    int
	FullRange        bool
}
