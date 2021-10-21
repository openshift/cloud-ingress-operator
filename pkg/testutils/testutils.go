package testutils

import (
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	configv1 "github.com/openshift/api/config/v1"
	cloudingressv1alpha1 "github.com/openshift/cloud-ingress-operator/pkg/apis/cloudingress/v1alpha1"

	operv1 "github.com/openshift/api/operator/v1"
	gcpproviderapi "github.com/openshift/cluster-api-provider-gcp/pkg/apis/gcpprovider/v1beta1"
	machineapi "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	awsprovider "sigs.k8s.io/cluster-api-provider-aws/pkg/apis/awsproviderconfig/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const masterMachineLabel string = "machine.openshift.io/cluster-api-machine-role"

const DefaultRegionName string = "us-east-1"
const DefaultAzName string = "us-east-1a"
const DefaultAPIEndpoint string = "https://api.unit.test:6443"
const DefaultClusterDomain string = "unit.test"

// ClusterTokenId represents the part of identifiers which is varied by the installer, eg
// clustername-clustertokenid, as in load balancer names: foo-12345-us-east-1a
const ClusterTokenId string = "12345"

type Mocks struct {
	FakeKubeClient client.Client
	MockCtrl       *gomock.Controller
	Scheme         *runtime.Scheme
}

// legacyConfig maps a full ConfigMap install-config as a sprintf template.
// Refer to CreateLegacyClusterConfig for usage
const legacyConfig string = `apiVersion: v1
baseDomain: %s
compute:
- hyperthreading: Enabled
  name: worker
  platform:
    aws:
      rootVolume:
        iops: 100
        size: 32
        type: gp2
      type: m5.xlarge
      zones:
      - %s
      - %s
      - %s
  replicas: %d
controlPlane:
  hyperthreading: Enabled1234
  name: master
  platform:
    aws:
      rootVolume:
        iops: 1000
        size: 350
        type: io1
      type: m5.xlarge
      zones:
      - %s
      - %s
      - %s
  replicas: %d
metadata:
  creationTimestamp: null
  name: %s
networking:
  clusterNetwork:
  - cidr: 10.128.0.0/14
    hostPrefix: 23
  machineCIDR: 10.0.0.0/16
  networkType: OpenShiftSDN
  serviceNetwork:
  - 172.30.0.0/16
platform:
  aws:
    region: %s
pullSecret: ""
sshKey: |
  ssh-rsa nothingreal
`

// NewMockTest sets up for a new mock test, pass in some localObjs to seed the fake Kubernetes environment
func NewTestMock(t *testing.T, localObjs []runtime.Object) *Mocks {
	mockctrl := gomock.NewController(t)
	s := scheme.Scheme
	if err := configv1.AddToScheme(s); err != nil {
		t.Fatalf("Couldn't add configv1 scheme: (%v)", err)
	}
	if err := machineapi.AddToScheme(s); err != nil {
		t.Fatalf("Couldn't add machine scheme: (%v)", err)
	}
	if err := cloudingressv1alpha1.SchemeBuilder.AddToScheme(s); err != nil {
		t.Fatalf("Couldn't add cloudingressv1alpha1 scheme: (%v)", err)
	}
	if err := operv1.AddToScheme(s); err != nil {
		t.Fatalf("Unable to add route scheme: (%v)", err)
	}
	ret := &Mocks{
		FakeKubeClient: fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(localObjs...).Build(),
		MockCtrl:       mockctrl,
		Scheme:         s,
	}

	return ret
}

// CreateAPIServerObject creates a configv1.APIServer object
func CreateAPIServerObject(clustername, clusterdomain string) *configv1.APIServer {
	ret := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: configv1.APIServerSpec{
			ClientCA: configv1.ConfigMapNameReference{
				Name: "",
			},
			ServingCerts: configv1.APIServerServingCerts{
				NamedCertificates: []configv1.APIServerNamedServingCert{
					{
						Names: []string{fmt.Sprintf("api.%s", clusterdomain)},
						ServingCertificate: configv1.SecretNameReference{
							Name: fmt.Sprintf("%s-primary-certbundle-secret", clustername),
						},
					},
				},
			},
		},
	}
	return ret
}

// CreateMachineObjectList makes a MachineList from the slice of names, and returns also a slice of Machine objects for convenience
func CreateMachineObjectList(name []string, clusterid, role, region, zone string) (*machineapi.MachineList, []machineapi.Machine) {
	machines := make([]machineapi.Machine, 0)
	for _, n := range name {
		machines = append(machines, CreateMachineObj(n, clusterid, role, region, zone))
	}
	ret := &machineapi.MachineList{
		Items: machines,
	}
	return ret, machines
}

