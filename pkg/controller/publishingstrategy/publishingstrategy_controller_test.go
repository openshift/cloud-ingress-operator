package publishingstrategy

import (
	"fmt"
	"testing"
	"time"

	"context"

	cloudingressv1alpha1 "github.com/openshift/cloud-ingress-operator/pkg/apis/cloudingress/v1alpha1"
	"github.com/openshift/cloud-ingress-operator/pkg/ingresscontroller"
	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestGetIngressName(t *testing.T) {

	domainName := "apps2.test.domain_name.org"

	expected := "apps2"
	result := getIngressName(domainName)

	if expected != result {
		t.Errorf("got %s \n, expected %s \n", result, expected)
	}
}

func TestGenerateIngressController(t *testing.T) {

	// expected result
	expected := &ingresscontroller.IngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "apps3",
			Namespace: "openshift-ingress-operator",
		},
		Spec: ingresscontroller.IngressControllerSpec{
			DefaultCertificate: &corev1.LocalObjectReference{
				Name: "example-cert-nondefault",
			},
			Domain: "example-domain-nondefault.example.com",
			EndpointPublishingStrategy: &ingresscontroller.EndpointPublishingStrategy{
				Type: ingresscontroller.LoadBalancerServiceStrategyType,
				LoadBalancer: &ingresscontroller.LoadBalancerStrategy{
					Scope: ingresscontroller.ExternalLoadBalancer,
				},
			},
			RouteSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{},
			},
		},
	}

	applicationIngress := cloudingressv1alpha1.ApplicationIngress{
		Listening:   "External",
		Default:     false,
		DNSName:     "example-domain-nondefault.example.com",
		Certificate: corev1.SecretReference{Name: "example-cert-nondefault", Namespace: "openshift-ingress-operator"},
	}

	result := generateIngressController(applicationIngress)

	// since these are pointers to different struct the pointer addresses are not the same, therefore reflect.DeepEqual won't work
	// compare parts that we can
	if result.Name != expected.Name && result.Spec.DefaultCertificate.Name != expected.Spec.DefaultCertificate.Name && result.Spec.Domain != expected.Spec.Domain {
		t.Errorf("expected different ingresscontroller")
	}
}

func TestValidateStaticStatus(t *testing.T) {

	// Build ApplicationIngress
	applicationIngress := cloudingressv1alpha1.ApplicationIngress{
		Listening:   "internal",
		Default:     true,
		DNSName:     "example.com",
		Certificate: corev1.SecretReference{Name: "example-cert-nondefault", Namespace: "openshift-ingress-operator"},
	}
	// Generate desired IngressContoller
	desiredIngressController := generateIngressController(applicationIngress)

	var replicas int32 = 2
	// Build "actual" IngressController that should fail
	actualIngressController1 := &ingresscontroller.IngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: "openshift-ingress-operator",
		},
		Spec: ingresscontroller.IngressControllerSpec{
			Replicas: &replicas,
		},
		Status: ingresscontroller.IngressControllerStatus{
			Domain: "example.com",
			EndpointPublishingStrategy: &ingresscontroller.EndpointPublishingStrategy{
				Type: ingresscontroller.LoadBalancerServiceStrategyType,
				LoadBalancer: &ingresscontroller.LoadBalancerStrategy{
					Scope: ingresscontroller.ExternalLoadBalancer,
				},
			},
		},
	}

	result := validateStaticStatus(*actualIngressController1, desiredIngressController.Spec)

	if result == true {
		t.Errorf("Expected IngressController and desired config to be different")
	}

	// Build "actual" IngressController that should pass
	actualIngressController2 := &ingresscontroller.IngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: "openshift-ingress-operator",
		},
		Spec: ingresscontroller.IngressControllerSpec{
			Replicas: &replicas,
		},
		Status: ingresscontroller.IngressControllerStatus{
			Domain: "example.com",
			EndpointPublishingStrategy: &ingresscontroller.EndpointPublishingStrategy{
				Type: ingresscontroller.LoadBalancerServiceStrategyType,
				LoadBalancer: &ingresscontroller.LoadBalancerStrategy{
					Scope: ingresscontroller.InternalLoadBalancer,
				},
			},
		},
	}

	result2 := validateStaticStatus(*actualIngressController2, desiredIngressController.Spec)

	if result2 == false {
		t.Errorf("Expected IngressController and desired config to be the same: %+v\n %+v\n", actualIngressController2.Status.EndpointPublishingStrategy.LoadBalancer.Scope, desiredIngressController.Spec.EndpointPublishingStrategy.LoadBalancer.Scope)
	}
}

