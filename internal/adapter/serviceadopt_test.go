package adapter

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// serviceName may prefix or truncate, which is what made the rollback wrong: it
// deleted opts.Name while the Service had been created under serviceName(...).
func TestServiceName_PrefixesLeadingDigit(t *testing.T) {
	if got := serviceName("7up"); got != "svc-7up" {
		t.Errorf("expected svc-7up, got %q", got)
	}
	if got := serviceName("web"); got != "web" {
		t.Errorf("expected web unchanged, got %q", got)
	}
}

// The wedge this fixes: a Service left over from an earlier failed create made
// the container name unusable forever, because the Service is d2k's own object
// and a Docker client cannot see or delete it.
func TestCreateContainer_AdoptsOrphanedService(t *testing.T) {
	const name = "floci-opensearch-poc-os"

	orphan := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName(name),
			Namespace: "test",
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "10.96.0.42",
			Ports:     []corev1.ServicePort{{Name: "stale", Port: 1234}},
		},
	}

	a := &KubernetesDockerAdapter{
		client:    fake.NewSimpleClientset(orphan),
		namespace: "test",
	}

	_, _, err := a.CreateContainer(context.Background(), RunOptions{
		Name:         name,
		Image:        "opensearchproject/opensearch:2.11.1",
		ExposedPorts: map[string]struct{}{"9200/tcp": {}},
		PortBindings: []string{"9400:9200"},
	})
	if err != nil {
		t.Fatalf("create should adopt the orphaned Service, got: %v", err)
	}

	// The Service must now describe the new container, not the stale one.
	svc, getErr := a.client.CoreV1().Services("test").Get(
		context.Background(), serviceName(name), metav1.GetOptions{})
	if getErr != nil {
		t.Fatalf("Service should still exist: %v", getErr)
	}
	for _, p := range svc.Spec.Ports {
		if p.Name == "stale" {
			t.Error("Service still carries the stale port; it was not updated")
		}
	}

	// ClusterIP is immutable, so adoption must carry the assigned one over.
	if svc.Spec.ClusterIP != "10.96.0.42" {
		t.Errorf("expected the existing ClusterIP to be preserved, got %q", svc.Spec.ClusterIP)
	}

	// And the Deployment must exist, so the container is genuinely usable.
	if _, err := a.client.AppsV1().Deployments("test").Get(
		context.Background(), name, metav1.GetOptions{}); err != nil {
		t.Errorf("expected the Deployment to be created: %v", err)
	}
}

// The ordinary path must keep working: no pre-existing Service, create succeeds.
func TestCreateContainer_CreatesServiceWhenAbsent(t *testing.T) {
	a := &KubernetesDockerAdapter{
		client:    fake.NewSimpleClientset(),
		namespace: "test",
	}

	_, _, err := a.CreateContainer(context.Background(), RunOptions{
		Name:         "web",
		Image:        "nginx:alpine",
		ExposedPorts: map[string]struct{}{"80/tcp": {}},
		PortBindings: []string{"8080:80"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := a.client.CoreV1().Services("test").Get(
		context.Background(), serviceName("web"), metav1.GetOptions{}); err != nil {
		t.Errorf("expected a Service to be created: %v", err)
	}
}
