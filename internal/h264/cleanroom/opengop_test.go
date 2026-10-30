package cleanroom

import "testing"

// Hand-built NAL units: header byte, then RBSP bits for first_mb_in_slice and
// slice_type (the probe reads nothing else from a slice).
var (
	nonIDRISlice = []byte{0x21, 0x88, 0x80} // ue(0)=1, ue(7)=0001000 (I, all slices)
	nonIDRPSlice = []byte{0x21, 0x98, 0x80} // ue(0)=1, ue(5)=00110   (P, all slices)
	idrISlice    = []byte{0x65, 0x88, 0x80}
	seiRecovery0 = []byte{0x06, 0x06, 0x01, 0x80, 0x80} // recovery_point, recovery_frame_cnt=0
	seiRecovery1 = []byte{0x06, 0x06, 0x01, 0x40, 0x80} // recovery_frame_cnt=1
)

func TestOpenGOPKeyframe(t *testing.T) {
	cases := []struct {
		name  string
		nalus [][]byte
		want  bool
	}{
		{"all-I non-IDR", [][]byte{nonIDRISlice, nonIDRISlice}, true},
		{"P non-IDR", [][]byte{nonIDRPSlice}, false},
		{"mixed I and P", [][]byte{nonIDRISlice, nonIDRPSlice}, false},
		{"recovery point 0 with P", [][]byte{seiRecovery0, nonIDRPSlice}, true},
		{"recovery point 1 with P", [][]byte{seiRecovery1, nonIDRPSlice}, false},
		{"IDR handled by normal path", [][]byte{idrISlice, nonIDRISlice}, false},
		{"no slices", [][]byte{seiRecovery0}, false},
	}
	for _, c := range cases {
		if got := openGOPKeyframe(c.nalus); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}