func TestValidateStaticSpec(t *testing.T) {

	// Build ApplicationIngress
	applicationIngress := cloudingressv1alpha1.ApplicationIngress{
		Listening:   "external",
		Default:     false,
		DNSName:     "example-domain-nondefault.example.com",
		Certificate: corev1.SecretReference{Name: "example-cert-nondefault", Namespace: "openshift-ingress-operator"},
	}
	// Generate desired IngressContoller
	desiredIngressController := generateIngressController(applicationIngress)

	// Build "actual" IngressController that should fail
	actualIngressController1 := &ingresscontroller.IngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "apps3",
			Namespace: "openshift-ingress-operator",
		},
		Spec: ingresscontroller.IngressControllerSpec{
			DefaultCertificate: &corev1.LocalObjectReference{
				Name: "example-cert-nondefault",
			},
			Domain: "example-domain.example.com",
			EndpointPublishingStrategy: &ingresscontroller.EndpointPublishingStrategy{
				Type: ingresscontroller.LoadBalancerServiceStrategyType,
				LoadBalancer: &ingresscontroller.LoadBalancerStrategy{
					Scope: ingresscontroller.ExternalLoadBalancer,
				},
			},
			RouteSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{},
			},
		},
	}

	result := validateStaticSpec(*actualIngressController1, desiredIngressController.Spec)

	if result == true {
		t.Errorf("Expected IngressController and desired config to be different")
	}

	// Build "actual" IngressController that should pass
	actualIngressController2 := &ingresscontroller.IngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "apps3",
			Namespace: "openshift-ingress-operator",
		},
		Spec: ingresscontroller.IngressControllerSpec{
			DefaultCertificate: &corev1.LocalObjectReference{
				Name: "example-cert-nondefault",
			},
			Domain: "example-domain-nondefault.example.com",
			EndpointPublishingStrategy: &ingresscontroller.EndpointPublishingStrategy{
				Type: ingresscontroller.LoadBalancerServiceStrategyType,
				LoadBalancer: &ingresscontroller.LoadBalancerStrategy{
					Scope: ingresscontroller.ExternalLoadBalancer,
				},
			},
			RouteSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{},
			},
		},
	}

	result2 := validateStaticSpec(*actualIngressController2, desiredIngressController.Spec)

	if result2 == false {
		t.Errorf("Expected IngressController and desired config to be the same: %+v\n %+v\n", actualIngressController2.Spec.EndpointPublishingStrategy, desiredIngressController.Spec.EndpointPublishingStrategy)
	}

}

func TestValidatePatchableSpec(t *testing.T) {

	// Build ApplicationIngress
	applicationIngress := cloudingressv1alpha1.ApplicationIngress{
		Listening:   "External",
		Default:     true,
		DNSName:     "example-domain-nondefault.example.com",
		Certificate: corev1.SecretReference{Name: "example-cert-nondefault", Namespace: "openshift-ingress-operator"},
		RouteSelector: metav1.LabelSelector{
			MatchLabels: map[string]string{"foo": "bar"},
		},
	}
	// Generate desired IngressContoller
	desiredIngressController := generateIngressController(applicationIngress)

	// Build "actual" IngressController that should fail
	actualIngressController1 := &ingresscontroller.IngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "apps3",
			Namespace: "openshift-ingress-operator",
		},
		Spec: ingresscontroller.IngressControllerSpec{
			DefaultCertificate: &corev1.LocalObjectReference{
				Name: "example-cert-nondefault",
			},
			NodePlacement: &ingresscontroller.NodePlacement{
				NodeSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"node-role.kubernetes.io/infra": ""},
				},
				Tolerations: []corev1.Toleration{
					{
						Key:      "node-role.kubernetes.io/infra",
						Effect:   corev1.TaintEffectNoSchedule,
						Operator: corev1.TolerationOpExists,
					},
				},
			},
			Domain: "example-domain.example.com",
			EndpointPublishingStrategy: &ingresscontroller.EndpointPublishingStrategy{
				Type: ingresscontroller.LoadBalancerServiceStrategyType,
				LoadBalancer: &ingresscontroller.LoadBalancerStrategy{
					Scope: ingresscontroller.ExternalLoadBalancer,
				},
			},
			RouteSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{},
			},
		},
	}

	result, field := validatePatchableSpec(*actualIngressController1, desiredIngressController.Spec)

	if result == true {
		t.Errorf("Expected IngressController and desired config to be different")
	} else if field != IngressControllerSelector {
		t.Errorf("Expected IngressController and desired config to have different RouteSelectors different")
	}

	// Build "actual" IngressController that should pass
	actualIngressController2 := &ingresscontroller.IngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "apps3",
			Namespace: "openshift-ingress-operator",
		},
		Spec: ingresscontroller.IngressControllerSpec{
			DefaultCertificate: &corev1.LocalObjectReference{
				Name: "example-cert-nondefault",
			},
			NodePlacement: &ingresscontroller.NodePlacement{
				NodeSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"node-role.kubernetes.io/infra": ""},
				},
				Tolerations: []corev1.Toleration{
					{
						Key:      "node-role.kubernetes.io/infra",
						Effect:   corev1.TaintEffectNoSchedule,
						Operator: corev1.TolerationOpExists,
					},
				},
			},
			Domain: "example-domain.example.com",
			EndpointPublishingStrategy: &ingresscontroller.EndpointPublishingStrategy{
				Type: ingresscontroller.LoadBalancerServiceStrategyType,
				LoadBalancer: &ingresscontroller.LoadBalancerStrategy{
					Scope: ingresscontroller.ExternalLoadBalancer,
				},
			},
			RouteSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"foo": "bar"},
			},
		},
	}

	result2, _ := validatePatchableSpec(*actualIngressController2, desiredIngressController.Spec)

	if result2 == false {
		t.Errorf("Expected IngressController and desired config to be the same %+v\n %+v\n", actualIngressController2.Spec.RouteSelector.MatchLabels, desiredIngressController.Spec.RouteSelector.MatchLabels)
	}
}

