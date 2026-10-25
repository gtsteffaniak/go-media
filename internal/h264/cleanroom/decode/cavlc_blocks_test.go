package decode

import "testing"

func TestTotalZerosTC1(t *testing.T) {
	// TotalCoeff=1: "1"→0, "011"→1, "010"→2, "0011"→3.
	cases := []struct {
		bits string
		want int
		used int
	}{
		{"1", 0, 1},
		{"011", 1, 3},
		{"010", 2, 3},
		{"0011", 3, 4},
	}
	for _, c := range cases {
		r := bitsReader(c.bits + "000000000")
		if got := decodeTotalZeros(r, 1); got != c.want || r.BitPos() != c.used {
			t.Errorf("totalZeros(TC1,%q)=%d used=%d, want %d/%d", c.bits, got, r.BitPos(), c.want, c.used)
		}
	}
}

func TestRunBefore(t *testing.T) {
	cases := []struct {
		bits      string
		zerosLeft int
		want      int
		used      int
	}{
		{"1", 1, 0, 1},
		{"0", 1, 1, 1},
		{"1", 2, 0, 1},
		{"01", 2, 1, 2},
		{"00", 2, 2, 2},
		{"111", 7, 0, 3},  // zerosLeft>6: temp=7 → 0
		{"110", 7, 1, 3},  // temp=6 → 1
		{"0001", 7, 7, 4}, // 000 + unary "1" → 7
	}
	for _, c := range cases {
		r := bitsReader(c.bits + "00000000")
		if got := decodeRunBefore(r, c.zerosLeft); got != c.want || r.BitPos() != c.used {
			t.Errorf("runBefore(%q,zl=%d)=%d used=%d, want %d/%d",
				c.bits, c.zerosLeft, got, r.BitPos(), c.want, c.used)
		}
	}
}

func TestInverseScan4x4(t *testing.T) {
	// scan[1]→raster 1, scan[2]→raster 4, scan[3]→raster 8 (zig-zag).
	scan := make([]int32, 16)
	scan[0] = 9
	scan[1] = 5
	scan[2] = 7
	scan[3] = 3
	b := inverseScan4x4(scan)
	if b[0] != 9 || b[1] != 5 || b[4] != 7 || b[8] != 3 {
		t.Errorf("scan→raster: b[0]=%d b[1]=%d b[4]=%d b[8]=%d, want 9/5/7/3",
			b[0], b[1], b[4], b[8])
	}
}

func TestResidualBlockSingleCoeff(t *testing.T) {
	// nC=0, maxNumCoeff=16. coeff_token (T0=0,TC=1)="000101";
	// level: prefix "1" → +2; total_zeros "1" → 0. Result: coeff[0]=2.
	r := bitsReader("000101" + "1" + "1" + "00000000")
	coeff, tc := residualBlockCAVLC(r, 0, 16)
	if tc != 1 {
		t.Fatalf("totalCoeff=%d, want 1", tc)
	}
	if coeff[0] != 2 {
		t.Errorf("coeff[0]=%d, want 2", coeff[0])
	}
	for i := 1; i < 16; i++ {
		if coeff[i] != 0 {
			t.Errorf("coeff[%d]=%d, want 0", i, coeff[i])
		}
	}
}

func TestResidualBlockTwoCoeff(t *testing.T) {
	// nC=0. coeff_token (T0=1,TC=2)="000100"; trailing-one sign "0"→+1 (level[0]);
	// level prefix "1" → +2 (level[1]); total_zeros=1 ("110"); run_before "1"→0.
	// runs=[0,1] → coeffNum: i=1 → 1 (coeff[1]=level[1]=2); i=0 → 2 (coeff[2]=level[0]=1).
	r := bitsReader("000100" + "0" + "1" + "110" + "1" + "00000000")
	coeff, tc := residualBlockCAVLC(r, 0, 16)
	if tc != 2 {
		t.Fatalf("totalCoeff=%d, want 2", tc)
	}
	if coeff[1] != 2 || coeff[2] != 1 {
		t.Errorf("coeff[1]=%d coeff[2]=%d, want 2/1", coeff[1], coeff[2])
	}
	for i := 0; i < 16; i++ {
		if i != 1 && i != 2 && coeff[i] != 0 {
			t.Errorf("coeff[%d]=%d, want 0", i, coeff[i])
		}
	}
}
