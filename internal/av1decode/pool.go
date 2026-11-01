package av1decode

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"

	"github.com/gtsteffaniak/go-media/internal/h264"
)

type pooledDecoder struct {
	dec     *Decoder
	maxUnit int
}

var (
	poolMu sync.Mutex
	pool   = map[string]*pooledDecoder{}
)

func configCacheKey(av1CExtradata []byte) string {
	if len(av1CExtradata) == 0 {
		return "empty"
	}
	sum := sha256.Sum256(av1CExtradata)
	return hex.EncodeToString(sum[:8])
}

// DecodeSampleCached reuses a process-wide KeyframeDecoder per av1C extradata.
func DecodeSampleCached(av1CExtradata, sample []byte) (*h264.ThumbYCbCr, error) {
	key := configCacheKey(av1CExtradata)
	poolMu.Lock()
	p := pool[key]
	var cached *Decoder
	if p != nil {
		cached = p.dec
	}
	poolMu.Unlock()
	if cached != nil {
		ycc, err := cached.DecodeSample(av1CExtradata, sample)
		if err == nil {
			return ycc, nil
		}
	}
	unit, err := ProbeTemporalUnit(av1CExtradata, sample)
	if err != nil {
		return nil, err
	}
	poolMu.Lock()
	defer poolMu.Unlock()
	p = pool[key]
	if p == nil || len(unit) > p.maxUnit {
		if p != nil {
			p.dec.Close()
		}
		dec, err := NewDecoderFromProbe(unit)
		if err != nil {
			return nil, err
		}
		p = &pooledDecoder{dec: dec, maxUnit: len(unit)}
		pool[key] = p
	}
	return p.dec.DecodeSample(av1CExtradata, sample)
}