func TestValidatePatchableStatus(t *testing.T) {

	// Build ApplicationIngress
	applicationIngress := cloudingressv1alpha1.ApplicationIngress{
		Listening:   "External",
		Default:     true,
		DNSName:     "example-domain-nondefault.example.com",
		Certificate: corev1.SecretReference{Name: "example-cert-nondefault", Namespace: "openshift-ingress-operator"},
		RouteSelector: metav1.LabelSelector{
			MatchLabels: map[string]string{"foo": "bar"},
		},
	}
	// Generate desired IngressContoller
	desiredIngressController := generateIngressController(applicationIngress)

	// Build "actual" IngressController that should fail
	actualIngressController1 := &ingresscontroller.IngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "apps3",
			Namespace: "openshift-ingress-operator",
		},
		Spec: ingresscontroller.IngressControllerSpec{
			DefaultCertificate: &corev1.LocalObjectReference{
				Name: "example-cert-nondefault",
			},
			Domain: "example-domain.example.com",
			EndpointPublishingStrategy: &ingresscontroller.EndpointPublishingStrategy{
				Type: ingresscontroller.LoadBalancerServiceStrategyType,
				LoadBalancer: &ingresscontroller.LoadBalancerStrategy{
					Scope: ingresscontroller.ExternalLoadBalancer,
				},
			},
			RouteSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{},
			},
		},
	}

	result, field := validatePatchableStatus(*actualIngressController1, desiredIngressController.Spec)

	if result == true {
		t.Errorf("Expected IngressController and desired config to be different")
	} else if field != IngressControllerSelector {
		t.Errorf("Expected IngressController and desired config to have different RouteSelectors different")
	}

	// Build "actual" IngressController that should pass
	actualIngressController2 := &ingresscontroller.IngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "apps3",
			Namespace: "openshift-ingress-operator",
		},
		Spec: ingresscontroller.IngressControllerSpec{
			DefaultCertificate: &corev1.LocalObjectReference{
				Name: "example-cert-nondefault",
			},
			Domain: "example-domain.example.com",
			EndpointPublishingStrategy: &ingresscontroller.EndpointPublishingStrategy{
				Type: ingresscontroller.LoadBalancerServiceStrategyType,
				LoadBalancer: &ingresscontroller.LoadBalancerStrategy{
					Scope: ingresscontroller.ExternalLoadBalancer,
				},
			},
		},
		Status: ingresscontroller.IngressControllerStatus{
			Selector: "foo=bar",
		},
	}

	result2, _ := validatePatchableStatus(*actualIngressController2, desiredIngressController.Spec)

	if result2 == false {
		t.Errorf("Expected IngressController and desired config to be the same %+v\n %+v\n", actualIngressController2.Status.Selector, desiredIngressController.Spec.RouteSelector.MatchLabels)
	}
}

