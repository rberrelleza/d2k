package adapter

import (
	"bytes"
	"io"
	"testing"
)

// The bug: StreamOptions.Stdin was always the hijacked connection, even when the
// exec had not attached stdin. client-go then opens a stdin stream and copies
// from a reader that never reaches EOF, because the client is sitting there
// waiting for output. Every such exec blocked until a timeout:
//
//	POST /exec/<id>/start  status 200  duration 30.17s
//
// and a client with a shorter deadline just saw its socket close.
func TestExecStdin_NilWhenNotAttached(t *testing.T) {
	reader := bytes.NewBufferString("never read")

	if got := execStdin(ExecOptions{AttachStdin: false}, reader); got != nil {
		t.Error("stdin must be nil when the exec did not attach it, otherwise the stream blocks until timeout")
	}
}

// An interactive exec still needs its stdin, so the fix must not break
// docker exec -i.
func TestExecStdin_PassedThroughWhenAttached(t *testing.T) {
	reader := bytes.NewBufferString("input")

	got := execStdin(ExecOptions{AttachStdin: true}, reader)
	if got == nil {
		t.Fatal("stdin must be attached when the exec asked for it")
	}

	data, err := io.ReadAll(got)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "input" {
		t.Errorf("expected the caller's reader, got %q", string(data))
	}
}

// A nil reader stays nil rather than becoming a non-nil interface holding nil,
// which would still make client-go open the stream.
func TestExecStdin_NilReaderStaysNil(t *testing.T) {
	if got := execStdin(ExecOptions{AttachStdin: false}, nil); got != nil {
		t.Error("expected nil")
	}
}
