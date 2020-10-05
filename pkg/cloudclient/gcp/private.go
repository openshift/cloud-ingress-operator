package gcp

// "Private" or non-interface conforming methods

import (
	"context"
	"errors"

	cloudingressv1alpha1 "github.com/openshift/cloud-ingress-operator/pkg/apis/cloudingress/v1alpha1"
	//utils "github.com/openshift/cloud-ingress-operator/pkg/controller/utils"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ensureAdminAPIDNS ensure the DNS record for the rh-api "admin API" for
// APIScheme is present and mapped to the corresponding Service's AWS
// LoadBalancer
func (c *Client) ensureAdminAPIDNS(ctx context.Context, kclient client.Client, instance *cloudingressv1alpha1.APIScheme, svc *corev1.Service) error {
	return c.ensureDNSForService(ctx, kclient, svc, instance.Spec.ManagementAPIServerIngress.DNSName, "RH API Endpoint")
}

// deleteAdminAPIDNS removes the DNS record for the rh-api "admin API" for
// APIScheme
func (c *Client) deleteAdminAPIDNS(ctx context.Context, kclient client.Client, instance *cloudingressv1alpha1.APIScheme, svc *corev1.Service) error {
	return c.removeDNSForService(ctx, kclient, svc, instance.Spec.ManagementAPIServerIngress.DNSName, "RH API Endpoint")
}

// ensureSSHDNS ensures the DNS record for the SSH Service LoadBalancer is set
func (c *Client) ensureSSHDNS(ctx context.Context, kclient client.Client, instance *cloudingressv1alpha1.SSHD, svc *corev1.Service) error {
	return c.ensureDNSForService(ctx, kclient, svc, instance.Spec.DNSName, "RH SSH Endpoint")
}

// deleteSSHDNS ensures the DNS record for the SSH Service AWS LoadBalancer is unset
func (c *Client) deleteSSHDNS(ctx context.Context, kclient client.Client, instance *cloudingressv1alpha1.SSHD, svc *corev1.Service) error {
	return c.removeDNSForService(ctx, kclient, svc, instance.Spec.DNSName, "RH SSH Endpoint")
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

func (c *Client) ensureDNSForService(ctx context.Context, kclient client.Client, svc *corev1.Service, dnsName, dnsComment string) error {
	return errors.New("ensureDNSForService is not implemented")
}

func (c *Client) removeDNSForService(ctx context.Context, kclient client.Client, svc *corev1.Service, dnsName, dnsComment string) error {
	return errors.New("removeDNSForService is not implemented")
}