func TestEnsureIngressController(t *testing.T) {
	desiredIngressController := makeIngressControllerCR("default", "internal", []string{ClusterIngressFinalizer})

	tests := []struct {
		Name              string
		IngressController *ingresscontroller.IngressController
		Resp              reconcile.Result
		ClientErr         map[string]string // used to instruct the client to generate an error on k8sclient Update, Delete or Create
		ErrorExpected     bool
		ErrorReason       string
	}{
		{
			Name:              "Should wait for ClusterIngressFinalizer to be deleted",
			IngressController: makeIngressControllerCR("default", "external", []string{ClusterIngressFinalizer}),
			Resp:              reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second},
			ErrorExpected:     false,
		},
		{
			Name:              "Should wait for RandomIngressFinalizer to be deleted",
			IngressController: makeIngressControllerCR("default", "external", []string{"RandomIngressFinalizer"}),
			Resp:              reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second},
			ErrorExpected:     false,
		},
		{
			Name:              "Should requeue when failing to delete CloudIngressFinalizer",
			IngressController: makeIngressControllerCR("default", "external", []string{CloudIngressFinalizer}),
			ClientErr:         map[string]string{"on": "Update", "type": "InternalError"},
			Resp:              reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second},
			ErrorExpected:     true,
			ErrorReason:       "InternalError",
		},
		{
			Name:              "Should requeue when failing to delete the IngressContoller",
			IngressController: makeIngressControllerCR("default", "external", []string{CloudIngressFinalizer}),
			ClientErr:         map[string]string{"on": "Delete", "type": "InternalError"},
			Resp:              reconcile.Result{Requeue: true},
			ErrorExpected:     true,
			ErrorReason:       "InternalError",
		},
		{
			Name:              "Should proceed if cluster-ingress already deleted the IngressController. However requeue and error if cluster-ingress was faster recreating it",
			IngressController: makeIngressControllerCR("default", "external", []string{CloudIngressFinalizer}),
			ClientErr:         map[string]string{"on": "Delete", "type": "IsNotFound"},
			Resp:              reconcile.Result{Requeue: true},
			ErrorExpected:     true,
			ErrorReason:       "AlreadyExists",
		},
		{
			Name:              "Should requeue and create desiredIngressController if deletion was successful",
			IngressController: makeIngressControllerCR("default", "external", []string{CloudIngressFinalizer}),
			Resp:              reconcile.Result{Requeue: true},
			ErrorExpected:     false,
		},
	}

	for _, test := range tests {
		testClient, testScheme := setUpTestClient([]client.Object{test.IngressController}, []runtime.Object{}, test.ClientErr["on"], test.ClientErr["type"], test.ClientErr["target"])
		r := &ReconcilePublishingStrategy{client: testClient, scheme: testScheme}
		result, err := r.ensureIngressController(log, test.IngressController, desiredIngressController)

		if err == nil && test.ErrorExpected || err != nil && !test.ErrorExpected {
			t.Fatalf("Test [%v] return mismatch. Expect error? %t: Return %+v", test.Name, test.ErrorExpected, err)
		}
		if err != nil && test.ErrorExpected && test.ErrorReason != fmt.Sprint(k8serr.ReasonForError(err)) {
			t.Fatalf("Test [%v] FAILED. Expected Error %v. Got %v", test.Name, test.ErrorReason, k8serr.ReasonForError(err))
		}
		if result != test.Resp {
			t.Fatalf("Test [%v] FAILED. Expected Response %v. Got %v", test.Name, test.Resp, result)
		}
	}

}

func TestDeleteUnpublishedIngressControllers(t *testing.T) {
	tests := []struct {
		Name              string
		IngressController *ingresscontroller.IngressController
		Map               map[string]bool
		Resp              reconcile.Result
		ErrorExpected     bool
		ErrorReason       string
		ClientErr         map[string]string // used to instruct the client to generate an error on k8sclient Update, Delete or Create
	}{
		{
			Name:              "Should do nothing when there all IngressController are in the publishingstrategy",
			IngressController: &ingresscontroller.IngressController{},
			Map:               map[string]bool{"default": true},
			Resp:              reconcile.Result{},
			ErrorExpected:     false,
		},
		{
			Name:              "Should error when failing to get the IngressController to delete",
			IngressController: makeIngressControllerCR("test-ingress-controller", "external", []string{ClusterIngressFinalizer}),
			Map:               map[string]bool{"test-ingress-controller": false},
			Resp:              reconcile.Result{},
			ErrorExpected:     true,
			ErrorReason:       "NotFound",
			ClientErr:         map[string]string{"on": "Get", "type": "IsNotFound"},
		},
		{
			Name:              "Should error when failing to delete the IngressController",
			IngressController: makeIngressControllerCR("test-ingress-controller", "external", []string{ClusterIngressFinalizer}),
			Map:               map[string]bool{"test-ingress-controller": false},
			Resp:              reconcile.Result{},
			ErrorExpected:     true,
			ErrorReason:       "NotFound",
			ClientErr:         map[string]string{"on": "Delete", "type": "IsNotFound"},
		},
	}
	for _, test := range tests {
		testClient, testScheme := setUpTestClient([]client.Object{test.IngressController}, []runtime.Object{}, test.ClientErr["on"], test.ClientErr["type"], test.ClientErr["target"])
		r := &ReconcilePublishingStrategy{client: testClient, scheme: testScheme}
		result, err := r.deleteUnpublishedIngressControllers(test.Map)

		if err == nil && test.ErrorExpected || err != nil && !test.ErrorExpected {
			t.Fatalf("Test [%v] return mismatch. Expect error? %t: Return %+v", test.Name, test.ErrorExpected, err)
		}
		if err != nil && test.ErrorExpected && test.ErrorReason != fmt.Sprint(k8serr.ReasonForError(err)) {
			t.Fatalf("Test [%v] FAILED. Expected Error %v. Got %v", test.Name, test.ErrorReason, k8serr.ReasonForError(err))
		}
		if result != test.Resp {
			t.Fatalf("Test [%v] FAILED. Expected Response %v. Got %v", test.Name, test.Resp, result)
		}
	}
}

