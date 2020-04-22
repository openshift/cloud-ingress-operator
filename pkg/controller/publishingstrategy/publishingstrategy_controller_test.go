package publishingstrategy

import (
	"context"
	"reflect"
	"strings"
	"testing"

	operatorv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/cloud-ingress-operator/pkg/apis"
	cloudingressv1alpha1 "github.com/openshift/cloud-ingress-operator/pkg/apis/cloudingress/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func mockIngressControllerList() *operatorv1.IngressControllerList {
	return &operatorv1.IngressControllerList{
		Items: []operatorv1.IngressController{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
				Spec: operatorv1.IngressControllerSpec{
					Domain: "example-domain",
					DefaultCertificate: &corev1.LocalObjectReference{
						Name: "",
					},
				},
				Status: operatorv1.IngressControllerStatus{
					EndpointPublishingStrategy: &operatorv1.EndpointPublishingStrategy{
						Type: operatorv1.LoadBalancerServiceStrategyType,
						LoadBalancer: &operatorv1.LoadBalancerStrategy{
							Scope: operatorv1.LoadBalancerScope("Internal"),
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "non-default",
				},
				Spec: operatorv1.IngressControllerSpec{
					Domain: "example-non-default-domain",
				},
				Status: operatorv1.IngressControllerStatus{
					EndpointPublishingStrategy: &operatorv1.EndpointPublishingStrategy{
						Type: operatorv1.LoadBalancerServiceStrategyType,
						LoadBalancer: &operatorv1.LoadBalancerStrategy{
							Scope: operatorv1.LoadBalancerScope("External"),
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
				Spec: operatorv1.IngressControllerSpec{
					Domain: "example-domain-3",
					DefaultCertificate: &corev1.LocalObjectReference{
						Name: "",
					},
				},
				Status: operatorv1.IngressControllerStatus{
					EndpointPublishingStrategy: &operatorv1.EndpointPublishingStrategy{
						Type: operatorv1.LoadBalancerServiceStrategyType,
						// LoadBalancer: &operatorv1.LoadBalancerStrategy{
						// 	Scope: operatorv1.LoadBalancerScope("Internal"),
						// },
					},
				},
			},
		},
	}
}

func mockDefaultIngressController() *operatorv1.IngressController {
	return &operatorv1.IngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
		},
		Spec: operatorv1.IngressControllerSpec{
			Domain: "example-domain",
			DefaultCertificate: &corev1.LocalObjectReference{
				Name: "",
			},
		},
		Status: operatorv1.IngressControllerStatus{
			EndpointPublishingStrategy: &operatorv1.EndpointPublishingStrategy{
				Type: operatorv1.LoadBalancerServiceStrategyType,
				LoadBalancer: &operatorv1.LoadBalancerStrategy{
					Scope: operatorv1.LoadBalancerScope("Internal"),
				},
			},
		},
	}
}

func mockNonDefaultIngressController() *operatorv1.IngressController {
	return &operatorv1.IngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name: "non-default",
		},
		Spec: operatorv1.IngressControllerSpec{
			Domain: "example-nonDefault-domain",
			DefaultCertificate: &corev1.LocalObjectReference{
				Name: "",
			},
		},
		Status: operatorv1.IngressControllerStatus{
			EndpointPublishingStrategy: &operatorv1.EndpointPublishingStrategy{
				Type: operatorv1.LoadBalancerServiceStrategyType,
				LoadBalancer: &operatorv1.LoadBalancerStrategy{
					Scope: operatorv1.LoadBalancerScope("Internal"),
				},
			},
		},
	}
}

func mockApplicationIngress() *cloudingressv1alpha1.ApplicationIngress {
	return &cloudingressv1alpha1.ApplicationIngress{
		Listening: cloudingressv1alpha1.Internal,
		Default:   true,
		DNSName:   "example-domain",
		Certificate: corev1.SecretReference{
			Name: "",
		},
	}
}

func mockApplicationIngressExternal() *cloudingressv1alpha1.ApplicationIngress {
	return &cloudingressv1alpha1.ApplicationIngress{
		Listening: cloudingressv1alpha1.External,
		Default:   true,
		DNSName:   "example-domain-3",
		Certificate: corev1.SecretReference{
			Name: "",
		},
	}
}