// CreateMachineObj makes a single AWS-style machinev1beta1.Machine object
func CreateMachineObj(name, clusterid, role, region, zone string) machineapi.Machine {
	ami := "ami-123456"
	provider := &awsprovider.AWSMachineProviderConfig{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "awsproviderconfig.openshift.io/v1beta1",
			Kind:       "AWSMachineProviderConfig",
		},
		InstanceType:       "small",
		BlockDevices:       []awsprovider.BlockDeviceMappingSpec{},
		AMI:                awsprovider.AWSResourceReference{ID: &ami},
		Tags:               []awsprovider.TagSpecification{{Name: fmt.Sprintf("kubernetes.io/cluster/%s", clusterid), Value: "owned"}},
		IAMInstanceProfile: &awsprovider.AWSResourceReference{ID: pointer.StringPtr(fmt.Sprintf("%s-%s-profile", clusterid, role))},
		UserDataSecret:     &corev1.LocalObjectReference{Name: "aws-cloud-credentials"},
		Placement:          awsprovider.Placement{Region: region, AvailabilityZone: zone},
		LoadBalancers: []awsprovider.LoadBalancerReference{
			{
				// <clustername>-<id>-ext
				Name: fmt.Sprintf("%s-%s-ext", clusterid, ClusterTokenId),
				Type: awsprovider.NetworkLoadBalancerType,
			},
			{
				// <clustername>-<id>-int
				Name: fmt.Sprintf("%s-%s-int", clusterid, ClusterTokenId),
				Type: awsprovider.NetworkLoadBalancerType,
			},
		},
		SecurityGroups: []awsprovider.AWSResourceReference{{
			Filters: []awsprovider.Filter{{
				Name:   "tag:Name",
				Values: []string{fmt.Sprintf("%s-%s-sg", clusterid, role)},
			}},
		}},
	}
	provider.Subnet.Filters = []awsprovider.Filter{{
		Name: "tag:Name",
		Values: []string{
			fmt.Sprintf("%s-private-%s", clusterid, zone),
			fmt.Sprintf("%s-public-%s", clusterid, zone),
		},
	}}
	labels := make(map[string]string)
	labels[masterMachineLabel] = role
	ret := machineapi.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "openshift-machine-api",
			Labels:    labels,
		},
		Spec: machineapi.MachineSpec{
			ProviderSpec: machineapi.ProviderSpec{
				Value: &runtime.RawExtension{Object: provider},
			},
			// not exactly the same as AWS
			ProviderID: pointer.StringPtr(fmt.Sprintf("aws:///%s/i-%s", zone, name)),
		},
	}
	return ret
}

// CreateGCPMachineObj makes a single AWS-style machinev1beta1.Machine object
func CreateGCPMachineObj(name, clusterid, role, region, zone string) machineapi.Machine {
	projectID := "o-1234567"
	provider := &gcpproviderapi.GCPMachineProviderSpec{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "gcpprovider.openshift.io/v1beta1",
			Kind:       "GCPMachineProviderSpec",
		},
		CanIPForward:       false,
		DeletionProtection: false,
		Metadata:           []*gcpproviderapi.GCPMetadata{},
		NetworkInterfaces:  []*gcpproviderapi.GCPNetworkInterface{},
		MachineType:        "custom-1-2345",
		Disks:              []*gcpproviderapi.GCPDisk{},
		ServiceAccounts:    []gcpproviderapi.GCPServiceAccount{},
		Tags:               []string{clusterid + "-master"},
		UserDataSecret:     &corev1.LocalObjectReference{Name: "gcp-cloud-credentials"},
		Region:             region,
		Zone:               zone,
		ProjectID:          projectID,
		TargetPools:        []string{clusterid + "-api"},
	}
	labels := make(map[string]string)
	labels[masterMachineLabel] = role
	ret := machineapi.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "openshift-machine-api",
			Labels:    labels,
		},
		Spec: machineapi.MachineSpec{
			ProviderSpec: machineapi.ProviderSpec{
				Value: &runtime.RawExtension{Object: provider},
			},
			ProviderID: pointer.StringPtr(fmt.Sprintf("gce:///%s/%s/%s", projectID, zone, name)),
		},
	}
	return ret
}

// CreateGCPMachineObjectList makes a MachineList from the slice of names, and returns also a slice of Machine objects for convenience
func CreateGCPMachineObjectList(name []string, clusterid, role, region, zone string) (*machineapi.MachineList, []machineapi.Machine) {
	machines := make([]machineapi.Machine, 0)
	for _, n := range name {
		machines = append(machines, CreateGCPMachineObj(n, clusterid, role, region, zone))
	}
	ret := &machineapi.MachineList{
		Items: machines,
	}
	return ret, machines
}