func TestEnsureStaticSpec(t *testing.T) {
	tests := []struct {
		Name                     string
		IngressController        *ingresscontroller.IngressController
		DesiredIngressController *ingresscontroller.IngressController
		Resp                     reconcile.Result
		ErrorExpected            bool
		ErrorReason              string
		ClientErr                map[string]string // used to instruct the client to generate an error on k8sclient Update, Delete or Create
	}{
		{
			Name:                     "Should requeue with error when failing to add CloudIngressFinalizer",
			IngressController:        makeIngressControllerCR("default", "external", []string{ClusterIngressFinalizer}),
			DesiredIngressController: makeIngressControllerCR("default", "internal", []string{ClusterIngressFinalizer}),
			Resp:                     reconcile.Result{Requeue: true},
			ErrorExpected:            true,
			ErrorReason:              "InternalError",
			ClientErr:                map[string]string{"on": "Update", "type": "InternalError"},
		},
		{
			Name:                     "Should requeue without error when failing to mark default IngressController for Deletion",
			IngressController:        makeIngressControllerCR("default", "external", []string{ClusterIngressFinalizer}),
			DesiredIngressController: makeIngressControllerCR("default", "internal", []string{ClusterIngressFinalizer}),
			Resp:                     reconcile.Result{Requeue: true},
			ErrorExpected:            false,
			ClientErr:                map[string]string{"on": "Delete", "type": "InternalError"},
		},
		{
			Name:                     "Should requeue without error when failing to mark non-default IngressController for Deletion",
			IngressController:        makeIngressControllerCR("non-default", "external", []string{ClusterIngressFinalizer}),
			DesiredIngressController: makeIngressControllerCR("non-default", "internal", []string{ClusterIngressFinalizer}),
			Resp:                     reconcile.Result{Requeue: true},
			ErrorExpected:            false,
			ClientErr:                map[string]string{"on": "Delete", "type": "InternalError"},
		},
		{
			Name:                     "Should do nothing when IngressController and DesiredIngressController match",
			IngressController:        makeIngressControllerCR("non-default", "internal", []string{ClusterIngressFinalizer}),
			DesiredIngressController: makeIngressControllerCR("non-default", "internal", []string{ClusterIngressFinalizer}),
			Resp:                     reconcile.Result{},
			ErrorExpected:            false,
		},
	}
	for _, test := range tests {
		testClient, testScheme := setUpTestClient([]client.Object{test.IngressController}, []runtime.Object{}, test.ClientErr["on"], test.ClientErr["type"], test.ClientErr["target"])
		r := &ReconcilePublishingStrategy{client: testClient, scheme: testScheme}
		result, err := r.ensureStaticSpec(log, test.IngressController, test.DesiredIngressController)

		if err == nil && test.ErrorExpected || err != nil && !test.ErrorExpected {
			t.Fatalf("Test [%v] return mismatch. Expect error? %t: Return %+v", test.Name, test.ErrorExpected, err)
		}
		if err != nil && test.ErrorExpected && test.ErrorReason != fmt.Sprint(k8serr.ReasonForError(err)) {
			t.Fatalf("Test [%v] FAILED. Expected Error %v. Got %v", test.Name, test.ErrorReason, k8serr.ReasonForError(err))
		}
		if result != test.Resp {
			t.Fatalf("Test [%v] FAILED. Expected Response %v. Got %v", test.Name, test.Resp, result)
		}
	}
}

