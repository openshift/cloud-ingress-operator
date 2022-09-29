package routerservice

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// TestRouterServiceController runs ReconcileRouterService.Reconcile() against a
// fake client that tracks a Service object.
func TestRouterServiceController(t *testing.T) {
	// Set the logger to development mode for verbose logs.
	logf.SetLogger(zap.New())

	var (
		name      = "router-default"
		namespace = "openshift-ingress"
	)

	// router-default service
	routerDefaultSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer,
		},
	}

	// Register operator types with the runtime scheme.
	s := scheme.Scheme

	// Create a fake client to mock API calls.
	cl := fake.
		NewClientBuilder().
		WithScheme(s).
		WithObjects(routerDefaultSvc).
		Build()

	s.AddKnownTypes(corev1.SchemeGroupVersion, routerDefaultSvc)

	log.Info("Creating ReconcileRouterService")
	// Create a ReconcileRouterService object with the scheme and fake client.
	r := &ReconcileRouterService{client: cl, scheme: s}

	// Mock request to simulate Reconcile() being called on an event for a
	// watched resource .
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}
	log.Info("Calling Reconcile()")
	res, err := r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}

	// Check the result of reconciliation to make sure it has the desired state.
	if res.Requeue {
		t.Error("reconcile requeue which is not expected")
	}

	// Reconcile again so Reconcile() checks routes and updates the Service
	// resources' Status.
	res, err = r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	if res != (reconcile.Result{}) {
		t.Error("reconcile did not return an empty Result")
	}

	// Get the updated Service object.
	actualSvc := &corev1.Service{}
	err = r.client.Get(context.TODO(), req.NamespacedName, actualSvc)
	if err != nil {
		t.Errorf("get service: (%v)", err)
	}
	if !metav1.HasAnnotation(actualSvc.ObjectMeta, ELBAnnotationKey) {
		t.Error("service does not have expected annotation")
	}
}
