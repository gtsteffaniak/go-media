package decode

import "github.com/gtsteffaniak/go-media/internal/h264/cleanroom/bitstream"

// zigzag4x4 — frame zig-zag scan: scan position → raster index of
// the 4x4 block (row*4 + col).
var zigzag4x4 = [16]int{0, 1, 4, 8, 5, 2, 3, 6, 9, 12, 13, 10, 7, 11, 14, 15}

// residualBlockCAVLC parses a single residual block and returns the
// coefficients in scan order (index = scan position), of length maxNumCoeff,
// plus the number of nonzero ones. nC — the neighbor context (or -1 for chroma DC).
func residualBlockCAVLC(r *bitstream.Reader, nC, maxNumCoeff int) ([]int32, int) {
	if maxNumCoeff > 16 {
		panic("residualBlockCAVLC: maxNumCoeff > 16")
	}
	trailingOnes, totalCoeff := decodeCoeffToken(r, nC)
	var coeffBuf [16]int32
	coeff := coeffBuf[:maxNumCoeff]
	if totalCoeff == 0 {
		return coeff, 0
	}
	var levelBuf [16]int32
	levels := readLevelsInto(r, trailingOnes, totalCoeff, levelBuf[:totalCoeff])

	zerosLeft := 0
	if totalCoeff < maxNumCoeff {
		if nC == -1 {
			zerosLeft = decodeTotalZerosChromaDC(r, totalCoeff)
		} else {
			zerosLeft = decodeTotalZeros(r, totalCoeff)
		}
	}

	var runsBuf [16]int
	runs := runsBuf[:totalCoeff]
	for i := 0; i < totalCoeff-1; i++ {
		if zerosLeft > 0 {
			runs[i] = decodeRunBefore(r, zerosLeft)
		}
		zerosLeft -= runs[i]
	}
	if zerosLeft < 0 {
		zerosLeft = 0
	}
	runs[totalCoeff-1] = zerosLeft

	coeffNum := -1
	for i := totalCoeff - 1; i >= 0; i-- {
		coeffNum += runs[i] + 1
		if coeffNum >= 0 && coeffNum < maxNumCoeff {
			coeff[coeffNum] = levels[i]
		}
	}
	return coeff, totalCoeff
}

// inverseScan4x4 places 16 coefficients from scan order into the raster order of
// the 4x4 block (zig-zag, frame MB).
func inverseScan4x4(scan []int32) [16]int32 {
	var block [16]int32
	for s := 0; s < 16 && s < len(scan); s++ {
		block[zigzag4x4[s]] = scan[s]
	}
	return block
}