func TestEnsurePatchableSpec(t *testing.T) {
	testDefaultCert := corev1.LocalObjectReference{Name: "random-cert-name"}
	testNodePlacement := ingresscontroller.NodePlacement{
		NodeSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"random": "label"},
		},
	}
	testRouterSelector := metav1.LabelSelector{MatchLabels: map[string]string{"random": "label"}}

	tests := []struct {
		Name                     string
		IngressController        *ingresscontroller.IngressController
		DesiredIngressController *ingresscontroller.IngressController
		Resp                     reconcile.Result
		ErrorExpected            bool
		ErrorReason              string
		ClientErr                map[string]string // used to instruct the client to generate an error on k8sclient Update, Delete or Create
	}{
		{
			Name:                     "Should do nothing when there are no patchable spec changes",
			IngressController:        makeIngressControllerCR("default", "external", []string{ClusterIngressFinalizer}),
			DesiredIngressController: makeIngressControllerCR("default", "internal", []string{ClusterIngressFinalizer}),
			Resp:                     reconcile.Result{},
			ErrorExpected:            false,
		},
		{
			Name:                     "Should error when failing to patch default IngressController",
			IngressController:        makeIngressControllerCR("default", "external", []string{ClusterIngressFinalizer}),
			DesiredIngressController: makeIngressControllerCR("default", "external", []string{ClusterIngressFinalizer}, testRouterSelector),
			Resp:                     reconcile.Result{},
			ErrorExpected:            true,
			ErrorReason:              "InternalError",
			ClientErr:                map[string]string{"on": "Patch", "type": "InternalError"},
		},
		{
			Name:                     "Should requeue without error when successfully patching default IngressController",
			IngressController:        makeIngressControllerCR("default", "external", []string{ClusterIngressFinalizer}),
			DesiredIngressController: makeIngressControllerCR("default", "external", []string{ClusterIngressFinalizer}, testRouterSelector),
			Resp:                     reconcile.Result{Requeue: true},
			ErrorExpected:            false,
		},
		{
			Name:                     "Should error when failing to patch non-default IngressController",
			IngressController:        makeIngressControllerCR("nondefault", "external", []string{ClusterIngressFinalizer}),
			DesiredIngressController: makeIngressControllerCR("nondefault", "external", []string{ClusterIngressFinalizer}, testRouterSelector),
			Resp:                     reconcile.Result{},
			ErrorExpected:            true,
			ErrorReason:              "InternalError",
			ClientErr:                map[string]string{"on": "Patch", "type": "InternalError"},
		},
		{
			Name:                     "Should requeue without error when patching IngressControllerCertificate of non-default IngressController",
			IngressController:        makeIngressControllerCR("nondefault", "external", []string{ClusterIngressFinalizer}),
			DesiredIngressController: makeIngressControllerCR("nondefault", "external", []string{ClusterIngressFinalizer}, testDefaultCert),
			Resp:                     reconcile.Result{Requeue: true},
			ErrorExpected:            false,
		},
		{
			Name:                     "Should requeue without error when patching IngressControllerNodePlacement of non-default IngressController",
			IngressController:        makeIngressControllerCR("nondefault", "external", []string{ClusterIngressFinalizer}, testNodePlacement),
			DesiredIngressController: makeIngressControllerCR("nondefault", "external", []string{ClusterIngressFinalizer}),
			Resp:                     reconcile.Result{Requeue: true},
			ErrorExpected:            false,
		},
	}

	for _, test := range tests {
		testClient, testScheme := setUpTestClient([]client.Object{test.IngressController}, []runtime.Object{}, test.ClientErr["on"], test.ClientErr["type"], test.ClientErr["target"])
		r := &ReconcilePublishingStrategy{client: testClient, scheme: testScheme}
		result, err := r.ensurePatchableSpec(log, test.IngressController, test.DesiredIngressController)

		if err == nil && test.ErrorExpected || err != nil && !test.ErrorExpected {
			t.Fatalf("Test [%v] return mismatch. Expect error? %t: Return %+v", test.Name, test.ErrorExpected, err)
		}
		if err != nil && test.ErrorExpected && test.ErrorReason != fmt.Sprint(k8serr.ReasonForError(err)) {
			t.Fatalf("Test [%v] FAILED. Expected Error %v. Got %v", test.Name, test.ErrorReason, k8serr.ReasonForError(err))
		}
		if result != test.Resp {
			t.Fatalf("Test [%v] FAILED. Expected Response %v. Got %v", test.Name, test.Resp, result)
		}
	}
}

