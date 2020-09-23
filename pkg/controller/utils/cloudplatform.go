package utils

import (
	configv1 "github.com/openshift/api/config/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetPlatformType returns the cloud platform type for the cluster
func GetPlatformType(kclient client.Client) (*configv1.PlatformType, error) {
	infra, err := getInfrastructureObject(kclient)
	if err != nil {
		return nil, err
	}
	return &infra.Status.Platform, nil
}
