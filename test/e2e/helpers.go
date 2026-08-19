//go:build osde2e
// +build osde2e

package osde2etests

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	openshiftapi "github.com/openshift/api"
	cloudingressv1alpha1 "github.com/openshift/cloud-ingress-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// e2eClient wraps controller-runtime's client.Client with convenience methods
// that match the openshift.Client API from osde2e-common, so existing test code
// needs minimal changes. This replaces the osde2e-common dependency entirely.
type e2eClient struct {
	client.Client
	config *rest.Config
	log    logr.Logger
}

func newE2EClient(log logr.Logger) (*e2eClient, error) {
	cfg, err := loadKubeConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}
	return newE2EClientFromConfig(cfg, log)
}

func newE2EClientFromConfig(cfg *rest.Config, log logr.Logger) (*e2eClient, error) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to register core/v1: %w", err)
	}
	if err := openshiftapi.Install(scheme); err != nil {
		return nil, fmt.Errorf("failed to register openshift api: %w", err)
	}
	if err := cloudingressv1alpha1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to register cloudingressv1alpha1: %w", err)
	}
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create controller-runtime client: %w", err)
	}
	return &e2eClient{Client: c, config: cfg, log: log}, nil
}

// Get wraps client.Get with positional name/namespace args for API compatibility
// with the old osde2e-common openshift.Client.
func (c *e2eClient) Get(ctx context.Context, name, namespace string, obj client.Object) error {
	return c.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, obj)
}

func (c *e2eClient) GetScheme() *runtime.Scheme {
	return c.Client.Scheme()
}

func (c *e2eClient) GetConfig() *rest.Config {
	return c.config
}

// Impersonate returns a new client acting as the given user and groups.
func (c *e2eClient) Impersonate(user string, groups ...string) (*e2eClient, error) {
	if user != "" {
		groups = append(groups, "system:authenticated", "system:authenticated:oauth")
	}
	impersonatedCfg := rest.CopyConfig(c.config)
	impersonatedCfg.Impersonate = rest.ImpersonationConfig{UserName: user, Groups: groups}
	return newE2EClientFromConfig(impersonatedCfg, c.log)
}

const (
	metadataConfigMap = "osd-cluster-metadata"
	configNamespace   = "openshift-config"
)

func (c *e2eClient) getClusterMetadata(ctx context.Context) (map[string]string, error) {
	var cm corev1.ConfigMap
	if err := c.Get(ctx, metadataConfigMap, configNamespace, &cm); err != nil {
		return nil, err
	}
	return cm.Data, nil
}

func (c *e2eClient) IsSTS(ctx context.Context) (bool, error) {
	data, err := c.getClusterMetadata(ctx)
	if err != nil {
		return false, err
	}
	return data["api.openshift.com_sts"] == "true", nil
}

func (c *e2eClient) GetProvider(ctx context.Context) (string, error) {
	data, err := c.getClusterMetadata(ctx)
	if err != nil {
		return "", err
	}
	return data["hive.openshift.io_cluster-platform"], nil
}

func (c *e2eClient) GetRegion(ctx context.Context) (string, error) {
	data, err := c.getClusterMetadata(ctx)
	if err != nil {
		return "", err
	}
	return data["hive.openshift.io_cluster-region"], nil
}

func loadKubeConfig() (*rest.Config, error) {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		kubeconfig = filepath.Join(home, ".kube", "config")
	}
	return clientcmd.BuildConfigFromFlags("", kubeconfig)
}
