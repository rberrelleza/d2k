package dockerstream

import (
	"bytes"
	"encoding/binary"
	"sync"
	"testing"
)

// frame is one decoded Docker multiplexed-stream frame.
type frame struct {
	stream  byte
	payload string
}

// decode parses the 8-byte-header stream a Docker client would parse, and fails
// the test on anything malformed, which is what a real client does by dropping
// the connection.
func decode(t *testing.T, b []byte) []frame {
	t.Helper()
	var out []frame
	for len(b) > 0 {
		if len(b) < 8 {
			t.Fatalf("truncated header: %d bytes left", len(b))
		}
		stream := b[0]
		length := binary.BigEndian.Uint32(b[4:8])
		b = b[8:]
		if uint32(len(b)) < length {
			t.Fatalf("frame claims %d bytes, only %d present", length, len(b))
		}
		out = append(out, frame{stream: stream, payload: string(b[:length])})
		b = b[length:]
	}
	return out
}

// Without framing, a client reading a connection advertised as multiplexed
// misreads the payload as a length prefix and abandons the stream.
func TestMuxer_FramesStdout(t *testing.T) {
	var buf bytes.Buffer
	mux := NewMuxer(&buf)

	if _, err := mux.Stdout().Write([]byte("hello")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	frames := decode(t, buf.Bytes())
	if len(frames) != 1 {
		t.Fatalf("expected 1 frame, got %d", len(frames))
	}
	if frames[0].stream != 1 {
		t.Errorf("expected stream 1 (stdout), got %d", frames[0].stream)
	}
	if frames[0].payload != "hello" {
		t.Errorf("expected payload %q, got %q", "hello", frames[0].payload)
	}
}

// stderr must be distinguishable, otherwise a client cannot separate the two.
func TestMuxer_FramesStderrSeparately(t *testing.T) {
	var buf bytes.Buffer
	mux := NewMuxer(&buf)

	mux.Stdout().Write([]byte("out"))
	mux.Stderr().Write([]byte("err"))

	frames := decode(t, buf.Bytes())
	if len(frames) != 2 {
		t.Fatalf("expected 2 frames, got %d", len(frames))
	}
	if frames[0].stream != 1 || frames[0].payload != "out" {
		t.Errorf("first frame wrong: stream=%d payload=%q", frames[0].stream, frames[0].payload)
	}
	if frames[1].stream != 2 || frames[1].payload != "err" {
		t.Errorf("second frame wrong: stream=%d payload=%q", frames[1].stream, frames[1].payload)
	}
}

// The length prefix must be big-endian, as the API specifies. A byte-order slip
// would still "work" for tiny payloads, so check one that spans two bytes.
func TestMuxer_LengthIsBigEndian(t *testing.T) {
	var buf bytes.Buffer
	payload := bytes.Repeat([]byte("x"), 300)

	NewMuxer(&buf).Stdout().Write(payload)

	raw := buf.Bytes()
	if got := binary.BigEndian.Uint32(raw[4:8]); got != 300 {
		t.Errorf("expected length 300, got %d", got)
	}
	if raw[1] != 0 || raw[2] != 0 || raw[3] != 0 {
		t.Errorf("bytes 1-3 must be zero padding, got % x", raw[1:4])
	}
}

// An empty write must not emit a zero-length frame, which would be noise on the
// wire for no payload.
func TestMuxer_EmptyWriteEmitsNothing(t *testing.T) {
	var buf bytes.Buffer

	n, err := NewMuxer(&buf).Stdout().Write(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 bytes written, got %d", n)
	}
	if buf.Len() != 0 {
		t.Errorf("expected nothing on the wire, got %d bytes", buf.Len())
	}
}

// lockedBuffer is a writer that is safe for the concurrency check below, so the
// test measures the Muxer's locking rather than racing on the buffer itself.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

// stdout and stderr share one connection, so a header must never be split by a
// write on the other stream. Run with -race to make this meaningful.
func TestMuxer_ConcurrentStreamsProduceIntactFrames(t *testing.T) {
	lb := &lockedBuffer{}
	mux := NewMuxer(lb)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); mux.Stdout().Write([]byte("aaaa")) }()
		go func() { defer wg.Done(); mux.Stderr().Write([]byte("bbbb")) }()
	}
	wg.Wait()

	frames := decode(t, lb.buf.Bytes())
	if len(frames) != 100 {
		t.Fatalf("expected 100 intact frames, got %d", len(frames))
	}
	for _, f := range frames {
		switch {
		case f.stream == 1 && f.payload == "aaaa":
		case f.stream == 2 && f.payload == "bbbb":
		default:
			t.Fatalf("interleaved frame: stream=%d payload=%q", f.stream, f.payload)
		}
	}
}

// flushWriter records whether the buffered writer was flushed, which a hijacked
// connection needs or the payload sits in the buffer while the client waits.
type flushWriter struct {
	bytes.Buffer
	flushed bool
}

func (f *flushWriter) Flush() error {
	f.flushed = true
	return nil
}

func TestMuxer_FlushesBufferedWriter(t *testing.T) {
	fw := &flushWriter{}

	NewMuxer(fw).Stdout().Write([]byte("data"))

	if !fw.flushed {
		t.Error("expected the writer to be flushed after a frame")
	}
}
