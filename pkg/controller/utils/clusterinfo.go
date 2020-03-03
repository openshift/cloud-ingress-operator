package utils

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	configv1 "github.com/openshift/api/config/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	machineapi "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
	awsproviderapi "sigs.k8s.io/cluster-api-provider-aws/pkg/apis/awsproviderconfig/v1beta1"
)

const masterMachineLabel string = "machine.openshift.io/cluster-api-machine-role"
const nameFilterKey string = "tag:Name"

// GetClusterBaseDomain returns the installed cluster's base domain name
func GetClusterBaseDomain(kclient client.Client) (string, error) {
	infra, err := getInfrastructureObject(kclient)
	if err != nil {
		return "", nil
	}
	// This starts with "api." that needs to be removed.
	u, err := url.Parse(infra.Status.APIServerURL)
	if err != nil {
		return "", fmt.Errorf("Couldn't parse the cluster's URI from %s: %s", infra.Status.APIServerURL, err)
	}
	return u.Hostname()[4:], nil
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
	err := kclient.List(context.TODO(), machineList, client.InNamespace("openshift-machine-api"), client.MatchingLabels{masterMachineLabel: "master"})
	if err != nil {
		return []string{}, err
	}
	subnets := []string{}
	// only append unique subnet IDs
	dedup := make(map[string]bool)
	codec, err := awsproviderapi.NewCodec()
	if err != nil {
		return []string{}, err
	}
	for _, machineObj := range machineList.Items {
		awsconfig := &awsproviderapi.AWSMachineProviderConfig{}
		err := codec.DecodeProviderSpec(&machineObj.Spec.ProviderSpec, awsconfig)
		//		clusterConfig, err := awsproviderapi.ClusterConfigFromProviderSpec(machineObj.Spec.ProviderSpec)
		if err != nil {
			return []string{}, err
		}
		subnetName, err := getNameFromFilters(&awsconfig.Subnet.Filters)
		if err != nil {
			return []string{}, err
		}
		if !dedup[subnetName] {
			subnets = append(subnets, subnetName)
			dedup[subnetName] = true
		}
	}
	return subnets, nil
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
	listOptions := []client.ListOption{
		client.InNamespace("openshift-machine-api"),
		client.MatchingLabels{masterMachineLabel: "master"},
	}
	err := kclient.List(context.TODO(), machineList, listOptions...)
	if err != nil {
		return []string{}, err
	}

	ids := make([]string, 0)

	for _, machineObj := range machineList.Items {
		r := strings.LastIndex(*machineObj.Spec.ProviderID, "/")
		if r != -1 {
			n := *machineObj.Spec.ProviderID
			ids = append(ids, n[r+1:])
		}
	}
	return ids, nil
}

func getInfrastructureObject(kclient client.Client) (*configv1.Infrastructure, error) {
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

// getNameFromFilters will return the value of the name filter tag
func getNameFromFilters(filters *[]awsproviderapi.Filter) (string, error) {
	for _, filter := range *filters {
		if filter.Name == nameFilterKey {
			return filter.Values[0], nil
		}
	}
	return "", fmt.Errorf("Didn't find a name filter")
}
