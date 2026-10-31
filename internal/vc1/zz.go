package vc1

func blkShifts(resFastTX bool) (leftSh, topSh int) {
	if resFastTX {
		return 0, 3
	}
	return 3, 0
}

func acZZTables(resFastTX bool, acPred bool, topPred bool) []uint8 {
	if resFastTX {
		if acPred {
			if topPred {
				return zz8[2][:]
			}
			return zz8[3][:]
		}
		return zz8[1][:]
	}
	if acPred {
		if topPred {
			return wmv1ZZ[2][:]
		}
		return wmv1ZZ[3][:]
	}
	return wmv1ZZ[1][:]
}