func mockApplicationIngressNotOnCluster() *cloudingressv1alpha1.ApplicationIngress {
	return &cloudingressv1alpha1.ApplicationIngress{
		Listening: cloudingressv1alpha1.External,
		Default:   false,
		DNSName:   "example-domain-nondefault",
		Certificate: corev1.SecretReference{
			Name: "example-cert-nondefault",
		},
	}
}

func mockPublishingStrategy() *cloudingressv1alpha1.PublishingStrategy {
	return &cloudingressv1alpha1.PublishingStrategy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "testPublishingStrategy",
		},
		Spec: cloudingressv1alpha1.PublishingStrategySpec{
			DefaultAPIServerIngress: cloudingressv1alpha1.DefaultAPIServerIngress{
				Listening: cloudingressv1alpha1.External,
			},
			ApplicationIngress: []cloudingressv1alpha1.ApplicationIngress{
				{
					Listening: cloudingressv1alpha1.External,
					Default:   true,
					DNSName:   "exaple-domain-to-pass-in",
					Certificate: corev1.SecretReference{
						Name: "example-cert-default",
					},
				},
				{
					Listening: cloudingressv1alpha1.External,
					Default:   false,
					DNSName:   "apps2.exaple-nondefault-domain-to-pass-in",
					Certificate: corev1.SecretReference{
						Name: "example-nondefault-cert-default",
					},
				},
			},
		},
	}
}

// Tests the convertIngressControllerToMap function to make sure we have the correct maps
func TestConvertIngressControllerToMap(t *testing.T) {

	convert := convertIngressControllerToMap(mockIngressControllerList().Items)

	expected := map[string]operatorv1.IngressController{"example-domain": mockIngressControllerList().Items[0], "example-non-default-domain": mockIngressControllerList().Items[1], "example-domain-3": mockIngressControllerList().Items[2]}

	equal := reflect.DeepEqual(convert, expected)
	if !equal {
		t.Errorf("got %v, expect %v \n", convert, expected)
	}
}

// Tests the isOnCluster function given an applicationingress and an ingresscontroller
func TestIsOnCluster(t *testing.T) {

	onCluster := isOnCluster(mockApplicationIngress(), mockIngressControllerList().Items[0])
	if !onCluster {
		t.Logf("compare scope %s, %s", string(mockIngressControllerList().Items[0].Status.EndpointPublishingStrategy.LoadBalancer.Scope), strings.Title(strings.ToLower(string(mockApplicationIngress().Listening))))
		t.Logf("compare domain %s, %s", mockIngressControllerList().Items[0].Spec.Domain, mockApplicationIngress().DNSName)
		t.Logf("compare certificate %s, %s", mockIngressControllerList().Items[0].Spec.DefaultCertificate.Name, mockApplicationIngress().Certificate.Name)
		t.Errorf("got false but expect true")
	}

	notOnCluster := isOnCluster(mockApplicationIngress(), mockIngressControllerList().Items[1])
	if notOnCluster == true {
		t.Errorf("got true but expect false \n")
	}
}

// nil
func TestIsOnClusterNil(t *testing.T) {
	onCluster := isOnCluster(mockApplicationIngressExternal(), mockIngressControllerList().Items[2])
	if !onCluster {
		t.Logf("compare domain %s, %s", mockIngressControllerList().Items[2].Spec.Domain, mockApplicationIngressExternal().DNSName)
		t.Logf("compare certificate %s, %s", mockIngressControllerList().Items[2].Spec.DefaultCertificate.Name, mockApplicationIngressExternal().Certificate.Name)
		t.Errorf("got false but expect true")
	}
}

// Tests the checkExistingIngress function
// Given a map of existing ingresscontroller and an application ingress, if applicationingress is there expect true
// If applicationingress is not on cluster, expect false
func TestCheckExistingIngress(t *testing.T) {

	existingIngressMap := map[string]operatorv1.IngressController{"example-domain": mockIngressControllerList().Items[0], "example-non-default-domain": mockIngressControllerList().Items[1]}

	check0 := checkExistingIngress(existingIngressMap, mockApplicationIngress())
	if !check0 {
		t.Errorf("got false but expect true \n")
	}

	check1 := checkExistingIngress(existingIngressMap, mockApplicationIngressNotOnCluster())
	if check1 {
		t.Errorf("got true but expect false \n")
	}
}

