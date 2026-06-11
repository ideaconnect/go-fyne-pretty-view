package parse

import (
	"bytes"
	"testing"
)

func makeBufBench(n int) []byte {
	b := make([]byte, 0, n)
	b = append(b, []byte("<root>")...)
	filler := bytes.Repeat([]byte("abcdefgh "), 1024)
	for len(b) < n-7 {
		b = append(b, filler...)
	}
	b = append(b, []byte("</root>")...)
	return b
}

func makeBufNoCloseBench(n int) []byte {
	b := make([]byte, 0, n)
	b = append(b, []byte("<root ")...)
	filler := bytes.Repeat([]byte("abcdefgh "), 1024)
	for len(b) < n {
		b = append(b, filler...)
	}
	return b
}

func BenchmarkXMLDetect8MB(b *testing.B) {
	buf := makeBufBench(8 << 20)
	p := xmlParser{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p.Detect(buf)
	}
}

func BenchmarkXMLDetectNoClose8MB(b *testing.B) {
	buf := makeBufNoCloseBench(8 << 20)
	p := xmlParser{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p.Detect(buf)
	}
}

func BenchmarkHTMLDetect8MB(b *testing.B) {
	buf := makeBufBench(8 << 20)
	p := htmlParser{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p.Detect(buf)
	}
}
