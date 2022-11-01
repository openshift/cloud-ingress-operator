package gcp

import (
	"context"
	"fmt"

	"golang.org/x/oauth2/google"
	computev1 "google.golang.org/api/compute/v1"
	dnsv1 "google.golang.org/api/dns/v1"
	"google.golang.org/api/option"

	configv1 "github.com/openshift/api/config/v1"
	machineapi "github.com/openshift/api/machine/v1beta1"
	cloudingressv1alpha1 "github.com/openshift/cloud-ingress-operator/api/v1alpha1"
	"github.com/openshift/cloud-ingress-operator/config"
	baseutils "github.com/openshift/cloud-ingress-operator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	k8s "sigs.k8s.io/controller-runtime/pkg/client"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// ClientIdentifier is what kind of cloud this implement supports
const ClientIdentifier configv1.PlatformType = configv1.GCPPlatformType

var (
	log = logf.Log.WithName("gcp_cloudclient")
)

// Client represents a GCP cloud Client
type Client struct {
	projectID      string
	region         string
	clusterName    string
	baseDomain     string
	masterList     *machineapi.MachineList
	dnsService     *dnsv1.Service
	computeService *computev1.Service
}

// EnsureAdminAPIDNS implements cloudclient.CloudClient
func (gc *Client) EnsureAdminAPIDNS(ctx context.Context, kclient k8s.Client, instance *cloudingressv1alpha1.APIScheme, svc *corev1.Service) error {
	return gc.ensureAdminAPIDNS(ctx, kclient, instance, svc)
}

// DeleteAdminAPIDNS implements cloudclient.CloudClient
func (gc *Client) DeleteAdminAPIDNS(ctx context.Context, kclient k8s.Client, instance *cloudingressv1alpha1.APIScheme, svc *corev1.Service) error {
	return gc.deleteAdminAPIDNS(ctx, kclient, instance, svc)
}

// SetDefaultAPIPrivate implements cloudclient.CloudClient
func (gc *Client) SetDefaultAPIPrivate(ctx context.Context, kclient k8s.Client, instance *cloudingressv1alpha1.PublishingStrategy) error {
	return gc.setDefaultAPIPrivate(ctx, kclient, instance)
}

// SetDefaultAPIPublic implements cloudclient.CloudClient
func (gc *Client) SetDefaultAPIPublic(ctx context.Context, kclient k8s.Client, instance *cloudingressv1alpha1.PublishingStrategy) error {
	return gc.setDefaultAPIPublic(ctx, kclient, instance)
}

// Healthcheck performs basic calls to make sure client is healthy
func (gc *Client) Healthcheck(ctx context.Context, kclient k8s.Client) error {
	_, err := gc.computeService.RegionBackendServices.List(gc.projectID, gc.region).Do()
	return err
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
func NewClient(kclient k8s.Client) (*Client, error) {
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

func getClusterRegion(kclient k8s.Client) (string, error) {
	infra, err := baseutils.GetInfrastructureObject(kclient)
	if err != nil {
		return "", err
	}
	return infra.Status.PlatformStatus.GCP.Region, nil
}
