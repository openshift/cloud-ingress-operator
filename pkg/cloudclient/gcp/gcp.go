package gcp

import (
	"context"
	"fmt"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/cloud-ingress-operator/config"
	cloudingressv1alpha1 "github.com/openshift/cloud-ingress-operator/pkg/apis/cloudingress/v1alpha1"
	baseutils "github.com/openshift/cloud-ingress-operator/pkg/utils"
	machineapi "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
	"golang.org/x/oauth2/google"
	computev1 "google.golang.org/api/compute/v1"
	dnsv1 "google.golang.org/api/dns/v1"
	"google.golang.org/api/option"
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

// GCPClient represents a GCP cloud Client
type GCPClient struct {
	projectID      string
	region         string
	clusterName    string
	baseDomain     string
	masterList     *machineapi.MachineList
	dnsService     *dnsv1.Service
	computeService *computev1.Service
}

// EnsureAdminAPIDNS implements cloudclient.CloudClient
func (c *GCPClient) EnsureAdminAPIDNS(ctx context.Context, kclient client.Client, instance *cloudingressv1alpha1.APIScheme, svc *corev1.Service) error {
	return c.ensureAdminAPIDNS(ctx, kclient, instance, svc)
}

// DeleteAdminAPIDNS implements cloudclient.CloudClient
func (c *GCPClient) DeleteAdminAPIDNS(ctx context.Context, kclient client.Client, instance *cloudingressv1alpha1.APIScheme, svc *corev1.Service) error {
	return c.deleteAdminAPIDNS(ctx, kclient, instance, svc)
}

// EnsureSSHDNS implements cloudclient.CloudClient
func (c *GCPClient) EnsureSSHDNS(ctx context.Context, kclient client.Client, instance *cloudingressv1alpha1.SSHD, svc *corev1.Service) error {
	return c.ensureSSHDNS(ctx, kclient, instance, svc)
}

// DeleteSSHDNS implements cloudclient.CloudClient
func (c *GCPClient) DeleteSSHDNS(ctx context.Context, kclient client.Client, instance *cloudingressv1alpha1.SSHD, svc *corev1.Service) error {
	return c.deleteSSHDNS(ctx, kclient, instance, svc)
}

// SetDefaultAPIPrivate implements cloudclient.CloudClient
func (c *GCPClient) SetDefaultAPIPrivate(ctx context.Context, kclient client.Client, instance *cloudingressv1alpha1.PublishingStrategy) error {
	return c.setDefaultAPIPrivate(ctx, kclient, instance)
}

// SetDefaultAPIPublic implements cloudclient.CloudClient
func (c *GCPClient) SetDefaultAPIPublic(ctx context.Context, kclient client.Client, instance *cloudingressv1alpha1.PublishingStrategy) error {
	return c.setDefaultAPIPublic(ctx, kclient, instance)
}

// Healthcheck performs basic calls to make sure client is healthy
func (c *GCPClient) Healthcheck(ctx context.Context, kclient client.Client) error {
	out, err := c.computeService.RegionBackendServices.List(c.projectID, c.region).Do()
	if err != nil {
		return err // possible client deformation
	}

	return performHealthCheck(out, c.clusterName)
}

func performHealthCheck(l *computev1.BackendServiceList, clusterName string) error {
	// checking internal lb to ensure it's there and available to use by cloud-client
	intLBName := clusterName + "-api-internal"
	for _, lb := range l.Items {
		if lb.Name == intLBName {
			return nil
		}
	}

	return fmt.Errorf("internal lb not found: exiting to refresh")
}

func newClient(ctx context.Context, serviceAccountJSON []byte) (*GCPClient, error) {
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

	return &GCPClient{
		projectID:      credentials.ProjectID,
		dnsService:     dnsService,
		computeService: computeService,
	}, nil
}

// NewClient creates a new CloudClient for use with GCP.
func NewClient(kclient client.Client) (*GCPClient, error) {
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
		return nil, fmt.Errorf("couldn't get Secret with credentials %w", err)
	}
	serviceAccountJSON, ok := secret.Data["service_account.json"]
	if !ok {
		return nil, fmt.Errorf("access credentials missing service account")
	}

	// initialize actual client
	c, err := newClient(ctx, serviceAccountJSON)
	if err != nil {
		return nil, fmt.Errorf("couldn't create GCP client %s", err)
	}

	// enchant the client with params required
	region, err := getClusterRegion(kclient)
	if err != nil {
		return nil, err
	}
	c.region = region

	masterList, err := baseutils.GetMasterMachines(kclient)
	if err != nil {
		return nil, err
	}
	c.masterList = masterList
	infrastructureName, err := baseutils.GetClusterName(kclient)
	if err != nil {
		return nil, err
	}
	c.clusterName = infrastructureName
	baseDomain, err := baseutils.GetClusterBaseDomain(kclient)
	if err != nil {
		return nil, err
	}
	c.baseDomain = baseDomain

	return c, nil
}

func getClusterRegion(kclient client.Client) (string, error) {
	infra, err := baseutils.GetInfrastructureObject(kclient)
	if err != nil {
		return "", err
	}
	return infra.Status.PlatformStatus.GCP.Region, nil
}
