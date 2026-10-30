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
	if trailingOnes > 0 {
		bits := r.U(trailingOnes)
		for i := 0; i < trailingOnes; i++ {
			if (bits>>uint(trailingOnes-1-i))&1 == 1 {
				level[i] = -1
			} else {
				level[i] = 1
			}
		}
	}
	if totalCoeff <= trailingOnes {
		return level
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
			case levelPrefix == 15:
				levelCode = 30 + int(r.U(12))
			default: // escape (High profiles, large levels)
				levelCode = levelEscape(r, levelPrefix, 0)
			}
		} else if levelPrefix >= 16 {
			levelCode = levelEscape(r, levelPrefix, 1)
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
		var levelCode int
		if levelPrefix >= 16 {
			levelCode = levelEscape(r, levelPrefix, suffixLength)
		} else {
			size := suffixLength
			if levelPrefix >= 15 {
				size = 12
			}
			levelCode = (levelPrefix << suffixLength) + int(r.U(size))
		}
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

// maxLevelPrefix bounds level_prefix: the spec allows at most 11+bitDepth
// (25 at 14-bit); anything longer is a corrupt stream.
const maxLevelPrefix = 25

// levelEscape returns levelCode for level_prefix >= 16 (spec 9.2.2.1):
// a (levelPrefix-3)-bit suffix plus (1<<(levelPrefix-3))-4096. These codes
// only occur for very large levels (High profiles, high bit depth). Corrupt
// prefixes set the reader error instead of reading unbounded suffixes.
func levelEscape(r *bitstream.Reader, levelPrefix, suffixLength int) int {
	if levelPrefix > maxLevelPrefix {
		r.Fail()
		return 0
	}
	code := (15 << uint(suffixLength)) + int(r.U(levelPrefix-3))
	if suffixLength == 0 {
		code += 15
	}
	return code + (1 << uint(levelPrefix-3)) - 4096
}
