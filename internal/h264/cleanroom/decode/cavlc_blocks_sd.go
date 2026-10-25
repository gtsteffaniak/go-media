package decode

import "github.com/gtsteffaniak/go-media/internal/h264/cleanroom/bitstream"

// residualBlockCAVLC decodes into sd.coeffScratch[:maxNumCoeff] and returns TotalCoeff.
// Coefficients are in scan order; the scratch buffer is reused (zero heap allocs).
func (sd *sliceDecoder) residualBlockCAVLC(r *bitstream.Reader, nC, maxNumCoeff int) int {
	if maxNumCoeff > 16 {
		panic("residualBlockCAVLC: maxNumCoeff > 16")
	}
	coeff := sd.coeffScratch[:maxNumCoeff]
	for i := range coeff {
		coeff[i] = 0
	}
	trailingOnes, totalCoeff := decodeCoeffToken(r, nC)
	if totalCoeff == 0 {
		return 0
	}
	levels := readLevelsInto(r, trailingOnes, totalCoeff, sd.levelScratch[:totalCoeff])

	zerosLeft := 0
	if totalCoeff < maxNumCoeff {
		if nC == -1 {
			zerosLeft = decodeTotalZerosChromaDC(r, totalCoeff)
		} else {
			zerosLeft = decodeTotalZeros(r, totalCoeff)
		}
	}

	runs := sd.runsScratch[:totalCoeff]
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
	return totalCoeff
}
