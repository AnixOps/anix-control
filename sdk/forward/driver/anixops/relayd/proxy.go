package relayd

import (
	"errors"
	"io"
	"net"
	"sync"
)

// duplex is one side of a proxied TCP connection: a net.Conn, or a stream of
// a carrier, with half-close.
type duplex interface {
	io.ReadWriteCloser
	CloseWrite() error
}

// connDuplex gives a net.Conn the half-close it has, or none.
type connDuplex struct{ net.Conn }

func (c connDuplex) CloseWrite() error {
	if cw, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return cw.CloseWrite()
	}
	return nil
}

// errQuota ends a connection whose hop used up its quota.
var errQuota = errors.New("relayd: quota used up")

type direction uint8

const (
	dirUp   direction = iota // client to target
	dirDown                  // target to client
)

// copyBuffer is the size of one read; it is also how far past quota_bytes a
// hop's traffic can run (within one copy buffer per direction).
const copyBuffer = 32 << 10

var bufPool = sync.Pool{New: func() any { b := make([]byte, copyBuffer); return &b }}

// pipe moves a connection's bytes both ways until both directions have ended
// (half-close is kept: a direction that reached EOF closes its destination's
// write side and the other direction runs on), or one of them fails, which
// ends the connection at once.
func (h *hop) pipe(client, out duplex) {
	done := make(chan struct{})
	var once sync.Once
	abort := func() {
		once.Do(func() {
			close(done)
			_ = client.Close()
			_ = out.Close()
		})
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if err := h.copyDir(out, client, dirUp, done); err != nil {
			abort()
		} else {
			_ = out.CloseWrite()
		}
	}()
	go func() {
		defer wg.Done()
		if err := h.copyDir(client, out, dirDown, done); err != nil {
			abort()
		} else {
			_ = client.CloseWrite()
		}
	}()
	wg.Wait()
	abort()
}

// copyDir copies src to dst, counting what it moves, holding the hop's
// bandwidth and quota. It returns nil on a clean EOF.
func (h *hop) copyDir(dst io.Writer, src io.Reader, dir direction, done <-chan struct{}) error {
	bp := bufPool.Get().(*[]byte)
	defer bufPool.Put(bp)
	buf := *bp
	for {
		n, rerr := src.Read(buf)
		if n > 0 {
			if h.quotaUsed() {
				return errQuota
			}
			h.limiter(dir).wait(done, n)
			select {
			case <-done:
				return net.ErrClosed
			default:
			}
			w, werr := dst.Write(buf[:n])
			h.add(dir, uint64(w), 0) // #nosec G115 -- w is a byte count
			if werr != nil {
				return werr
			}
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				return nil
			}
			return rerr
		}
	}
}
