package decode

import (
	"math/rand"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/h264/cleanroom/bitstream"
)

// refCabac is the original spec-literal engine (bit-by-bit renormalization). It is the
// bit-exact reference the optimized cabacEngine is checked against.
type refCabac struct {
	r          *bitstream.Reader
	codIRange  uint32
	codIOffset uint32
	ctx        [numCabacCtx]cabacCtx
}

func newRefCabac(data []byte, sliceQP int) *refCabac {
	e := &refCabac{r: bitstream.NewReader(data), codIRange: 510}
	e.codIOffset = e.r.U(9)
	e2 := &cabacEngine{}
	e2.initContexts(sliceQP, 0)
	e.ctx = e2.ctx
	return e
}

func (e *refCabac) renorm() {
	for e.codIRange < 256 {
		e.codIRange <<= 1
		e.codIOffset = (e.codIOffset << 1) | e.r.Bit()
	}
}

func (e *refCabac) decodeDecision(ctxIdx int) uint32 {
	c := &e.ctx[ctxIdx]
	rangeLPS := uint32(rangeTabLPS[c.state][(e.codIRange>>6)&3])
	e.codIRange -= rangeLPS
	var binVal uint32
	if e.codIOffset >= e.codIRange {
		binVal = uint32(1 - c.mps)
		e.codIOffset -= e.codIRange
		e.codIRange = rangeLPS
		if c.state == 0 {
			c.mps = 1 - c.mps
		}
		c.state = stateTrans[c.state][0]
	} else {
		binVal = uint32(c.mps)
		c.state = stateTrans[c.state][1]
	}
	e.renorm()
	return binVal
}

func (e *refCabac) decodeBypass() uint32 {
	e.codIOffset = (e.codIOffset << 1) | e.r.Bit()
	if e.codIOffset >= e.codIRange {
		e.codIOffset -= e.codIRange
		return 1
	}
	return 0
}

func (e *refCabac) decodeTerminate() uint32 {
	e.codIRange -= 2
	if e.codIOffset >= e.codIRange {
		return 1
	}
	e.renorm()
	return 0
}

// diffCabac drives both engines with the same operation sequence and fails on the
// first divergence in any decoded bin, range, offset or context state.
func diffCabac(t testing.TB, data []byte, ops []byte, qp int) {
	t.Helper()
	if len(data) < 2 {
		return
	}
	// A conforming stream never starts with codIOffset >= 510.
	if first9 := uint32(data[0])<<1 | uint32(data[1])>>7; first9 >= 510 {
		return
	}
	ref := newRefCabac(data, qp)
	got := newCabacDecoder(bitstream.NewReader(data), qp, 0)
	for i, op := range ops {
		var a, b uint32
		var name string
		switch {
		case op < 200:
			ctx := int(op) * 7 % numCabacCtx
			a, b, name = ref.decodeDecision(ctx), got.decodeDecision(ctx), "decision"
		case op < 250:
			a, b, name = ref.decodeBypass(), got.decodeBypass(), "bypass"
		default:
			if ref.codIRange <= 2 {
				continue
			}
			a, b, name = ref.decodeTerminate(), got.decodeTerminate(), "terminate"
			if a == 1 {
				// A terminated slice ends here; the engines are not compared further.
				if a != b {
					t.Fatalf("op %d terminate: got %d want %d", i, b, a)
				}
				return
			}
		}
		if a != b || ref.codIRange != got.codIRange || ref.codIOffset != got.offset() {
			t.Fatalf("op %d (%s): bin got %d want %d; range got %d want %d; offset got %d want %d",
				i, name, b, a, got.codIRange, ref.codIRange, got.offset(), ref.codIOffset)
		}
	}
	if ref.ctx != got.ctx {
		t.Fatal("context states diverged")
	}
	if (ref.r.Err() != nil) != (got.err() != nil) {
		t.Fatalf("overrun flag: ref err=%v got err=%v", ref.r.Err(), got.err())
	}
}

func TestCabacEngineMatchesReference(t *testing.T) {
	for seed := int64(0); seed < 400; seed++ {
		rng := rand.New(rand.NewSource(seed))
		data := make([]byte, 16+rng.Intn(600))
		rng.Read(data)
		ops := make([]byte, 1+rng.Intn(6000))
		rng.Read(ops)
		diffCabac(t, data, ops, rng.Intn(52))
	}
}

// TestCabacEngineRunsPastEnd feeds far more operations than the data holds so the
// zero-padded tail and the overrun flag are exercised.
func TestCabacEngineRunsPastEnd(t *testing.T) {
	for seed := int64(0); seed < 100; seed++ {
		rng := rand.New(rand.NewSource(1000 + seed))
		data := make([]byte, 2+rng.Intn(9))
		rng.Read(data)
		ops := make([]byte, 400)
		for i := range ops {
			ops[i] = byte(rng.Intn(250)) // no terminate, so decoding runs on
		}
		diffCabac(t, data, ops, rng.Intn(52))
	}
}

func FuzzCabacEngine(f *testing.F) {
	f.Add([]byte{0x12, 0x34, 0x56, 0x78, 0x9a}, []byte{1, 2, 3, 210, 255}, 26)
	f.Fuzz(func(t *testing.T, data, ops []byte, qp int) {
		if qp < 0 {
			qp = -qp
		}
		diffCabac(t, data, ops, qp%52)
	})
}
