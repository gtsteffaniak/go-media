package decode

import "github.com/gtsteffaniak/go-media/internal/h264/cleanroom/bitstream"

// readLevelPrefix counts the leading zeros and consumes the trailing one
// (level_prefix).
func readLevelPrefix(r *bitstream.Reader) int {
	if r.Err() != nil {
		return 0
	}
	return r.ReadUnaryPrefix()
}

// readLevels reads coefficient levels into dst (len ≥ totalCoeff).
func readLevels(r *bitstream.Reader, trailingOnes, totalCoeff int) []int32 {
	var buf [16]int32
	return readLevelsInto(r, trailingOnes, totalCoeff, buf[:totalCoeff])
}

func readLevelsInto(r *bitstream.Reader, trailingOnes, totalCoeff int, level []int32) []int32 {

	// Signs of trailing ones: bit 1 → -1, 0 → +1 (most significant first).
	for i := 0; i < trailingOnes; i++ {
		if r.Flag() {
			level[i] = -1
		} else {
			level[i] = 1
		}
	}

	i := trailingOnes
	suffixLength := 1

	if totalCoeff > trailingOnes {
		levelPrefix := readLevelPrefix(r)
		var levelCode int
		if totalCoeff < 11 || trailingOnes == 3 { // equivalent to suffixLength==0
			switch {
			case levelPrefix < 14:
				levelCode = levelPrefix
			case levelPrefix == 14:
				levelCode = 14 + int(r.U(4))
			default: // 15
				levelCode = 30 + int(r.U(12))
			}
		} else {
			size := suffixLength
			if levelPrefix >= 15 {
				size = 12
			}
			levelCode = (levelPrefix << 1) + int(r.U(size))
		}
		if trailingOnes < 3 {
			levelCode += 2
		}
		level[i] = int32((levelCode + 2) >> 1)
		if level[i] > 3 {
			suffixLength = 2
		}
		if levelCode&1 != 0 {
			level[i] = -level[i]
		}
		i++
	}

	for ; i < totalCoeff; i++ {
		levelPrefix := readLevelPrefix(r)
		size := suffixLength
		if levelPrefix >= 15 {
			size = 12
		}
		levelCode := (levelPrefix << suffixLength) + int(r.U(size))
		level[i] = int32((levelCode >> 1) + 1)
		if level[i] > int32(3<<(suffixLength-1)) && suffixLength < 6 {
			suffixLength++
		}
		if levelCode&1 != 0 {
			level[i] = -level[i]
		}
	}
	return level
}
