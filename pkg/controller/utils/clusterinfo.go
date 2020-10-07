package utils

import (
	"context"
	"fmt"
	"net/url"

	configv1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	machineapi "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
	awsproviderapi "sigs.k8s.io/cluster-api-provider-aws/pkg/apis/awsproviderconfig/v1beta1"

	"sigs.k8s.io/yaml"
)

// installConfig represents the bare minimum requirement to get the AWS cluster region from the install-config
// See https://bugzilla.redhat.com/show_bug.cgi?id=1814332
type installConfig struct {
	Platform struct {
		AWS struct {
			Region string `json:"region"`
		} `json:"aws"`
	} `json:"platform"`
}

const masterMachineLabel string = "machine.openshift.io/cluster-api-machine-role"

// GetClusterBaseDomain returns the installed cluster's base domain name
func GetClusterBaseDomain(kclient client.Client) (string, error) {
	infra, err := getInfrastructureObject(kclient)
	if err != nil {
		return "", err
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
		return "", err
	}
	return string(infra.Status.Platform), nil
}

// GetClusterName returns the installed cluster's name (max 27 characters)
func GetClusterName(kclient client.Client) (string, error) {
	infra, err := getInfrastructureObject(kclient)
	if err != nil {
		return "", err
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
	subnets := make(map[string]string)
	machineList, err := GetMasterMachines(kclient)
	if err != nil {
		return subnets, err
	}
	if len(machineList.Items) == 0 {
		return subnets, fmt.Errorf("Did not find any master Machine objects")
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
		return "", err
	} else if infra.Status.PlatformStatus == nil {
		// Try the deprecated configmap. See https://bugzilla.redhat.com/show_bug.cgi?id=1814332
		return readClusterRegionFromConfigMap(kclient)
	}
	return infra.Status.PlatformStatus.AWS.Region, nil
}

// GetMasterNodes returns a machineList object whose .Items can be iterated
// over to perform actions on/with information from each master machine object
func GetMasterMachines(kclient client.Client) (*machineapi.MachineList, error) {
	machineList := &machineapi.MachineList{}
	listOptions := []client.ListOption{
		client.InNamespace("openshift-machine-api"),
		client.MatchingLabels{masterMachineLabel: "master"},
	}
	err := kclient.List(context.TODO(), machineList, listOptions...)
	if err != nil {
		return nil, err
	}
	return machineList, nil
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

func readClusterRegionFromConfigMap(kclient client.Client) (string, error) {
	cm, err := getClusterConfigMap(kclient)
	if err != nil {
		return "", err
	}
	return parseClusterRegionFromConfigMap(cm)
}

func getClusterConfigMap(kclient client.Client) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{}
	ns := types.NamespacedName{
		Namespace: "kube-system",
		Name:      "cluster-config-v1",
	}
	err := kclient.Get(context.TODO(), ns, cm)
	return cm, err
}

func parseClusterRegionFromConfigMap(cm *corev1.ConfigMap) (string, error) {
	if cm == nil || cm.Data == nil {
		return "", fmt.Errorf("unexpected nil configmap or nil configmap Data")
	}
	data, ok := cm.Data["install-config"]
	if !ok {
		return "", fmt.Errorf("Missing install-config in configmap")
	}
	var ic installConfig
	if err := yaml.Unmarshal([]byte(data), &ic); err != nil {
		return "", fmt.Errorf("Invalid install-config: %v\njson:%s", err, data)
	}
	return ic.Platform.AWS.Region, nil
}
