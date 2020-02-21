package utils

import (
	"context"
	"fmt"
	"net/url"
	"regexp"

	configv1 "github.com/openshift/api/config/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	// machineapi and awsprovider are tied
	awsproviderapi "sigs.k8s.io/cluster-api-provider-aws/pkg/apis/awsprovider/v1alpha1"
	machineapi "sigs.k8s.io/cluster-api/pkg/apis/deprecated/v1alpha1"
)

const masterMachineLabel string = "machine.openshift.io/cluster-api-machine-role"
const instanceRegex string = `^aws:\/\/\/.*\/(.*)$`

// GetClusterBaseDomain returns the installed cluster's base domain name
func GetClusterBaseDomain(kclient client.Client) (string, error) {
	infra, err := getInfrastructureObject(kclient)
	if err != nil {
		return "", nil
	}
	u, err := url.Parse(infra.Status.APIServerURL)
	if err != nil {
		return "", fmt.Errorf("Couldn't parse the cluster's URI from %s: %s", infra.Status.APIServerURL, err)
	}
	return u.Hostname(), nil
}

// GetClusterPlatform will return the installed cluster's platform type
func GetClusterPlatform(kclient client.Client) (string, error) {
	infra, err := getInfrastructureObject(kclient)
	if err != nil {
		return "", nil
	}
	return string(infra.Status.Platform), nil
}

// GetClusterName returns the installed cluster's name (max 27 characters)
func GetClusterName(kclient client.Client) (string, error) {
	infra, err := getInfrastructureObject(kclient)
	if err != nil {
		return "", nil
	}
	return infra.Status.InfrastructureName, nil
}

// GetMasterNodeSubnets returns all the subnets for Machines with 'master' label.
// TODO: Validate the return here are AWS identifiers.
func GetMasterNodeSubnets(kclient client.Client) ([]string, error) {
	machineList := &machineapi.MachineList{}
	s := map[string]string{masterMachineLabel: "master"}

	subnets := []string{}

	err := kclient.List(context.TODO(), machineList, &client.ListOptions{LabelSelector: labels.SelectorFromSet(s)})
	if err != nil {
		return subnets, err
	}

	// only append unique subnet IDs
	dedup := make(map[string]bool)
	for _, machineObj := range machineList.Items {
		clusterConfig, err := awsproviderapi.ClusterConfigFromProviderSpec(machineObj.Spec.ProviderSpec)
		if err != nil {
			return subnets, err
		}
		for _, subnet := range clusterConfig.NetworkSpec.Subnets {
			if !dedup[subnet.ID] {
				subnets = append(subnets, subnet.ID)
				dedup[subnet.ID] = true
			}
		}
	}
	return subnets, nil
}

// GetMasterNodeVPCs returns all the VPCs for Machines with 'master' label.
// TODO: Validate the return here are AWS identifiers.
func GetMasterNodeVPCs(kclient client.Client) ([]string, error) {
	machineList := &machineapi.MachineList{}
	s := map[string]string{masterMachineLabel: "master"}

	vpcs := []string{}

	err := kclient.List(context.TODO(), machineList, &client.ListOptions{LabelSelector: labels.SelectorFromSet(s)})
	if err != nil {
		return vpcs, err
	}

	// only append unique subnet IDs
	dedup := make(map[string]bool)
	for _, machineObj := range machineList.Items {
		clusterConfig, err := awsproviderapi.ClusterConfigFromProviderSpec(machineObj.Spec.ProviderSpec)
		if err != nil {
			return vpcs, err
		}

		if !dedup[clusterConfig.NetworkSpec.VPC.ID] {
			vpcs = append(vpcs, clusterConfig.NetworkSpec.VPC.ID)
			dedup[clusterConfig.NetworkSpec.VPC.ID] = true
		}
	}
	return vpcs, nil
}

// GetClusterRegion returns the installed cluster's AWS region
func GetClusterRegion(kclient client.Client) (string, error) {
	infra, err := getInfrastructureObject(kclient)
	if err != nil {
		return "", nil
	} else if infra.Status.PlatformStatus == nil {
		return "", fmt.Errorf("Expected to have a PlatformStatus for Infrastructure/cluster, but it was nil")
	}
	return infra.Status.PlatformStatus.AWS.Region, nil
}

// GetClusterMasterInstances gets all the instance IDs for Master nodes
// For AWS the form is aws:///<availability zone>/<instance ID>
// This could come from parsing the arbitrarily formatted .Status.ProviderStatus
// but .Spec.ProviderID is standard
func GetClusterMasterInstances(kclient client.Client) ([]string, error) {
	machineList := &machineapi.MachineList{}
	s := map[string]string{masterMachineLabel: "master"}

	err := kclient.List(context.TODO(), machineList, &client.ListOptions{LabelSelector: labels.SelectorFromSet(s)})
	if err != nil {
		return []string{}, err
	}

	ids := make([]string, 0)
	matcher := regexp.MustCompile(instanceRegex)
	for _, machine := range machineList.Items {

		r := matcher.FindString(*machine.Spec.ProviderID)
		if r != "" {
			ids = append(ids, r)
		}
	}
	return ids, nil
}

func getInfrastructureObject(kclient client.Client) (*configv1.Infrastructure, error) {
	infra := &configv1.Infrastructure{}
	ns := types.NamespacedName{
		Namespace: "",
		Name:      "",
	}
	err := kclient.Get(context.TODO(), ns, infra)
	if err != nil {
		return nil, err
	}
	return infra, nil
}

// AWSOwnerTag returns owner taglist for the cluster
func AWSOwnerTag(kclient client.Client) (map[string]string, error) {
	m := make(map[string]string)
	name, err := GetClusterName(kclient)
	if err != nil {
		return m, err
	}

	m[fmt.Sprintf("kubernetes.io/cluster/%s", name)] = "owned"
	return m, nil
}
