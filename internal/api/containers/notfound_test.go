package containers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/portainer/d2k/internal/adapter"
)

// A missing container must map to 404 with Docker's body. Clients rely on the
// distinction: tooling that stops or removes a name before reusing it treats 404
// as "nothing to do" and 500 as a hard failure.
func TestWriteContainerError_NotFoundIs404(t *testing.T) {
	rec := httptest.NewRecorder()

	// Wrapped, as the adapter returns it, so errors.Is has to unwrap.
	err := fmt.Errorf("container %q: %w", "missing", adapter.ErrContainerNotFound)
	writeContainerError(rec, "missing", err)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if want := "No such container: missing"; body["message"] != want {
		t.Errorf("expected message %q, got %q", want, body["message"])
	}
}

// Anything else must stay a 500, otherwise a broken backend looks like a missing
// container and clients silently carry on.
func TestWriteContainerError_OtherIs500(t *testing.T) {
	rec := httptest.NewRecorder()

	writeContainerError(rec, "boom", errors.New("apiserver unreachable"))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if body["message"] != "apiserver unreachable" {
		t.Errorf("expected the underlying error, got %q", body["message"])
	}
}

// The sentinel must survive wrapping at any depth, since callers add context.
func TestWriteContainerError_DeeplyWrapped(t *testing.T) {
	rec := httptest.NewRecorder()

	err := fmt.Errorf("stopping: %w",
		fmt.Errorf("container %q: %w", "deep", adapter.ErrContainerNotFound))
	writeContainerError(rec, "deep", err)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 through two layers of wrapping, got %d", rec.Code)
	}
}