func TestReconcile(t *testing.T) {
	defaultPublishingStrategy := &cloudingressv1alpha1.PublishingStrategy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "publishingstrategy",
			Namespace: "openshift-cloud-ingress-operator",
		},
		Spec: cloudingressv1alpha1.PublishingStrategySpec{
			DefaultAPIServerIngress: cloudingressv1alpha1.DefaultAPIServerIngress{Listening: cloudingressv1alpha1.External},
			ApplicationIngress: []cloudingressv1alpha1.ApplicationIngress{
				{
					Default:       true,
					DNSName:       "example-domain.example.com",
					Listening:     "external",
					Certificate:   corev1.SecretReference{Name: "test-cert-bundle-secret", Namespace: "openshift-ingress-operator"},
					RouteSelector: metav1.LabelSelector{MatchLabels: map[string]string{}},
				},
			},
		},
	}

	tests := []struct {
		Name          string
		Resp          reconcile.Result
		ClientObj     []client.Object
		RuntimeObj    []runtime.Object
		ClientErr     map[string]string // used to instruct the client to generate an error on k8sclient Update, Delete or Create
		ErrorExpected bool
		ErrorReason   string
	}{
		{
			Name:          "Should complete without error when PublishingStrategy is NotFound",
			Resp:          reconcile.Result{},
			ErrorExpected: false,
			ClientErr:     map[string]string{"on": "Get", "type": "IsNotFound"},
		},
		{
			Name:          "Should error when failing to retrieve PublishingStrategy",
			Resp:          reconcile.Result{},
			ErrorExpected: true,
			ErrorReason:   "InternalError",
			ClientErr:     map[string]string{"on": "Get", "type": "InternalError"},
		},
		{
			Name:          "Should error when failing to list IngressControllerList",
			Resp:          reconcile.Result{},
			ErrorExpected: true,
			ErrorReason:   "InternalError",
			ClientObj:     []client.Object{defaultPublishingStrategy},
			ClientErr:     map[string]string{"on": "List", "type": "InternalError"},
		},
		{
			Name:          "Should error when failing to retrieve ingresscontroller",
			Resp:          reconcile.Result{},
			ErrorExpected: true,
			ErrorReason:   "InternalError",
			ClientObj:     []client.Object{defaultPublishingStrategy, &ingresscontroller.IngressController{}},
			ClientErr:     map[string]string{"on": "Get", "type": "InternalError"},
			RuntimeObj:    []runtime.Object{&ingresscontroller.IngressControllerList{}},
		},
		{
			Name:          "Should error when failing to create missing ingresscontroller",
			Resp:          reconcile.Result{},
			ErrorExpected: true,
			ErrorReason:   "InternalError",
			ClientObj:     []client.Object{defaultPublishingStrategy, &ingresscontroller.IngressController{}},
			ClientErr:     map[string]string{"on": "Create", "type": "InternalError"},
			RuntimeObj:    []runtime.Object{&ingresscontroller.IngressControllerList{}},
		},
		{
			Name:          "Should requeue when succesfully creating missing ingresscontroller",
			Resp:          reconcile.Result{Requeue: true},
			ErrorExpected: false,
			ClientObj:     []client.Object{defaultPublishingStrategy, &ingresscontroller.IngressController{}},
			RuntimeObj:    []runtime.Object{&ingresscontroller.IngressControllerList{}},
		},
		{
			Name:          "Should requeue with delay when ingresscontroller is marked as deleted",
			Resp:          reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second},
			ErrorExpected: false,
			ClientObj:     []client.Object{defaultPublishingStrategy, makeIngressControllerCR("default", "external", []string{ClusterIngressFinalizer}, metav1.Now())},
			RuntimeObj:    []runtime.Object{&ingresscontroller.IngressControllerList{}},
		},
		{
			Name:          "Should requeue with erorr when failing to ensure static specs on ingresscontroller",
			Resp:          reconcile.Result{Requeue: true},
			ErrorExpected: true,
			ErrorReason:   "InternalError",
			ClientObj:     []client.Object{defaultPublishingStrategy, makeIngressControllerCR("default", "internal", []string{ClusterIngressFinalizer})},
			ClientErr:     map[string]string{"on": "Update", "type": "InternalError"},
			RuntimeObj:    []runtime.Object{&ingresscontroller.IngressControllerList{}},
		},
		{
			Name:          "Should erorr when failing to ensure patchable specs on ingresscontroller",
			Resp:          reconcile.Result{},
			ErrorExpected: true,
			ErrorReason:   "InternalError",
			ClientObj: []client.Object{
				defaultPublishingStrategy,
				makeIngressControllerCR("default", "external", []string{ClusterIngressFinalizer}, metav1.LabelSelector{MatchLabels: map[string]string{"random": "label"}}),
			},
			ClientErr:  map[string]string{"on": "Patch", "type": "InternalError"},
			RuntimeObj: []runtime.Object{&ingresscontroller.IngressControllerList{}},
		},
		{
			Name:          "Should erorr when failing delete punblished ingresscontroller",
			Resp:          reconcile.Result{},
			ErrorExpected: true,
			ErrorReason:   "InternalError",
			ClientObj: []client.Object{
				defaultPublishingStrategy,
				makeIngressControllerCR("default", "external", []string{ClusterIngressFinalizer}),
				makeIngressControllerCR("unpublished-ingress", "external", []string{}),
			},
			ClientErr:  map[string]string{"on": "Delete", "type": "InternalError"},
			RuntimeObj: []runtime.Object{&ingresscontroller.IngressControllerList{}},
		},
	}

	for _, test := range tests {
		testClient, testScheme := setUpTestClient(test.ClientObj, test.RuntimeObj, test.ClientErr["on"], test.ClientErr["type"], test.ClientErr["target"])
		r := &ReconcilePublishingStrategy{client: testClient, scheme: testScheme}
		result, err := r.Reconcile(context.TODO(), reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "publishingstrategy",
				Namespace: "openshift-cloud-ingress-operator",
			},
		})

		if err == nil && test.ErrorExpected || err != nil && !test.ErrorExpected {
			t.Fatalf("Test [%v] return mismatch. Expect error? %t: Return %+v", test.Name, test.ErrorExpected, err)
		}
		if err != nil && test.ErrorExpected && test.ErrorReason != fmt.Sprint(k8serr.ReasonForError(err)) {
			t.Fatalf("Test [%v] FAILED. Expected Error %v. Got %v", test.Name, test.ErrorReason, k8serr.ReasonForError(err))
		}
		if result != test.Resp {
			t.Fatalf("Test [%v] FAILED. Expected Response %v. Got %v", test.Name, test.Resp, result)
		}
	}
}

