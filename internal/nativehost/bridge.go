package nativehost

import (
	"context"
	"io"
	"net"
	"sync"
	"time"
)

const (
	dialTimeout = 2 * time.Second
	retryMin    = 100 * time.Millisecond
	retryMax    = time.Second
)

type stdoutGate struct {
	mu sync.Mutex
	w  io.Writer
}

func (g *stdoutGate) Write(p []byte) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.w.Write(p)
}

func (g *stdoutGate) writeGone() {
	g.mu.Lock()
	defer g.mu.Unlock()
	writeProviderGone(g.w)
}

// Bridge copies Chrome stdio to the GUI Unix socket and stays running if the GUI restarts.
// It returns when stdin closes (the extension dropped native messaging).
func Bridge(socketPath string, stdin io.Reader, stdout io.Writer) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	var sock net.Conn
	out := &stdoutGate{w: stdout}

	setSock := func(c net.Conn) {
		mu.Lock()
		sock = c
		mu.Unlock()
	}
	closeSock := func() {
		mu.Lock()
		c := sock
		sock = nil
		mu.Unlock()
		if c != nil {
			_ = c.Close()
		}
	}
	writeSock := func(p []byte) {
		mu.Lock()
		c := sock
		mu.Unlock()
		if c == nil {
			return
		}
		if _, err := c.Write(p); err != nil {
			_ = c.Close()
		}
	}

	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := stdin.Read(buf)
			if n > 0 {
				writeSock(buf[:n])
			}
			if err != nil {
				cancel()
				closeSock()
				return
			}
		}
	}()

	delay := time.Duration(0)
	for ctx.Err() == nil {
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil
			case <-timer.C:
			}
		}
		conn, err := net.DialTimeout("unix", socketPath, dialTimeout)
		if err != nil {
			if delay == 0 {
				delay = retryMin
			} else {
				delay *= 2
				if delay > retryMax {
					delay = retryMax
				}
			}
			continue
		}
		delay = 0
		setSock(conn)
		_, _ = io.Copy(out, conn)
		closeSock()
		if ctx.Err() != nil {
			return nil
		}
		out.writeGone()
	}
	return nil
}
