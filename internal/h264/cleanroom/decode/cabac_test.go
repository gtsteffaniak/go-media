package decode

import "testing"

func TestCabacInit(t *testing.T) {
	// codIRange=510; codIOffset = first 9 bits. "101010101" = 0b101010101 = 341.
	e := newCabacEngine(bitsReader("101010101"+"0000000"), 4)
	if e.codIRange != 510 {
		t.Errorf("codIRange=%d, want 510", e.codIRange)
	}
	if e.codIOffset != 0b101010101 {
		t.Errorf("codIOffset=%d, want %d", e.codIOffset, 0b101010101)
	}
}

func TestCabacInitContexts(t *testing.T) {
	// I-slice (model 0), SliceQP=26.
	// ctx[0]: m=20,n=-15 → pre=clip(((20*26)>>4)-15)=32-15=17 → state=63-17=46, mps=0.
	// ctx[1]: m=2,n=54  → pre=((2*26)>>4)+54=3+54=57 → state=63-57=6, mps=0.
	e := newCabacDecoder(bitsReader("0000000000"), 26, 0)
	if e.ctx[0].state != 46 || e.ctx[0].mps != 0 {
		t.Errorf("ctx[0] = {%d,%d}, want {46,0}", e.ctx[0].state, e.ctx[0].mps)
	}
	if e.ctx[1].state != 6 || e.ctx[1].mps != 0 {
		t.Errorf("ctx[1] = {%d,%d}, want {6,0}", e.ctx[1].state, e.ctx[1].mps)
	}
	if len(e.ctx) != 460 {
		t.Errorf("context count = %d, want 460", len(e.ctx))
	}
}

func TestCabacDecodeDecisionMPS(t *testing.T) {
	// state=0,mps=0, range=510, offset=100. rangeLPS=rangeTabLPS[0][3]=240 →
	// range=270; 100<270 → MPS path: binVal=0, state→transIdxMPS[0]=1, no renorm.
	e := &cabacEngine{r: bitsReader("00000000"), codIRange: 510, codIOffset: 100, ctx: make([]cabacCtx, 1)}
	if b := e.decodeDecision(0); b != 0 {
		t.Fatalf("binVal=%d, want 0 (MPS)", b)
	}
	if e.ctx[0].state != 1 || e.codIRange != 270 || e.codIOffset != 100 {
		t.Errorf("state=%d range=%d offset=%d, want 1/270/100", e.ctx[0].state, e.codIRange, e.codIOffset)
	}
}

func TestCabacDecodeDecisionLPS(t *testing.T) {
	// state=0,mps=0, range=510, offset=300. range=270; 300>=270 → LPS path:
	// binVal=1, offset=30, range=240, state0→mps flips to 1, state→transIdxLPS[0]=0,
	// renorm: 240<256 → range=480, offset=(30<<1)|0=60.
	e := &cabacEngine{r: bitsReader("00000000"), codIRange: 510, codIOffset: 300, ctx: make([]cabacCtx, 1)}
	if b := e.decodeDecision(0); b != 1 {
		t.Fatalf("binVal=%d, want 1 (LPS)", b)
	}
	if e.ctx[0].mps != 1 || e.ctx[0].state != 0 || e.codIRange != 480 || e.codIOffset != 60 {
		t.Errorf("mps=%d state=%d range=%d offset=%d, want 1/0/480/60",
			e.ctx[0].mps, e.ctx[0].state, e.codIRange, e.codIOffset)
	}
}

func TestCabacDecodeBypass(t *testing.T) {
	// range=300, offset=200, next bit=1 → offset=(200<<1)|1=401; 401>=300 → 1,
	// offset=101.
	e := &cabacEngine{r: bitsReader("1" + "0000000"), codIRange: 300, codIOffset: 200}
	if b := e.decodeBypass(); b != 1 {
		t.Fatalf("bypass=%d, want 1", b)
	}
	if e.codIOffset != 101 {
		t.Errorf("offset=%d, want 101", e.codIOffset)
	}
}

func TestCabacDecodeTerminate(t *testing.T) {
	// range=256, offset=300 → range=254; 300>=254 → terminate (1).
	e := &cabacEngine{r: bitsReader("00000000"), codIRange: 256, codIOffset: 300}
	if b := e.decodeTerminate(); b != 1 {
		t.Fatalf("terminate=%d, want 1", b)
	}
	// Not terminated: range=300, offset=100 → range=298; 100<298 → 0, no renorm.
	e = &cabacEngine{r: bitsReader("00000000"), codIRange: 300, codIOffset: 100}
	if b := e.decodeTerminate(); b != 0 {
		t.Fatalf("terminate=%d, want 0", b)
	}
	if e.codIRange != 298 || e.codIOffset != 100 {
		t.Errorf("range=%d offset=%d, want 298/100", e.codIRange, e.codIOffset)
	}
}
