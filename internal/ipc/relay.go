package ipc

import (
	"io"
)

// Relay copies stdin to sock and sock to stdout until either side closes.
func Relay(sock io.ReadWriteCloser, stdin io.Reader, stdout io.Writer) error {
	errc := make(chan error, 2)
	go func() {
		_, err := io.Copy(sock, stdin)
		errc <- err
	}()
	go func() {
		_, err := io.Copy(stdout, sock)
		errc <- err
	}()
	err := <-errc
	// Shutting down the copy; the other side may already be closed.
	_ = sock.Close()
	<-errc
	if err == io.EOF {
		return nil
	}
	return err
}
