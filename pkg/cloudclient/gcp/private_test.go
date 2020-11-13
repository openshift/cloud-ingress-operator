package gcp

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cioerrors "github.com/openshift/cloud-ingress-operator/pkg/errors"
)

func TestGetIPAddressesFromService(t *testing.T) {
	tests := []struct {
		name         string
		svc          *corev1.Service
		expected_ips []string
		expected_err error
	}{
		{
			name: "single IP",
			svc: &corev1.Service{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Service",
					APIVersion: corev1.SchemeGroupVersion.String(),
				},
				Status: corev1.ServiceStatus{
					LoadBalancer: corev1.LoadBalancerStatus{
						Ingress: []corev1.LoadBalancerIngress{
							{
								IP: "127.0.0.1",
							},
						},
					},
				},
			},
			expected_ips: []string{
				"127.0.0.1",
			},
		},
		{
			name: "multiple IPs",
			svc: &corev1.Service{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Service",
					APIVersion: corev1.SchemeGroupVersion.String(),
				},
				Status: corev1.ServiceStatus{
					LoadBalancer: corev1.LoadBalancerStatus{
						Ingress: []corev1.LoadBalancerIngress{
							{
								IP: "127.0.0.1",
							},
							{
								IP: "10.0.0.1",
							},
						},
					},
				},
			},
			expected_ips: []string{
				"127.0.0.1",
				"10.0.0.1",
			},
		},
		{
			name: "no IPs",
			svc: &corev1.Service{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Service",
					APIVersion: corev1.SchemeGroupVersion.String(),
				},
				Status: corev1.ServiceStatus{
					LoadBalancer: corev1.LoadBalancerStatus{
						Ingress: []corev1.LoadBalancerIngress{},
					},
				},
			},
			expected_ips: nil,
			expected_err: cioerrors.NewLoadBalancerNotReadyError(),
		},
	}

	for _, test := range tests {
		actual, err := getIPAddressesFromService(test.svc)

		if !reflect.DeepEqual(actual, test.expected_ips) {
			t.Errorf("%s: expected %v, got %v", test.name, actual, test.expected_ips)
		}

		actualErrorType := reflect.TypeOf(err)
		expectErrorType := reflect.TypeOf(test.expected_err)
		if actualErrorType != expectErrorType {
			t.Errorf("%s error: expected %v, got %v", test.name, actualErrorType, expectErrorType)
		}
	}
}
