package h264

import (
	"sync"

	"github.com/Eyevinn/hi264/pkg/decoder"
)

var decoderPool = sync.Pool{
	New: func() interface{} { return decoder.New() },
}

func borrowDecoder(fast bool) *decoder.Decoder {
	dec := decoderPool.Get().(*decoder.Decoder)
	dec.SkipDeblock = fast
	return dec
}

func releaseDecoder(dec *decoder.Decoder) {
	decoderPool.Put(dec)
}
