package adapter

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func pod(name, deployment, ip string, phase corev1.PodPhase) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test",
			Labels:    map[string]string{"app": deployment},
		},
		Status: corev1.PodStatus{PodIP: ip, Phase: phase},
	}
}

// The case the bug was about: a container with no published ports has no
// Service, so an address derived only from LoadBalancer ingress is empty. The
// pod IP is routable in-namespace and must be reported instead.
func TestPodIP_ReturnsRunningPodIP(t *testing.T) {
	a := &KubernetesDockerAdapter{
		client:    fake.NewSimpleClientset(pod("web-abc", "web", "10.1.2.3", corev1.PodRunning)),
		namespace: "test",
	}

	if got := a.podIP(context.Background(), "web"); got != "10.1.2.3" {
		t.Errorf("expected 10.1.2.3, got %q", got)
	}
}

// A running pod wins over one that merely has an address, so callers get an
// endpoint that can actually accept a connection.
func TestPodIP_PrefersRunningOverPending(t *testing.T) {
	a := &KubernetesDockerAdapter{
		client: fake.NewSimpleClientset(
			pod("web-pending", "web", "10.1.2.4", corev1.PodPending),
			pod("web-running", "web", "10.1.2.5", corev1.PodRunning),
		),
		namespace: "test",
	}

	if got := a.podIP(context.Background(), "web"); got != "10.1.2.5" {
		t.Errorf("expected the running pod's IP 10.1.2.5, got %q", got)
	}
}

// Something is better than nothing while a pod is still starting.
func TestPodIP_FallsBackToPendingWithIP(t *testing.T) {
	a := &KubernetesDockerAdapter{
		client:    fake.NewSimpleClientset(pod("web-pending", "web", "10.1.2.6", corev1.PodPending)),
		namespace: "test",
	}

	if got := a.podIP(context.Background(), "web"); got != "10.1.2.6" {
		t.Errorf("expected the pending pod's IP 10.1.2.6, got %q", got)
	}
}

// Pods of a different Deployment must not be reported: returning a neighbour's
// address would be worse than returning none.
func TestPodIP_IgnoresOtherDeployments(t *testing.T) {
	a := &KubernetesDockerAdapter{
		client:    fake.NewSimpleClientset(pod("other-abc", "other", "10.9.9.9", corev1.PodRunning)),
		namespace: "test",
	}

	if got := a.podIP(context.Background(), "web"); got != "" {
		t.Errorf("expected no IP for an unrelated deployment, got %q", got)
	}
}

// A pod with no address yet is not an address.
func TestPodIP_EmptyWhenNoIPAssigned(t *testing.T) {
	a := &KubernetesDockerAdapter{
		client:    fake.NewSimpleClientset(pod("web-abc", "web", "", corev1.PodPending)),
		namespace: "test",
	}

	if got := a.podIP(context.Background(), "web"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}
