// This package is for low-level utility functions used by both controllers
// and CloudClient interface implementations.
package utils

import (
	"context"
	"fmt"
	"net/url"

	configv1 "github.com/openshift/api/config/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetInfrastructureObject returns the canonical Infrastructure object
func GetInfrastructureObject(kclient client.Client) (*configv1.Infrastructure, error) {
	infra := &configv1.Infrastructure{}
	ns := types.NamespacedName{
		Namespace: "",
		Name:      "cluster",
	}
	err := kclient.Get(context.TODO(), ns, infra)
	if err != nil {
		return nil, err
	}
	return infra, nil
}

// GetClusterBaseDomain returns the installed clsuter's base domain name
func GetClusterBaseDomain(kclient client.Client) (string, error) {
	infra, err := GetInfrastructureObject(kclient)
	if err != nil {
		return "", err
	}
	serverURL, err := url.Parse(infra.Status.APIServerURL)
	if err != nil {
		return "", fmt.Errorf("Couldn't parse the API server URL from %s: %s", infra.Status.APIServerURL, err)
	}
	// Trim the leading "api." from the hostname.
	return serverURL.Hostname()[4:], nil
}

// GetClusterName returns the installed cluster's name (max 27 characters)
func GetClusterName(kclient client.Client) (string, error) {
	infra, err := GetInfrastructureObject(kclient)
	if err != nil {
		return "", err
	}
	return infra.Status.InfrastructureName, nil
}

// GetPlatformType returns the cloud platform type for the cluster
func GetPlatformType(kclient client.Client) (*configv1.PlatformType, error) {
	infra, err := GetInfrastructureObject(kclient)
	if err != nil {
		return nil, err
	}
	return &infra.Status.Platform, nil
}
