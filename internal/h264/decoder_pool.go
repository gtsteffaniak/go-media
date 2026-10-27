package h264

import (
	"sync"

	"github.com/gtsteffaniak/go-media/internal/h264/cleanroom"
)

var decoderPool = sync.Pool{
	New: func() interface{} { return cleanroom.New() },
}

func borrowDecoder(fast bool) *cleanroom.Decoder {
	dec := decoderPool.Get().(*cleanroom.Decoder)
	dec.Reset()
	dec.SkipDeblock = fast
	return dec
}

func releaseDecoder(dec *cleanroom.Decoder) {
	decoderPool.Put(dec)
}
