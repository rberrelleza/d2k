package exec

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The bug: the status line was chosen by TTY, so a non-TTY exec got 200 even
// though the client had asked to upgrade. The Docker CLI itself fails on that:
//
//	$ docker exec <container> echo hi
//	unable to upgrade to tcp, received 200
//
// Status depends on the upgrade request; the content type describes the framing.
func TestExecResponseHeader(t *testing.T) {
	cases := []struct {
		name        string
		upgrade     bool
		tty         bool
		wantStatus  string
		wantType    string
		wantUpgrade bool
	}{
		{
			name:       "non-TTY exec asking to upgrade, the case that was broken",
			upgrade:    true,
			tty:        false,
			wantStatus: "HTTP/1.1 101 UPGRADED",
			wantType:   "application/vnd.docker.multiplexed-stream",
			// the client took the connection over, so the upgrade headers belong
			wantUpgrade: true,
		},
		{
			name:        "TTY exec asking to upgrade",
			upgrade:     true,
			tty:         true,
			wantStatus:  "HTTP/1.1 101 UPGRADED",
			wantType:    "application/vnd.docker.raw-stream",
			wantUpgrade: true,
		},
		{
			name:        "non-TTY exec without an upgrade request",
			upgrade:     false,
			tty:         false,
			wantStatus:  "HTTP/1.1 200 OK",
			wantType:    "application/vnd.docker.multiplexed-stream",
			wantUpgrade: false,
		},
		{
			name:        "TTY exec without an upgrade request",
			upgrade:     false,
			tty:         true,
			wantStatus:  "HTTP/1.1 200 OK",
			wantType:    "application/vnd.docker.raw-stream",
			wantUpgrade: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := execResponseHeader(c.upgrade, c.tty)

			if !strings.HasPrefix(got, c.wantStatus) {
				t.Errorf("expected status %q, got %q", c.wantStatus, firstLine(got))
			}
			if !strings.Contains(got, "Content-Type: "+c.wantType) {
				t.Errorf("expected content type %q in %q", c.wantType, got)
			}
			if strings.Contains(got, "Upgrade: tcp") != c.wantUpgrade {
				t.Errorf("upgrade headers present = %v, want %v", !c.wantUpgrade, c.wantUpgrade)
			}
			if !strings.HasSuffix(got, "\r\n\r\n") {
				t.Errorf("headers must end with a blank line, got %q", got)
			}
		})
	}
}

func firstLine(s string) string {
	if i := strings.Index(s, "\r\n"); i >= 0 {
		return s[:i]
	}
	return s
}

// The header is matched case-insensitively, since HTTP header values from real
// clients vary.
func TestWantsUpgrade(t *testing.T) {
	cases := []struct {
		header string
		want   bool
	}{
		{"tcp", true},
		{"TCP", true},
		{"Tcp", true},
		{"", false},
		{"websocket", false},
	}

	for _, c := range cases {
		r := httptest.NewRequest(http.MethodPost, "/exec/abc/start", nil)
		if c.header != "" {
			r.Header.Set("Upgrade", c.header)
		}
		if got := wantsUpgrade(r); got != c.want {
			t.Errorf("Upgrade: %q -> %v, want %v", c.header, got, c.want)
		}
	}
}
