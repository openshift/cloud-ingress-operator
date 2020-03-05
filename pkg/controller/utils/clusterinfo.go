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
// return structure:
// {
//   public => subnetname,
//   private => subnetname,
// }
//
func GetMasterNodeSubnets(kclient client.Client) (map[string]string, error) {
	machineList := &machineapi.MachineList{}
	subnets := make(map[string]string)
	err := kclient.List(context.TODO(), machineList, client.InNamespace("openshift-machine-api"), client.MatchingLabels{masterMachineLabel: "master"})
	if err != nil {
		return subnets, err
	}

	// get the AZ from a Master object's providerSpec.
	codec, err := awsproviderapi.NewCodec()

	if err != nil {
		return subnets, err
	}

	// Obtain the availability zone
	awsconfig := &awsproviderapi.AWSMachineProviderConfig{}
	err = codec.DecodeProviderSpec(&machineList.Items[0].Spec.ProviderSpec, awsconfig)
	if err != nil {
		return subnets, err
	}

	// Infra object gives us the Infrastructure name, which is the combination of
	// cluster name and identifier.
	infra, err := getInfrastructureObject(kclient)
	if err != nil {
		return subnets, err
	}
	subnets["public"] = fmt.Sprintf("%s-public-%s", infra.Status.InfrastructureName, awsconfig.Placement.AvailabilityZone)
	subnets["private"] = fmt.Sprintf("%s-private-%s", infra.Status.InfrastructureName, awsconfig.Placement.AvailabilityZone)

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

// GetClusterMasterInstancesIDs gets all the instance IDs for Master nodes
// For AWS the form is aws:///<availability zone>/<instance ID>
// This could come from parsing the arbitrarily formatted .Status.ProviderStatus
// but .Spec.ProviderID is standard
func GetClusterMasterInstancesIDs(kclient client.Client) ([]string, error) {
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

// MasterInstance used to fill out TargetDescription
// when calling registerTargetInput
type MasterInstance struct {
	AvailabilityZone string
	IPaddress        string
}

// GetMasterInstancesAZsandIPs gets all the master instances AZs and IPs
func GetMasterInstancesAZsandIPs(kclient client.Client) ([]MasterInstance, error) {
	machineList := &machineapi.MachineList{}
	listOptions := []client.ListOption{
		client.InNamespace("openshift-machine-api"),
		client.MatchingLabels{masterMachineLabel: "master"},
	}
	err := kclient.List(context.TODO(), machineList, listOptions...)
	if err != nil {
		return []MasterInstance{}, err
	}

	masterInstances := make([]MasterInstance, 0)
	for _, mi := range machineList.Items {
		// get the AZ from a Master object's providerSpec.
		codec, err := awsproviderapi.NewCodec()
		if err != nil {
			return masterInstances, err
		}
		awsconfig := &awsproviderapi.AWSMachineProviderConfig{}
		err = codec.DecodeProviderSpec(&machineList.Items[0].Spec.ProviderSpec, awsconfig)
		if err != nil {
			return masterInstances, err
		}
		az := awsconfig.Placement.AvailabilityZone

		// get the IP address from Master object's status
		ip := mi.Status.Addresses[0].Address

		masterInstances = append(masterInstances, MasterInstance{
			AvailabilityZone: az,
			IPaddress:        ip,
		})
	}
	return masterInstances, nil
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
