package gcp

import (
	"context"
	"fmt"

	"golang.org/x/oauth2/google"
	computev1 "google.golang.org/api/compute/v1"
	dnsv1 "google.golang.org/api/dns/v1"
	"google.golang.org/api/option"

	configv1 "github.com/openshift/api/config/v1"
	cloudingressv1alpha1 "github.com/openshift/cloud-ingress-operator/pkg/apis/cloudingress/v1alpha1"
	"github.com/openshift/cloud-ingress-operator/pkg/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// ClientIdentifier is what kind of cloud this implement supports
const ClientIdentifier configv1.PlatformType = configv1.GCPPlatformType

var (
	log = logf.Log.WithName("gcp_cloudclient")
)

// Client represents a GCP Client
type Client struct {
	projectID      string
	dnsService     *dnsv1.Service
	computeService *computev1.Service
}

// EnsureAdminAPIDNS implements cloudclient.CloudClient
func (c *Client) EnsureAdminAPIDNS(ctx context.Context, kclient client.Client, instance *cloudingressv1alpha1.APIScheme, svc *corev1.Service) error {
	return c.ensureAdminAPIDNS(ctx, kclient, instance, svc)
}

// DeleteAdminAPIDNS implements cloudclient.CloudClient
func (c *Client) DeleteAdminAPIDNS(ctx context.Context, kclient client.Client, instance *cloudingressv1alpha1.APIScheme, svc *corev1.Service) error {
	return c.deleteAdminAPIDNS(ctx, kclient, instance, svc)
}

// EnsureSSHDNS implements cloudclient.CloudClient
func (c *Client) EnsureSSHDNS(ctx context.Context, kclient client.Client, instance *cloudingressv1alpha1.SSHD, svc *corev1.Service) error {
	return c.ensureSSHDNS(ctx, kclient, instance, svc)
}

// DeleteSSHDNS implements cloudclient.CloudClient
func (c *Client) DeleteSSHDNS(ctx context.Context, kclient client.Client, instance *cloudingressv1alpha1.SSHD, svc *corev1.Service) error {
	return c.deleteSSHDNS(ctx, kclient, instance, svc)
}

// SetDefaultAPIPrivate implements cloudclient.CloudClient
func (c *Client) SetDefaultAPIPrivate(ctx context.Context, kclient client.Client, instance *cloudingressv1alpha1.PublishingStrategy) error {
	return c.setDefaultAPIPrivate(ctx, kclient, instance)
}

// SetDefaultAPIPublic implements cloudclient.CloudClient
func (c *Client) SetDefaultAPIPublic(ctx context.Context, kclient client.Client, instance *cloudingressv1alpha1.PublishingStrategy) error {
	return c.setDefaultAPIPublic(ctx, kclient, instance)
}

func newClient(ctx context.Context, serviceAccountJSON []byte) (*Client, error) {
	credentials, err := google.CredentialsFromJSON(
		ctx, serviceAccountJSON,
		dnsv1.NdevClouddnsReadwriteScope,
		computev1.ComputeScope)
	if err != nil {
		return nil, err
	}

	dnsService, err := dnsv1.NewService(ctx, option.WithCredentials(credentials))
	if err != nil {
		return nil, err
	}

	computeService, err := computev1.NewService(ctx, option.WithCredentials(credentials))
	if err != nil {
		return nil, err
	}

	return &Client{
		projectID:      credentials.ProjectID,
		dnsService:     dnsService,
		computeService: computeService,
	}, nil
}

// NewClient creates a new CloudClient for use with GCP.
func NewClient(kclient client.Client) *Client {
	ctx := context.Background()
	secret := &corev1.Secret{}
	err := kclient.Get(
		ctx,
		types.NamespacedName{
			Name:      config.GCPSecretName,
			Namespace: config.OperatorNamespace,
		},
		secret)
	if err != nil {
		panic(fmt.Sprintf("Couldn't get Secret with credentials %s", err.Error()))
	}
	serviceAccountJSON, ok := secret.Data["service_account.json"]
	if !ok {
		panic(fmt.Sprintf("Access credentials missing service account"))
	}

	c, err := newClient(ctx, serviceAccountJSON)

	if err != nil {
		panic(fmt.Sprintf("Couldn't create GCP client %s", err.Error()))
	}

	return c
}
