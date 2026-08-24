// Package dockerstream frames output in Docker's multiplexed stream format.
//
// When an exec or attach is created without a TTY, the Docker Engine API
// responds with Content-Type application/vnd.docker.multiplexed-stream and
// every payload carries an 8-byte header:
//
//	byte 0    stream type (0=stdin, 1=stdout, 2=stderr)
//	bytes 1-3 zero padding
//	bytes 4-7 payload length, big-endian uint32
//
// Clients that follow the API (the Docker SDKs, docker-java) parse those frames
// strictly. Writing raw bytes on a connection advertised as multiplexed makes
// them misread the length prefix and abort the stream, which surfaces as a
// closed socket rather than a useful error.
package dockerstream

import (
	"io"
	"sync"
)

// Muxer serialises stdout and stderr onto one connection as Docker frames.
//
// Both streams share the underlying writer, so a mutex is required: a frame
// header and its payload must not be split by a write on the other stream.
type Muxer struct {
	mu sync.Mutex
	w  io.Writer
}

// flusher matches buffered writers such as *bufio.ReadWriter, which a hijacked
// connection hands back. Without an explicit flush the payload can sit in the
// buffer while the client waits for it.
type flusher interface {
	Flush() error
}

// NewMuxer returns a Muxer writing Docker frames to w.
func NewMuxer(w io.Writer) *Muxer {
	return &Muxer{w: w}
}

// Stdout returns a writer that frames payloads as stream 1.
func (m *Muxer) Stdout() io.Writer { return &streamWriter{mux: m, stream: 1} }

// Stderr returns a writer that frames payloads as stream 2.
func (m *Muxer) Stderr() io.Writer { return &streamWriter{mux: m, stream: 2} }

func (m *Muxer) write(stream byte, p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var header [8]byte
	header[0] = stream
	l := uint32(len(p))
	header[4] = byte(l >> 24)
	header[5] = byte(l >> 16)
	header[6] = byte(l >> 8)
	header[7] = byte(l)

	if _, err := m.w.Write(header[:]); err != nil {
		return 0, err
	}
	n, err := m.w.Write(p)
	if err != nil {
		return n, err
	}
	if f, ok := m.w.(flusher); ok {
		if ferr := f.Flush(); ferr != nil {
			return n, ferr
		}
	}
	return n, nil
}

type streamWriter struct {
	mux    *Muxer
	stream byte
}

func (s *streamWriter) Write(p []byte) (int, error) {
	return s.mux.write(s.stream, p)
}
