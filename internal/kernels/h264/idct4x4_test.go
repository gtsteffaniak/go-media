package h264

import "testing"

func TestInvTransform4x4DCOnly(t *testing.T) {
	block := []int16{16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	dst := make([]int16, 16)
	InvTransform4x4(block, dst)
	for i, v := range dst {
		if v != 16 {
			t.Fatalf("coeff %d: got %d want 16", i, v)
		}
	}
}
