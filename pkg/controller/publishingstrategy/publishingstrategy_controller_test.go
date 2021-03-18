package publishingstrategy

import (
	"testing"

	operatorv1 "github.com/openshift/api/operator/v1"
	cloudingressv1alpha1 "github.com/openshift/cloud-ingress-operator/pkg/apis/cloudingress/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	expected := &operatorv1.IngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "apps3",
			Namespace: "openshift-ingress-operator",
		},
		Spec: operatorv1.IngressControllerSpec{
			DefaultCertificate: &corev1.LocalObjectReference{
				Name: "example-cert-nondefault",
			},
			Domain: "example-domain-nondefault.example.com",
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
	actualIngressController1 := &operatorv1.IngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: "openshift-ingress-operator",
		},
		Spec: operatorv1.IngressControllerSpec{
			Replicas: &replicas,
		},
		Status: operatorv1.IngressControllerStatus{
			Domain: "example.com",
			EndpointPublishingStrategy: &operatorv1.EndpointPublishingStrategy{
				Type: operatorv1.LoadBalancerServiceStrategyType,
				LoadBalancer: &operatorv1.LoadBalancerStrategy{
					Scope: operatorv1.ExternalLoadBalancer,
				},
			},
		},
	}

	result := validateStaticStatus(*actualIngressController1, desiredIngressController.Spec)

	if result == true {
		t.Errorf("Expected IngressController and desired config to be different")
	}

	// Build "actual" IngressController that should pass
	actualIngressController2 := &operatorv1.IngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: "openshift-ingress-operator",
		},
		Spec: operatorv1.IngressControllerSpec{
			Replicas: &replicas,
		},
		Status: operatorv1.IngressControllerStatus{
			Domain: "example.com",
			EndpointPublishingStrategy: &operatorv1.EndpointPublishingStrategy{
				Type: operatorv1.LoadBalancerServiceStrategyType,
				LoadBalancer: &operatorv1.LoadBalancerStrategy{
					Scope: operatorv1.InternalLoadBalancer,
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
	actualIngressController1 := &operatorv1.IngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "apps3",
			Namespace: "openshift-ingress-operator",
		},
		Spec: operatorv1.IngressControllerSpec{
			DefaultCertificate: &corev1.LocalObjectReference{
				Name: "example-cert-nondefault",
			},
			Domain: "example-domain.example.com",
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

	result := validateStaticSpec(*actualIngressController1, desiredIngressController.Spec)

	if result == true {
		t.Errorf("Expected IngressController and desired config to be different")
	}

	// Build "actual" IngressController that should pass
	actualIngressController2 := &operatorv1.IngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "apps3",
			Namespace: "openshift-ingress-operator",
		},
		Spec: operatorv1.IngressControllerSpec{
			DefaultCertificate: &corev1.LocalObjectReference{
				Name: "example-cert-nondefault",
			},
			Domain: "example-domain-nondefault.example.com",
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
	actualIngressController1 := &operatorv1.IngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "apps3",
			Namespace: "openshift-ingress-operator",
		},
		Spec: operatorv1.IngressControllerSpec{
			DefaultCertificate: &corev1.LocalObjectReference{
				Name: "example-cert-nondefault",
			},
			NodePlacement: &operatorv1.NodePlacement{
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

	result, field := validatePatchableSpec(*actualIngressController1, desiredIngressController.Spec)

	if result == true {
		t.Errorf("Expected IngressController and desired config to be different")
	} else if field != IngressControllerSelector {
		t.Errorf("Expected IngressController and desired config to have different RouteSelectors different")
	}

	// Build "actual" IngressController that should pass
	actualIngressController2 := &operatorv1.IngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "apps3",
			Namespace: "openshift-ingress-operator",
		},
		Spec: operatorv1.IngressControllerSpec{
			DefaultCertificate: &corev1.LocalObjectReference{
				Name: "example-cert-nondefault",
			},
			NodePlacement: &operatorv1.NodePlacement{
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
			EndpointPublishingStrategy: &operatorv1.EndpointPublishingStrategy{
				Type: operatorv1.LoadBalancerServiceStrategyType,
				LoadBalancer: &operatorv1.LoadBalancerStrategy{
					Scope: operatorv1.ExternalLoadBalancer,
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
	actualIngressController1 := &operatorv1.IngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "apps3",
			Namespace: "openshift-ingress-operator",
		},
		Spec: operatorv1.IngressControllerSpec{
			DefaultCertificate: &corev1.LocalObjectReference{
				Name: "example-cert-nondefault",
			},
			Domain: "example-domain.example.com",
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

	result, field := validatePatchableStatus(*actualIngressController1, desiredIngressController.Spec)

	if result == true {
		t.Errorf("Expected IngressController and desired config to be different")
	} else if field != IngressControllerSelector {
		t.Errorf("Expected IngressController and desired config to have different RouteSelectors different")
	}

	// Build "actual" IngressController that should pass
	actualIngressController2 := &operatorv1.IngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "apps3",
			Namespace: "openshift-ingress-operator",
		},
		Spec: operatorv1.IngressControllerSpec{
			DefaultCertificate: &corev1.LocalObjectReference{
				Name: "example-cert-nondefault",
			},
			Domain: "example-domain.example.com",
			EndpointPublishingStrategy: &operatorv1.EndpointPublishingStrategy{
				Type: operatorv1.LoadBalancerServiceStrategyType,
				LoadBalancer: &operatorv1.LoadBalancerStrategy{
					Scope: operatorv1.ExternalLoadBalancer,
				},
			},
		},
		Status: operatorv1.IngressControllerStatus{
			Selector: "foo=bar",
		},
	}

	result2, _ := validatePatchableStatus(*actualIngressController2, desiredIngressController.Spec)

	if result2 == false {
		t.Errorf("Expected IngressController and desired config to be the same %+v\n %+v\n", actualIngressController2.Status.Selector, desiredIngressController.Spec.RouteSelector.MatchLabels)
	}
}
