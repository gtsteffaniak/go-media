package h264

import (
	"sync"

	"github.com/gtsteffaniak/go-media/internal/h264/intradec"
)

var decoderPool = sync.Pool{
	New: func() interface{} { return intradec.New() },
}

func borrowDecoder(fast bool) *intradec.Decoder {
	dec := decoderPool.Get().(*intradec.Decoder)
	dec.SkipDeblock = fast
	return dec
}

func releaseDecoder(dec *intradec.Decoder) {
	decoderPool.Put(dec)
}
