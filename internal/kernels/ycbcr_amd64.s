//go:build amd64

#include "textflag.h"

// func areaDownRowYAsm(dstPtr unsafe.Pointer, srcPtr unsafe.Pointer, srcStride, sy0, sy1, x0, x1 int)
TEXT ·areaDownRowYAsm(SB), NOSPLIT, $0-56
	MOVQ dstPtr+0(FP), DI
	MOVQ srcPtr+8(FP), SI
	MOVQ srcStride+16(FP), R8
	MOVQ sy0+24(FP), R9
	MOVQ sy1+32(FP), R10
	MOVQ x0+40(FP), R11
	MOVQ x1+48(FP), R12

	XORQ R13, R13
	XORQ R14, R14

syloop:
	CMPQ R9, R10
	JGE syend
	MOVQ R9, AX
	IMULQ R8, AX
	ADDQ SI, AX
	MOVQ R11, BX
xloop:
	CMPQ BX, R12
	JGE xend
	MOVBQZX (AX)(BX*1), DX
	ADDQ DX, R13
	INCQ R14
	INCQ BX
	JMP xloop
xend:
	INCQ R9
	JMP syloop
syend:
	TESTQ R14, R14
	JNZ havediv
	MOVQ $1, R14
havediv:
	MOVQ R13, AX
	MOVQ R14, BX
	XORQ DX, DX
	DIVQ BX
	MOVB AL, (DI)
	RET
