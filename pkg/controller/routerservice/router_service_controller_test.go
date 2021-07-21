package routerservice

import (
	"context"
	"fmt"
	"testing"

	"github.com/openshift/cloud-ingress-operator/pkg/testutils"
	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
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

func TestReconcile(t *testing.T) {
	// Set the logger to development mode for verbose logs.
	logf.SetLogger(zap.New())

	var (
		name      = "router-default"
		namespace = "openshift-ingress"
	)

	// router-default service
	routerDefaultSvc := testutils.ClientObj{
		Obj: &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: corev1.ServiceSpec{
				Type: corev1.ServiceTypeLoadBalancer,
			},
		},
		GroupVersion: corev1.SchemeGroupVersion,
	}

	tests := []struct {
		Name          string
		Resp          reconcile.Result
		ClientObj     []testutils.ClientObj
		RuntimeObj    []testutils.RuntimeObj
		ClientErr     map[string]string // used to instruct the client to generate an error on k8sclient Update, Delete or Create
		ErrorExpected bool
		ErrorReason   string
	}{
		{
			Name:          "Should complete without error when router service is NotFound",
			Resp:          reconcile.Result{},
			ErrorExpected: false,
			ClientObj:     []testutils.ClientObj{routerDefaultSvc},
			ClientErr:     map[string]string{"on": "Get", "type": "IsNotFound"},
		},
		{
			Name:          "Should error when failing to retrieve the router service",
			Resp:          reconcile.Result{},
			ErrorExpected: true,
			ErrorReason:   "InternalError",
			ClientObj:     []testutils.ClientObj{routerDefaultSvc},
			ClientErr:     map[string]string{"on": "Get", "type": "InternalError"},
		},
		{
			Name:          "Should error when failing to update the service annotation",
			Resp:          reconcile.Result{},
			ErrorExpected: true,
			ErrorReason:   "InternalError",
			ClientObj:     []testutils.ClientObj{routerDefaultSvc},
			ClientErr:     map[string]string{"on": "Update", "type": "InternalError"},
		},
		{
			Name:          "Should complete without error router service is already up to date",
			Resp:          reconcile.Result{},
			ErrorExpected: false,
			ClientObj: []testutils.ClientObj{
				{
					Obj: &corev1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Name:        name,
							Namespace:   namespace,
							Annotations: map[string]string{ELBAnnotationKey: ELBAnnotationValue},
						},
						Spec: corev1.ServiceSpec{
							Type: corev1.ServiceTypeLoadBalancer,
						},
					},
					GroupVersion: corev1.SchemeGroupVersion,
				},
			},
		},
	}

	for _, test := range tests {
		testClient, testScheme := testutils.SetUpTestClient(test.ClientObj, test.RuntimeObj, test.ClientErr["on"], test.ClientErr["type"], test.ClientErr["target"])
		r := &ReconcileRouterService{client: testClient, scheme: testScheme}
		result, err := r.Reconcile(context.TODO(), reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      name,
				Namespace: namespace,
			},
		})

		if err == nil && test.ErrorExpected || err != nil && !test.ErrorExpected {
			t.Fatalf("Test [%v] return mismatch. Expect error? %t: Return %+v", test.Name, test.ErrorExpected, err)
		}
		if err != nil && test.ErrorExpected && test.ErrorReason != fmt.Sprint(k8serr.ReasonForError(err)) {
			t.Fatalf("Test [%v] FAILED. Excepted Error %v. Got %v", test.Name, test.ErrorReason, k8serr.ReasonForError(err))
		}
		if result != test.Resp {
			t.Fatalf("Test [%v] FAILED. Excepted Response %v. Got %v", test.Name, test.Resp, result)
		}
	}
}
