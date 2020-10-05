package gcp

// "Private" or non-interface conforming methods

import (
	"context"
	"errors"

	cloudingressv1alpha1 "github.com/openshift/cloud-ingress-operator/pkg/apis/cloudingress/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ensureAdminAPIDNS ensures the DNS record for the "admin API" Service
// LoadBalancer is accurately set
func (c *Client) ensureAdminAPIDNS(ctx context.Context, kclient client.Client, instance *cloudingressv1alpha1.APIScheme, svc *corev1.Service) error {
	return c.ensureDNSForService(kclient, svc, instance.Spec.ManagementAPIServerIngress.DNSName)
}

// deleteAdminAPIDNS ensures the DNS record for the "admin API" Service
// LoadBalancer is deleted
func (c *Client) deleteAdminAPIDNS(ctx context.Context, kclient client.Client, instance *cloudingressv1alpha1.APIScheme, svc *corev1.Service) error {
	return c.removeDNSForService(kclient, svc, instance.Spec.ManagementAPIServerIngress.DNSName)
}

// ensureSSHDNS ensures the DNS record for the SSH Service LoadBalancer
// is accurately set
func (c *Client) ensureSSHDNS(ctx context.Context, kclient client.Client, instance *cloudingressv1alpha1.SSHD, svc *corev1.Service) error {
	return c.ensureDNSForService(kclient, svc, instance.Spec.DNSName)
}

// deleteSSHDNS ensures the DNS record for the SSH Service LoadBalancer
// is deleted
func (c *Client) deleteSSHDNS(ctx context.Context, kclient client.Client, instance *cloudingressv1alpha1.SSHD, svc *corev1.Service) error {
	return c.removeDNSForService(kclient, svc, instance.Spec.DNSName)
}

// setDefaultAPIPrivate sets the default api (api.<cluster-domain>) to private
// scope
func (c *Client) setDefaultAPIPrivate(ctx context.Context, kclient client.Client, _ *cloudingressv1alpha1.PublishingStrategy) error {
	return errors.New("setDefaultAPIPrivate is not implemented")
}

// setDefaultAPIPublic sets the default API (api.<cluster-domain>) to public
// scope
func (c *Client) setDefaultAPIPublic(ctx context.Context, kclient client.Client, instance *cloudingressv1alpha1.PublishingStrategy) error {
	return errors.New("setDefaultAPIPublic is not implemented")
}

func (c *Client) ensureDNSForService(kclient client.Client, svc *corev1.Service, dnsName string) error {
	return errors.New("ensureDNSForService is not implemented")
}

func (c *Client) removeDNSForService(kclient client.Client, svc *corev1.Service, dnsName string) error {
	return errors.New("removeDNSForService is not implemented")
}
