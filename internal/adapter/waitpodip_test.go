package adapter

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func testPod(name, deployment, ip string, phase corev1.PodPhase) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test",
			Labels:    map[string]string{"app": deployment},
		},
		Status: corev1.PodStatus{PodIP: ip, Phase: phase},
	}
}

// An address that is already there must be returned without waiting, so the
// common case stays fast.
func TestWaitForPodIP_ReturnsImmediatelyWhenReady(t *testing.T) {
	a := &KubernetesDockerAdapter{
		client:    fake.NewSimpleClientset(testPod("web-abc", "web", "10.1.2.3", corev1.PodRunning)),
		namespace: "test",
	}

	start := time.Now()
	got := a.waitForPodIP(context.Background(), "web", 5*time.Second)
	elapsed := time.Since(start)

	if got != "10.1.2.3" {
		t.Errorf("expected 10.1.2.3, got %q", got)
	}
	if elapsed > time.Second {
		t.Errorf("expected an immediate return, took %s", elapsed)
	}
}

// A pod that never gets an address must not hang the caller forever: the wait is
// bounded and best effort, so start degrades rather than failing.
func TestWaitForPodIP_GivesUpAtTimeout(t *testing.T) {
	a := &KubernetesDockerAdapter{
		client:    fake.NewSimpleClientset(),
		namespace: "test",
	}

	start := time.Now()
	got := a.waitForPodIP(context.Background(), "web", 1200*time.Millisecond)
	elapsed := time.Since(start)

	if got != "" {
		t.Errorf("expected no address, got %q", got)
	}
	if elapsed < time.Second {
		t.Errorf("returned before the timeout elapsed, after %s", elapsed)
	}
	if elapsed > 5*time.Second {
		t.Errorf("overshot the timeout badly, took %s", elapsed)
	}
}

// A cancelled context must abort the wait promptly, so a client that hangs up
// does not leave the server polling.
func TestWaitForPodIP_StopsOnContextCancel(t *testing.T) {
	a := &KubernetesDockerAdapter{
		client:    fake.NewSimpleClientset(),
		namespace: "test",
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(300 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	got := a.waitForPodIP(ctx, "web", 30*time.Second)
	elapsed := time.Since(start)

	if got != "" {
		t.Errorf("expected no address, got %q", got)
	}
	if elapsed > 5*time.Second {
		t.Errorf("did not stop on cancellation, took %s", elapsed)
	}
}
