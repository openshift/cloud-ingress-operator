package testutils

import (
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	configv1 "github.com/openshift/api/config/v1"
	cloudingressv1alpha1 "github.com/openshift/cloud-ingress-operator/api/v1alpha1"
	"github.com/openshift/cloud-ingress-operator/pkg/ingresscontroller"

	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const masterMachineLabel string = "machine.openshift.io/cluster-api-machine-role"

const DefaultRegionName string = "us-east-1"
const DefaultAzName string = "us-east-1a"
const DefaultAPIEndpoint string = "https://api.unit.test:6443"
const DefaultClusterDomain string = "unit.test"
const DefaultAMIID string = "ami-123456"

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
`

// NewMockTest sets up for a new mock test, pass in some localObjs to seed the fake Kubernetes environment
func NewTestMock(t *testing.T, localObjs []runtime.Object) *Mocks {
	mockctrl := gomock.NewController(t)
	s := scheme.Scheme
	if err := configv1.Install(s); err != nil {
		t.Fatalf("Couldn't add configv1 scheme: (%v)", err)
	}
	if err := machinev1beta1.Install(s); err != nil {
		t.Fatalf("Couldn't add machine scheme: (%v)", err)
	}
	if err := cloudingressv1alpha1.SchemeBuilder.AddToScheme(s); err != nil {
		t.Fatalf("Couldn't add cloudingressv1alpha1 scheme: (%v)", err)
	}
	if err := ingresscontroller.AddToScheme(s); err != nil {
		t.Fatalf("Unable to add route scheme: (%v)", err)
	}
	s.AddKnownTypes(machinev1beta1.SchemeGroupVersion,
		&machinev1beta1.AWSMachineProviderConfig{},
	)
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
func CreateMachineObjectList(name []string, clusterid, role, region, zone string) (*machinev1beta1.MachineList, []machinev1beta1.Machine) {
	machines := make([]machinev1beta1.Machine, 0)
	for _, n := range name {
		machines = append(machines, CreateMachineObjPre411(n, clusterid, role, region, zone))
	}
	ret := &machinev1beta1.MachineList{
		Items: machines,
	}
	return ret, machines
}

// CreateMachineObjPre411 makes a single AWS-style machinev1beta1.Machine object with a
// AWSMachineProviderConfig with a GVK from pre-4.11
func CreateMachineObjPre411(name, clusterid, role, region, zone string) machinev1beta1.Machine {
	ami := string(DefaultAMIID)
	provider := &machinev1beta1.AWSMachineProviderConfig{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "machine.openshift.io/v1beta1",
			Kind:       "AWSMachineProviderConfig",
		},
		InstanceType:       "small",
		BlockDevices:       []machinev1beta1.BlockDeviceMappingSpec{},
		AMI:                machinev1beta1.AWSResourceReference{ID: &ami},
		Tags:               []machinev1beta1.TagSpecification{{Name: fmt.Sprintf("kubernetes.io/cluster/%s", clusterid), Value: "owned"}},
		IAMInstanceProfile: &machinev1beta1.AWSResourceReference{ID: ptr.To(fmt.Sprintf("%s-%s-profile", clusterid, role))},
		UserDataSecret:     &corev1.LocalObjectReference{Name: "aws-cloud-credentials"},
		Placement:          machinev1beta1.Placement{Region: region, AvailabilityZone: zone},
		LoadBalancers: []machinev1beta1.LoadBalancerReference{
			{
				// <clustername>-<id>-ext
				Name: fmt.Sprintf("%s-%s-ext", clusterid, ClusterTokenId),
				Type: machinev1beta1.NetworkLoadBalancerType,
			},
			{
				// <clustername>-<id>-int
				Name: fmt.Sprintf("%s-%s-int", clusterid, ClusterTokenId),
				Type: machinev1beta1.NetworkLoadBalancerType,
			},
		},
		SecurityGroups: []machinev1beta1.AWSResourceReference{{
			Filters: []machinev1beta1.Filter{{
				Name:   "tag:Name",
				Values: []string{fmt.Sprintf("%s-%s-sg", clusterid, role)},
			}},
		}},
	}
	provider.Subnet.Filters = []machinev1beta1.Filter{{
		Name: "tag:Name",
		Values: []string{
			fmt.Sprintf("%s-private-%s", clusterid, zone),
			fmt.Sprintf("%s-public-%s", clusterid, zone),
		},
	}}
	labels := make(map[string]string)
	labels[masterMachineLabel] = role
	ret := machinev1beta1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "openshift-machine-api",
			Labels:    labels,
		},
		Spec: machinev1beta1.MachineSpec{
			ProviderSpec: machinev1beta1.ProviderSpec{
				Value: &runtime.RawExtension{Object: provider},
			},
			// not exactly the same as AWS
			ProviderID: ptr.To(fmt.Sprintf("aws:///%s/i-%s", zone, name)),
		},
	}
	return ret
}

// CreateMachineObj411 makes a single AWS-style machinev1beta1.Machine object with a
// AWSMachineProviderConfig with a GVK from 4.11+
func CreateMachineObj411(name, clusterid, role, region, zone string) machinev1beta1.Machine {
	ami := "ami-123456"
	provider := &machinev1beta1.AWSMachineProviderConfig{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "machine.openshift.io/v1beta1",
			Kind:       "AWSMachineProviderConfig",
		},
		InstanceType:       "small",
		BlockDevices:       []machinev1beta1.BlockDeviceMappingSpec{},
		AMI:                machinev1beta1.AWSResourceReference{ID: &ami},
		Tags:               []machinev1beta1.TagSpecification{{Name: fmt.Sprintf("kubernetes.io/cluster/%s", clusterid), Value: "owned"}},
		IAMInstanceProfile: &machinev1beta1.AWSResourceReference{ID: ptr.To(fmt.Sprintf("%s-%s-profile", clusterid, role))},
		UserDataSecret:     &corev1.LocalObjectReference{Name: "aws-cloud-credentials"},
		Placement:          machinev1beta1.Placement{Region: region, AvailabilityZone: zone},
		LoadBalancers: []machinev1beta1.LoadBalancerReference{
			{
				// <clustername>-<id>-ext
				Name: fmt.Sprintf("%s-%s-ext", clusterid, ClusterTokenId),
				Type: machinev1beta1.NetworkLoadBalancerType,
			},
			{
				// <clustername>-<id>-int
				Name: fmt.Sprintf("%s-%s-int", clusterid, ClusterTokenId),
				Type: machinev1beta1.NetworkLoadBalancerType,
			},
		},
		SecurityGroups: []machinev1beta1.AWSResourceReference{{
			Filters: []machinev1beta1.Filter{{
				Name:   "tag:Name",
				Values: []string{fmt.Sprintf("%s-%s-sg", clusterid, role)},
			}},
		}},
	}
	provider.Subnet.Filters = []machinev1beta1.Filter{{
		Name: "tag:Name",
		Values: []string{
			fmt.Sprintf("%s-private-%s", clusterid, zone),
			fmt.Sprintf("%s-public-%s", clusterid, zone),
		},
	}}
	labels := make(map[string]string)
	labels[masterMachineLabel] = role
	ret := machinev1beta1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "openshift-machine-api",
			Labels:    labels,
		},
		Spec: machinev1beta1.MachineSpec{
			ProviderSpec: machinev1beta1.ProviderSpec{
				Value: &runtime.RawExtension{Object: provider},
			},
			// not exactly the same as AWS
			ProviderID: ptr.To(fmt.Sprintf("aws:///%s/i-%s", zone, name)),
		},
	}
	return ret
}

// CreateGCPMachineObjPre411 makes a single AWS-style machinev1beta1.Machine object with a
// GCPMachineProviderConfig with a GVK from pre-4.11
func CreateGCPMachineObjPre411(name, clusterid, role, region, zone string) machinev1beta1.Machine {
	projectID := "o-1234567"
	provider := &machinev1beta1.GCPMachineProviderSpec{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "gcpprovider.openshift.io/v1beta1",
			Kind:       "GCPMachineProviderSpec",
		},
		CanIPForward:       false,
		DeletionProtection: false,
		Metadata:           []*machinev1beta1.GCPMetadata{},
		NetworkInterfaces:  []*machinev1beta1.GCPNetworkInterface{},
		MachineType:        "custom-1-2345",
		Disks:              []*machinev1beta1.GCPDisk{},
		ServiceAccounts:    []machinev1beta1.GCPServiceAccount{},
		Tags:               []string{clusterid + "-master"},
		UserDataSecret:     &corev1.LocalObjectReference{Name: "gcp-cloud-credentials"},
		Region:             region,
		Zone:               zone,
		ProjectID:          projectID,
		TargetPools:        []string{clusterid + "-api"},
	}
	labels := make(map[string]string)
	labels[masterMachineLabel] = role
	ret := machinev1beta1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "openshift-machine-api",
			Labels:    labels,
		},
		Spec: machinev1beta1.MachineSpec{
			ProviderSpec: machinev1beta1.ProviderSpec{
				Value: &runtime.RawExtension{Object: provider},
			},
			ProviderID: ptr.To(fmt.Sprintf("gce:///%s/%s/%s", projectID, zone, name)),
		},
	}
	return ret
}

// CreateGCPMachineObj411 makes a single AWS-style machinev1beta1.Machine object with a
// GCPMachineProviderConfig with a GVK from 4.11+
func CreateGCPMachineObj411(name, clusterid, role, region, zone string) machinev1beta1.Machine {
	projectID := "o-1234567"
	provider := &machinev1beta1.GCPMachineProviderSpec{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "machine.openshift.io/v1beta1",
			Kind:       "GCPMachineProviderSpec",
		},
		CanIPForward:       false,
		DeletionProtection: false,
		Metadata:           []*machinev1beta1.GCPMetadata{},
		NetworkInterfaces:  []*machinev1beta1.GCPNetworkInterface{},
		MachineType:        "custom-1-2345",
		Disks:              []*machinev1beta1.GCPDisk{},
		ServiceAccounts:    []machinev1beta1.GCPServiceAccount{},
		Tags:               []string{clusterid + "-master"},
		UserDataSecret:     &corev1.LocalObjectReference{Name: "gcp-cloud-credentials"},
		Region:             region,
		Zone:               zone,
		ProjectID:          projectID,
		TargetPools:        []string{clusterid + "-api"},
	}
	labels := make(map[string]string)
	labels[masterMachineLabel] = role
	ret := machinev1beta1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "openshift-machine-api",
			Labels:    labels,
		},
		Spec: machinev1beta1.MachineSpec{
			ProviderSpec: machinev1beta1.ProviderSpec{
				Value: &runtime.RawExtension{Object: provider},
			},
			ProviderID: ptr.To(fmt.Sprintf("gce:///%s/%s/%s", projectID, zone, name)),
		},
	}
	return ret
}

// CreateGCPMachineObjectList makes a MachineList from the slice of names, and returns also a slice of Machine objects for convenience
func CreateGCPMachineObjectList(name []string, clusterid, role, region, zone string) (*machinev1beta1.MachineList, []machinev1beta1.Machine) {
	machines := make([]machinev1beta1.Machine, 0)
	for _, n := range name {
		machines = append(machines, CreateGCPMachineObjPre411(n, clusterid, role, region, zone))
	}
	ret := &machinev1beta1.MachineList{
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
func ValidateMachineLB(spec *machinev1beta1.AWSMachineProviderConfig) (int, []string, []machinev1beta1.AWSLoadBalancerType, error) {
	names := make([]string, 0)
	lbTypes := make([]machinev1beta1.AWSLoadBalancerType, 0)
	l := 0
	for _, lb := range spec.LoadBalancers {
		names = append(names, lb.Name)
		lbTypes = append(lbTypes, lb.Type)
		l++
	}
	return l, names, lbTypes, nil
}