// utils
// makeIngressControllerCR creates an IngressControllerCR
func makeIngressControllerCR(name, lbScope string, finalizers []string, overrides ...interface{}) *ingresscontroller.IngressController {
	var scope ingresscontroller.LoadBalancerScope
	var timestamp metav1.Time

	routerSelector := metav1.LabelSelector{}
	defaultCert := corev1.LocalObjectReference{Name: "test-cert-bundle-secret"}
	nodeSelector := ingresscontroller.NodePlacement{
		NodeSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{infraNodeLabelKey: ""},
		},
		Tolerations: []corev1.Toleration{
			{
				Key:      infraNodeLabelKey,
				Effect:   corev1.TaintEffectNoSchedule,
				Operator: corev1.TolerationOpExists,
			},
		},
	}

	switch lbScope {
	case "internal":
		scope = ingresscontroller.InternalLoadBalancer
	default:
		scope = ingresscontroller.ExternalLoadBalancer
	}

	for _, override := range overrides {
		switch v := override.(type) {
		case metav1.Time:
			timestamp = v
		case corev1.LocalObjectReference:
			defaultCert = v
		case metav1.LabelSelector:
			routerSelector = v
		case ingresscontroller.NodePlacement:
			nodeSelector = v
		}

	}

	return &ingresscontroller.IngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "openshift-ingress-operator",
			Annotations: map[string]string{
				"Owner": "cloud-ingress-operator",
			},
			Finalizers:        finalizers,
			DeletionTimestamp: &timestamp,
		},
		Spec: ingresscontroller.IngressControllerSpec{
			DefaultCertificate: &defaultCert,

			Domain: "example-domain.example.com",
			EndpointPublishingStrategy: &ingresscontroller.EndpointPublishingStrategy{
				Type: ingresscontroller.LoadBalancerServiceStrategyType,
				LoadBalancer: &ingresscontroller.LoadBalancerStrategy{
					Scope: scope,
				},
			},
			NodePlacement: &nodeSelector,
			RouteSelector: &routerSelector,
		},
	}
}

//setUpTestClient builds and returns a fakeclient for testing
//func setUpTestClient(cr *operatorv1.IngressController, errorOn, errorType string) (*customClient, *runtime.Scheme) {
func setUpTestClient(cr []client.Object, ro []runtime.Object, errorOn, errorType, errorTarget string) (*customClient, *runtime.Scheme) {
	s := scheme.Scheme
	for _, v := range cr {
		s.AddKnownTypes(cloudingressv1alpha1.SchemeGroupVersion, v)
	}
	for _, v := range ro {
		s.AddKnownTypes(cloudingressv1alpha1.SchemeGroupVersion, v)
	}

	testClient := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(ro...).WithObjects(cr...).Build()
	return &customClient{testClient, errorOn, errorType, errorTarget}, s
}

// A custom k8s client, which can fail on demand, on get, create, update or delete operations
type customClient struct {
	client.Client
	errorOn     string
	errorType   string
	errorTarget string // when specified, will only error if the action errorOn is done this target.
}

func (c *customClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if c.errorOn == "Update" {
		return getK8sError(c.errorType, fmt.Sprintf("%T", obj))
	}
	return c.Client.Update(ctx, obj, opts...)
}

func (c *customClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if c.errorOn == "Delete" {
		return getK8sError(c.errorType, fmt.Sprintf("%T", obj))
	}
	return c.Client.Delete(ctx, obj, opts...)
}

func (c *customClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if c.errorOn == "Create" {
		return getK8sError(c.errorType, fmt.Sprintf("%T", obj))
	}

	return c.Client.Create(ctx, obj, opts...)
}

func (c *customClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if c.errorOn == "Patch" {
		return getK8sError(c.errorType, fmt.Sprintf("%T", obj))
	}

	return c.Client.Patch(ctx, obj, patch, opts...)
}

func (c *customClient) Get(ctx context.Context, key types.NamespacedName, obj client.Object) error {
	if c.errorOn == "Get" {
		t := fmt.Sprintf("%T", obj)
		if c.errorTarget == "" || c.errorTarget == t {
			return getK8sError(c.errorType, fmt.Sprintf("%T", obj))
		}
	}

	return c.Client.Get(ctx, key, obj)
}

func (c *customClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if c.errorOn == "List" {
		return getK8sError(c.errorType, fmt.Sprintf("%T", list))
	}

	return c.Client.List(ctx, list, opts...)
}

func getK8sError(errorType string, objType string) error {
	errorMap := map[string]error{
		"IsNotFound": k8serr.NewNotFound(schema.GroupResource{Group: "ingresscontrollers.cloudingress.managed.openshift.io",
			Resource: "varies"}, objType),
	}
	if err, found := errorMap[errorType]; found {
		return err
	} else {
		// by default we return internal error, when the error type specified doesn't match something we preconfigured
		return k8serr.NewInternalError(fmt.Errorf("%v was raised", errorType))

	}
}
