package cloudclient

import (
	"context"
	"fmt"

	configv1 "github.com/openshift/api/config/v1"

	cloudingressv1alpha1 "github.com/openshift/cloud-ingress-operator/pkg/apis/cloudingress/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CloudClient defines the interface for a cloud agnostic implementation
type CloudClient interface {

	/* APIScheme */
	// EnsureAdminAPIDNS ensures there's a rh-api (for example) alias to the Service for the APIScheme
	// May return loadBalancerNotFound or other specific errors
	EnsureAdminAPIDNS(context.Context, client.Client, *cloudingressv1alpha1.APIScheme, *corev1.Service) error

	// DeleteAdminAPIDNS will ensure that the A record for the admin API (rh-api) is removed
	DeleteAdminAPIDNS(context.Context, client.Client, *cloudingressv1alpha1.APIScheme, *corev1.Service) error

	/* SSH */
	// EnsureSSHDNS ensures there's a rh-ssh (for example) alias to the Service for the SSH pod
	EnsureSSHDNS(context.Context, client.Client, *cloudingressv1alpha1.SSHD, *corev1.Service) error

	// DeleteSSHDNS will ensure that the A record for the SSH pod (rh-ssh) is removed
	DeleteSSHDNS(context.Context, client.Client, *cloudingressv1alpha1.SSHD, *corev1.Service) error

	/* Publishing Strategy */
	// SetDefaultAPIPrivate ensures that the default API is private, per user configure
	SetDefaultAPIPrivate(context.Context, client.Client, *cloudingressv1alpha1.PublishingStrategy) error

	// SetDefaultAPIPublic ensures that the default API is public, per user configure
	SetDefaultAPIPublic(context.Context, client.Client, *cloudingressv1alpha1.PublishingStrategy) error
}

var controllerMapping = map[configv1.PlatformType]Factory{}

type Factory func(client.Client) CloudClient

func Register(name configv1.PlatformType, factoryFunc Factory) {
	controllerMapping[name] = factoryFunc
}

// GetClientFor returns the CloudClient for the given cloud provider, identified
// by the provider's ID, eg aws for AWS's cloud client, gcp for GCP's cloud
// client.
func GetClientFor(kclient client.Client, cloudID configv1.PlatformType) CloudClient {
	if _, ok := controllerMapping[cloudID]; ok {
		return controllerMapping[cloudID](kclient)
	}
	// TODO: Return a minimal interface?
	panic(fmt.Sprintf("Couldn't find a client matching %s", cloudID))
}
