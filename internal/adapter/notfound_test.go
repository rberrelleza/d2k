package adapter

import (
	"context"
	"errors"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// resolveDeploymentName must report a miss with ErrContainerNotFound, so the API
// layer can turn it into a 404 rather than a blanket 500.
func TestResolveDeploymentName_UnknownIsErrContainerNotFound(t *testing.T) {
	a := &KubernetesDockerAdapter{
		client:    fake.NewSimpleClientset(),
		namespace: "test",
	}

	_, err := a.resolveDeploymentName(context.Background(), "does-not-exist")
	if err == nil {
		t.Fatal("expected an error for an unknown container")
	}
	if !errors.Is(err, ErrContainerNotFound) {
		t.Errorf("expected ErrContainerNotFound, got %v", err)
	}
}

// A container that does exist must resolve cleanly, so the sentinel cannot be
// returned for the happy path.
func TestResolveDeploymentName_KnownResolves(t *testing.T) {
	a := &KubernetesDockerAdapter{
		client: fake.NewSimpleClientset(&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "test"},
		}),
		namespace: "test",
	}

	got, err := a.resolveDeploymentName(context.Background(), "web")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "web" {
		t.Errorf("expected %q, got %q", "web", got)
	}
}