// CreateLegacyClusterConfig creates kube-config/configmaps/cluster-config-v1
// To test https://bugzilla.redhat.com/show_bug.cgi?id=1814332
func CreateLegacyClusterConfig(clusterdomain, infraName, region string, workerCount, masterCount int) *corev1.ConfigMap {
	yamlConfig := fmt.Sprintf(legacyConfig, clusterdomain, DefaultAzName, DefaultAzName, DefaultAzName, workerCount,
		DefaultAzName, DefaultAzName, DefaultAzName, masterCount, infraName, region)
	fmt.Printf("Sprintf'd YAML Config = %s\n", yamlConfig)
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "kube-system",
			Name:      "cluster-config-v1",
		},
		Data: map[string]string{
			"install-config": yamlConfig,
		},
	}
}

// CreatOldInfraObject creates an Infrastructure object that is missing information
// eg for https://bugzilla.redhat.com/show_bug.cgi?id=1814332
func CreatOldInfraObject(infraName, apiInternalURL, apiURL, region string) *configv1.Infrastructure {
	return &configv1.Infrastructure{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster",
			Namespace: "",
		},
		Spec: configv1.InfrastructureSpec{
			CloudConfig: configv1.ConfigMapFileReference{
				Name: "",
			},
		},
		Status: configv1.InfrastructureStatus{
			InfrastructureName:   infraName,
			APIServerInternalURL: apiInternalURL,
			APIServerURL:         apiURL,
			Platform:             configv1.AWSPlatformType,
			// Note: Absent PlatformStatus is intentional
		},
	}
}

// CreateInfraObject creates an configv1.Infrastructure object
func CreateInfraObject(infraName, apiInternalURL, apiURL, region string) *configv1.Infrastructure {
	return &configv1.Infrastructure{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster",
			Namespace: "",
		},
		Spec: configv1.InfrastructureSpec{
			CloudConfig: configv1.ConfigMapFileReference{
				Name: "",
			},
		},
		Status: configv1.InfrastructureStatus{
			InfrastructureName:   infraName,
			APIServerInternalURL: apiInternalURL,
			APIServerURL:         apiURL,
			Platform:             configv1.AWSPlatformType,
			PlatformStatus: &configv1.PlatformStatus{
				Type: configv1.AWSPlatformType,
				AWS: &configv1.AWSPlatformStatus{
					Region: region,
				},
			},
		},
	}
}

// CreateGCPInfraObject creates an configv1.Infrastructure object
func CreateGCPInfraObject(infraName, apiInternalURL, apiURL, region string) *configv1.Infrastructure {
	return &configv1.Infrastructure{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster",
			Namespace: "",
		},
		Spec: configv1.InfrastructureSpec{
			CloudConfig: configv1.ConfigMapFileReference{
				Name: "",
			},
		},
		Status: configv1.InfrastructureStatus{
			InfrastructureName:   infraName,
			APIServerInternalURL: apiInternalURL,
			APIServerURL:         apiURL,
			Platform:             configv1.GCPPlatformType,
			PlatformStatus: &configv1.PlatformStatus{
				Type: configv1.GCPPlatformType,
				GCP: &configv1.GCPPlatformStatus{
					Region: region,
				},
			},
		},
	}
}

// CreateAPISchemeObject makes an APISCheme object
func CreateAPISchemeObject(dnsname string, enabled bool, cidrs []string) *cloudingressv1alpha1.APIScheme {
	return &cloudingressv1alpha1.APIScheme{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rh-api",
			Namespace: "openshift-cloud-ingress-operator",
		},
		Spec: cloudingressv1alpha1.APISchemeSpec{
			ManagementAPIServerIngress: cloudingressv1alpha1.ManagementAPIServerIngress{
				Enabled:           enabled,
				DNSName:           dnsname,
				AllowedCIDRBlocks: cidrs,
			},
		},
	}
}

// ValidateMachineLB returns length, names and types (slices) and any error if one
// The purpose is to have an easy way to condense 12+ lines of code
func ValidateMachineLB(m *machineapi.Machine) (int, []string, []awsprovider.AWSLoadBalancerType, error) {
	names := make([]string, 0)
	lbTypes := make([]awsprovider.AWSLoadBalancerType, 0)
	l := 0
	codec, err := awsprovider.NewCodec()
	if err != nil {
		return l, names, lbTypes, err
	}
	awsconfig := &awsprovider.AWSMachineProviderConfig{}
	err = codec.DecodeProviderSpec(&m.Spec.ProviderSpec, awsconfig)
	if err != nil {
		return l, names, lbTypes, err
	}
	for _, lb := range awsconfig.LoadBalancers {
		names = append(names, lb.Name)
		lbTypes = append(lbTypes, lb.Type)
		l++
	}
	return l, names, lbTypes, nil
}
