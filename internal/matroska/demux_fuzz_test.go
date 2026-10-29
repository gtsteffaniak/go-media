package matroska

import (
	"bytes"
	"testing"
)

// FuzzClusterWalk feeds arbitrary bytes to the synchronous cluster/block
// demuxer, the Cues picker, and the lacing splitter. None may panic, loop
// forever, or return frames outside the input.
func FuzzClusterWalk(f *testing.F) {
	jp := []byte{0xFF, 0xD8, 0xFF, 0xD9}
	f.Add(buildMJPEGMKV(f, mkvOpts{clusters: 2, framesPer: 2, frameStepMs: 100, cues: true, jpegForFrame: func(int) []byte { return jp }}))
	f.Add(ebmlElem(idClusterTest, u64Elem(idTimecodeTest, 5), simpleBlock(1, 0, true, []byte{1, 2}), simpleBlock(9, 0, false, nil)))
	f.Add(ebmlUnknown(idClusterTest, u64Elem(idTimecodeTest, 5), simpleBlock(1, -3, true, []byte{0x02, 1, 1, 9, 9, 9})))
	f.Add(ebmlElem(idClusterTest, ebmlElem(idBlockGroup, ebmlElem(idBlock, []byte{0x81, 0, 0, 0x06, 0x02, 0x83, 0xBF, 1, 2, 3, 4, 5, 6}), u64Elem(idReferenceBlock, 1))))
	f.Fuzz(func(t *testing.T, data []byte) {
		r := bytes.NewReader(data)
		n := int64(len(data))
		w := clusterWalker{r: r, segEnd: n, off: 0}
		for i := 0; ; i++ {
			if i > len(data)+1 {
				t.Fatal("walker did not make progress")
			}
			bi, err := w.next()
			if err != nil {
				break
			}
			if bi.payloadOff < 0 || bi.payloadOff+bi.payloadLen > n {
				t.Fatalf("payload [%d,+%d) outside %d", bi.payloadOff, bi.payloadLen, n)
			}
			frames, err := readFrames(r, bi)
			if err != nil {
				continue
			}
			var sum int
			for _, fr := range frames {
				sum += len(fr)
			}
			if int64(sum) > bi.payloadLen {
				t.Fatalf("frames total %d > payload %d", sum, bi.payloadLen)
			}
		}
		_, _ = pickCueCluster(data, 1, 1000)
		_, _ = parseSeekHeadCues(data)
		for lacing := byte(0); lacing < 4; lacing++ {
			_, _ = splitLaces(data, lacing)
		}
		if ix, err := indexSegment(r, n); err == nil {
			_, _ = collectSeekFrames(r, ix, 1, 1_000_000, 500, true)
			_, _ = collectSeekFrames(r, ix, 1, 1_000_000, 500, false)
		}
	})
}
