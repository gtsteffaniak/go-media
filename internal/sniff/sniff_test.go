package sniff

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/format"
)

// ebmlFile builds an EBML header with the given DocType and padding bytes in
// its body, followed by a Segment start. Sizes use 8-byte vints.
func ebmlFile(docType string, padding int) []byte {
	doc := append([]byte{0x42, 0x82}, vint8(uint64(len(docType)))...)
	doc = append(doc, docType...)
	body := append(doc, make([]byte, padding)...)
	out := []byte{0x1A, 0x45, 0xDF, 0xA3}
	out = append(out, vint8(uint64(len(body)))...)
	out = append(out, body...)
	// A Segment follows, so the file looks like a real container.
	return append(out, 0x18, 0x53, 0x80, 0x67, 0x01, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF)
}

// vint8 encodes v as an 8-byte EBML size vint.
func vint8(v uint64) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], v)
	return append([]byte{0x01}, b[1:]...)
}

func detect(t *testing.T, data []byte) format.ID {
	t.Helper()
	id, err := Detect(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	return id
}

func TestDetectEBMLDocType(t *testing.T) {
	if got := detect(t, ebmlFile("webm", 0)); got != format.WebM {
		t.Errorf("webm doctype = %v, want WebM", got)
	}
	if got := detect(t, ebmlFile("matroska", 0)); got != format.MKV {
		t.Errorf("matroska doctype = %v, want MKV", got)
	}
}

// A header larger than 4 KB is not scanned. It is treated as Matroska, and the
// thumbnail path decides from the bounded track header.
func TestDetectOversizedEBMLHeaderIsMKV(t *testing.T) {
	data := ebmlFile("webm", 8<<10)
	if got := detect(t, data); got != format.MKV {
		t.Errorf("oversized header = %v, want MKV", got)
	}
}

// The header read must not scan past the element. A "webm" string after the
// header must not change the result.
func TestDetectDoesNotScanPastHeader(t *testing.T) {
	data := append(ebmlFile("matroska", 0), []byte("webm webm webm")...)
	if got := detect(t, data); got != format.MKV {
		t.Errorf("doctype matroska with trailing webm text = %v, want MKV", got)
	}
}

func TestEBMLHeaderBytesRequiresMagic(t *testing.T) {
	data := ebmlFile("webm", 0)
	data[0] = 0x00
	if _, ok := ebmlHeaderBytes(bytes.NewReader(data), int64(len(data))); ok {
		t.Error("header with wrong magic was accepted")
	}
}

func TestEBMLHeaderBytesTruncated(t *testing.T) {
	data := ebmlFile("webm", 0)[:6]
	if _, ok := ebmlHeaderBytes(bytes.NewReader(data), int64(len(data))); ok {
		t.Error("truncated header was accepted")
	}
}
