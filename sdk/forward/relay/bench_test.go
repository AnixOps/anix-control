package relay

import (
	"context"
	"io"
	"testing"
)

// Stubs for the benchmark plan of anixops-protocol.md section 7.2 (A5 owns
// the real ones): a single stream's throughput over loopback TCP, and the
// cost of a stream on a warm carrier.

func BenchmarkStreamThroughput(b *testing.B) {
	d, a := newPair(b, "tcp", Config{}, Config{})
	s, err := d.Open(testParams())
	if err != nil {
		b.Fatal(err)
	}
	in, err := a.Accept(context.Background())
	if err != nil {
		b.Fatal(err)
	}
	go func() { _, _ = io.Copy(io.Discard, in) }()
	buf := make([]byte, 64*1024)
	b.SetBytes(int64(len(buf)))
	b.ResetTimer()
	for range b.N {
		if _, err := s.Write(buf); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	_ = s.CloseWrite()
}

func BenchmarkOpenAnswerClose(b *testing.B) {
	d, a := newPair(b, "tcp", Config{}, Config{})
	go func() {
		for {
			in, err := a.Accept(context.Background())
			if err != nil {
				return
			}
			_ = in.Accept()
			_ = in.CloseWrite()
			go func() { _, _ = io.Copy(io.Discard, in); _ = in.Close() }()
		}
	}()
	b.ResetTimer()
	for range b.N {
		s, err := d.Open(testParams())
		if err != nil {
			b.Fatal(err)
		}
		if err := s.AwaitResult(context.Background()); err != nil {
			b.Fatal(err)
		}
		_ = s.CloseWrite()
		_, _ = io.Copy(io.Discard, s)
		_ = s.Close()
	}
}
