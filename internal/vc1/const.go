package vc1

var blockMap = [6]int{0, 2, 1, 3, 4, 5}

var dcPred = [...]int16{
	-1, 1024, 512, 341, 256, 205, 171, 146, 128,
	114, 102, 93, 85, 79, 73, 68, 64,
	60, 57, 54, 51, 49, 47, 45, 43,
	41, 39, 38, 37, 35, 34, 33,
}

var dcpred = dcPred

const (
	csHighRateIntra = 0
	csHighMotIntra  = 1
	csMidRateIntra  = 2
	csLowMotIntra   = 3
	csHighRateInter = 4
	csHighMotInter  = 5
	csMidRateInter  = 6
	csLowMotInter   = 7
)

func codingSetY(pqIndex, table int) int {
	switch table {
	case 0:
		if pqIndex <= 8 {
			return csHighRateIntra
		}
		return csLowMotIntra
	case 1:
		return csHighMotIntra
	case 2:
		return csMidRateIntra
	default:
		return csHighRateIntra
	}
}

func codingSetC(pqIndex, table int) int {
	switch table {
	case 0:
		if pqIndex <= 8 {
			return csHighRateInter
		}
		return csLowMotInter
	case 1:
		return csHighMotInter
	case 2:
		return csMidRateInter
	default:
		return csHighRateInter
	}
}

func abs16(v int16) int {
	if v < 0 {
		return -int(v)
	}
	return int(v)
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
