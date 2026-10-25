//go:build amd64 && !purego

#include "textflag.h"

// func reconstructLuma4x4Asm(y unsafe.Pointer, stride int, px int, py int, pred unsafe.Pointer, residual unsafe.Pointer)
TEXT ·reconstructLuma4x4Asm(SB), NOSPLIT, $0-48
	MOVQ y+0(FP), DI
	MOVQ stride+8(FP), R8
	MOVQ px+16(FP), R9
	MOVQ py+24(FP), R10
	MOVQ pred+32(FP), SI
	MOVQ residual+40(FP), R11

	MOVQ R10, AX
	IMULQ R8, AX
	ADDQ R9, AX
	ADDQ DI, AX

	XORQ R12, R12
rowloop:
	CMPQ R12, $4
	JGE done
	MOVQ R12, BX
	SHLQ $2, BX
	MOVQ $0, R14
rowx:
	CMPQ R14, $4
	JGE rowend
	MOVBQZX (SI)(BX*1), R13
	MOVL (R11)(BX*4), R15
	SHLQ $32, R15
	SARQ $32, R15
	ADDQ R15, R13
	CMPQ R13, $0
	JGE cliphi
	MOVQ $0, R13
	JMP writedst
cliphi:
	CMPQ R13, $255
	JLE writedst
	MOVQ $255, R13
writedst:
	MOVB R13, (AX)(R14*1)
	INCQ R14
	INCQ BX
	JMP rowx
rowend:
	ADDQ R8, AX
	INCQ R12
	JMP rowloop
done:
	RET
