package router

import "testing"

// The bug: the stripper treated any path beginning with "/v" as version
// prefixed, so /volumes/create was rewritten to /create and never matched its
// route. Every /volumes/* endpoint was unreachable, which makes the documented
// docker volume to PersistentVolumeClaim translation unusable.
func TestIsAPIVersion(t *testing.T) {
	cases := []struct {
		segment string
		want    bool
		why     string
	}{
		{"v1.41", true, "the usual Docker CLI form"},
		{"v1", true, "major only"},
		{"v1.2.3", true, "extra components are still a version"},
		{"v0", true, "zero is a number"},

		{"volumes", false, "the bug: a collection whose name starts with v"},
		{"version", false, "a real endpoint, not a version"},
		{"v", false, "bare v carries no version"},
		{"volumes2", false, "digits later in the word do not make it a version"},
		{"containers", false, "unrelated collection"},
		{"", false, "empty segment"},
		{"1.41", false, "no v prefix"},
		{"va.b", false, "letters after v are not a version"},
	}

	for _, c := range cases {
		if got := isAPIVersion(c.segment); got != c.want {
			t.Errorf("isAPIVersion(%q) = %v, want %v (%s)", c.segment, got, c.want, c.why)
		}
	}
}

// Guard the specific regression: these are the paths that broke, and the first
// segment of each must survive routing.
func TestIsAPIVersion_VolumeRoutesSurvive(t *testing.T) {
	for _, segment := range []string{"volumes"} {
		if isAPIVersion(segment) {
			t.Errorf("%q would be stripped as a version, making /%s/... unreachable", segment, segment)
		}
	}
}