func TestGetIngressName(t *testing.T) {

	domainName := "apps2.test.domain_name.org"

	expected := "apps2"
	result := getIngressName(domainName)

	if expected != result {
		t.Errorf("got %s \n, expected %s \n", result, expected)
	}
}

func TestNewApplicationIngressControllerCR(t *testing.T) {

	// expected result
	expected := &operatorv1.IngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "apps3",
			Namespace: "openshift-ingress-operator",
		},
		Spec: operatorv1.IngressControllerSpec{
			DefaultCertificate: &corev1.LocalObjectReference{
				Name: "example-cert-nondefault",
			},
			Domain: "example-domain-nondefault",
			EndpointPublishingStrategy: &operatorv1.EndpointPublishingStrategy{
				Type: operatorv1.LoadBalancerServiceStrategyType,
				LoadBalancer: &operatorv1.LoadBalancerStrategy{
					Scope: operatorv1.ExternalLoadBalancer,
				},
			},
			RouteSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{},
			},
		},
	}

	// set up function param
	listening := string(mockApplicationIngressNotOnCluster().Listening)
	domain := mockApplicationIngressNotOnCluster().DNSName
	newCertificate := &corev1.LocalObjectReference{
		Name: mockApplicationIngressNotOnCluster().Certificate.Name,
	}
	routeSelector := mockApplicationIngressNotOnCluster().RouteSelector.MatchLabels

	result, _ := newApplicationIngressControllerCR("apps3", listening, domain, newCertificate, routeSelector)

	// since these are pointers to different struct the pointer addresses are not the same, therefore reflect.DeepEqual won't work
	// compare parts that we can
	if result.Name != expected.Name && result.Spec.DefaultCertificate.Name != expected.Spec.DefaultCertificate.Name && result.Spec.Domain != expected.Spec.Domain {
		t.Errorf("expected different ingresscontroller")
	}
}

// create new fake k8s client to mock API calls
func newTestReconciler() *ReconcilePublishingStrategy {
	return &ReconcilePublishingStrategy{
		client: fake.NewFakeClient(),
		scheme: scheme.Scheme,
	}
}

// TestIngressHandle tests both the defaultIngressHandle and the nonDefaultIngressHandle functions
func TestIngressHandle(t *testing.T) {
	// set up schemes
	ctx := context.TODO()
	r := newTestReconciler()
	s := scheme.Scheme

	if err := operatorv1.AddToScheme(s); err != nil {
		t.Fatalf("Unable to add operatorv1 scheme (%v)", err)
	}

	if err := apis.AddToScheme(s); err != nil {
		t.Fatalf("Unable to add route scheme (%v)", err)
	}

	err := r.client.Create(ctx, mockDefaultIngressController())
	if err != nil {
		t.Errorf("couldn't create ingresscontroller %s", err)
	}

	err = r.client.Create(ctx, mockNonDefaultIngressController())
	if err != nil {
		t.Errorf("couldn't create ingresscontroller %s", err)
	}

	err = r.client.Create(ctx, mockPublishingStrategy())
	if err != nil {
		t.Errorf("couldn't create ingresscontroller %s", err)
	}

	list := &operatorv1.IngressControllerList{}
	opts := client.ListOptions{}

	err = r.client.List(ctx, list, &opts)
	if err != nil {
		t.Errorf("couldn't get ingresscontroller list %s", err)
	}

	t.Logf("appIngress %v", mockPublishingStrategy().Spec.ApplicationIngress[1])

	t.Logf("before list %v", list)
	// given a new defaultIngressController that does not exist on cluster
	// the result should be this new default ingresscontroller

	newCertificate := &corev1.LocalObjectReference{
		Name: "new-certificate",
	}

	err = r.defaultIngressHandle(mockPublishingStrategy().Spec.ApplicationIngress[0], list, newCertificate)
	if err != nil {
		t.Fatalf("couldn't handle default ingress")
	}

	err = r.nonDefaultIngressHandle(mockPublishingStrategy().Spec.ApplicationIngress[1], list, newCertificate)
	if err != nil {
		t.Fatalf("couldn't handle non-default ingress")
	}
}
