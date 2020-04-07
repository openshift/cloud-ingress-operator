package apischeme

import (
	"context"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	configv1 "github.com/openshift/api/config/v1"
	cloudingressv1alpha1 "github.com/openshift/cloud-ingress-operator/pkg/apis/cloudingress/v1alpha1"

	awsclient "github.com/openshift/cloud-ingress-operator/pkg/awsclient/mock"

	utils "github.com/openshift/cloud-ingress-operator/pkg/controller/utils"
	machineapi "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	awsprovider "sigs.k8s.io/cluster-api-provider-aws/pkg/apis/awsproviderconfig/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const masterMachineLabel string = "machine.openshift.io/cluster-api-machine-role"
const defaultRegionName string = "us-east-1"
const defaultAzName string = "us-east-1a"
const defaultAPIEndpoint string = "https://api.unit.test:6443"
const defaultClusterDomain string = "unit.test"

type mocks struct {
	fakeKubeClient client.Client
	mockCtrl       *gomock.Controller
	mockAws        *awsclient.MockClient
	scheme         *runtime.Scheme
}

// setupTests will make sure each test has the same starting point
func setupTests(t *testing.T, localObjs []runtime.Object) *mocks {
	mockctrl := gomock.NewController(t)
	s := scheme.Scheme
	if err := cloudingressv1alpha1.SchemeBuilder.AddToScheme(s); err != nil {
		t.Fatalf("Couldn't add APIScheme scheme: (%v)", err)
	}
	if err := configv1.AddToScheme(s); err != nil {
		t.Fatalf("Couldn't add configv1 scheme: (%v)", err)
	}
	if err := machineapi.AddToScheme(s); err != nil {
		t.Fatalf("Couldn't add machine scheme: (%v)", err)
	}

	ret := &mocks{
		fakeKubeClient: fake.NewFakeClientWithScheme(s, localObjs...),
		mockCtrl:       mockctrl,
		mockAws:        awsclient.NewMockClient(mockctrl),
		scheme:         s,
	}

	return ret
}

func createAPIServerObject(clustername, clusterdomain string) *configv1.APIServer {
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

func createMachineObjectList(name []string, clusterid, role, region, zone string) (*machineapi.MachineList, []machineapi.Machine) {
	machines := make([]machineapi.Machine, 0)
	for _, n := range name {
		machines = append(machines, createMachineObj(n, clusterid, role, region, zone))
	}
	ret := &machineapi.MachineList{
		Items: machines,
	}
	return ret, machines
}

// create a machineObj, largely from openshift/installer
func createMachineObj(name, clusterid, role, region, zone string) machineapi.Machine {
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

func infraObj(infraName, apiInternalURL, apiURL, region string) *configv1.Infrastructure {
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
func makeAPISchemeObj(dnsname string, enabled bool, cidrs []string) *cloudingressv1alpha1.APIScheme {
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
func TestMasterNodeSubnets(t *testing.T) {
	tests := []struct {
		region       string
		az           string
		clustername  string
		cidrs        []string
		endpointName string
		masterCount  int
		apiEndpoint  string

		mockSubnetIDInput   []string // name to id lookup input
		mockSubnetIDOutput0 []string // name to id lookup output
		mockSubnetIDOutput1 error    // name to id lookup error
	}{
		{
			region:       defaultRegionName,
			az:           defaultAzName,
			clustername:  "test1",
			cidrs:        []string{"0.0.0.0/0"},
			endpointName: "rh-api",
			masterCount:  3,
			apiEndpoint:  defaultAPIEndpoint,

			mockSubnetIDInput:   []string{"test1-public-" + defaultAzName},
			mockSubnetIDOutput0: []string{"subnet-12345"},
			mockSubnetIDOutput1: nil,
		},
		{
			region:       defaultRegionName,
			az:           defaultAzName,
			clustername:  "test2",
			cidrs:        []string{"0.0.0.0/0"},
			endpointName: "rh-api",
			masterCount:  3,
			apiEndpoint:  defaultAPIEndpoint,

			mockSubnetIDInput:   []string{"test2-public-" + defaultAzName},
			mockSubnetIDOutput0: []string{"subnet-abcdef"},
			mockSubnetIDOutput1: nil,
		},
	}

	for testNumber, test := range tests {
		aObj := makeAPISchemeObj(test.endpointName, true, test.cidrs)
		masterNames := make([]string, test.masterCount)
		for i := 0; i < test.masterCount; i++ {
			masterNames[i] = fmt.Sprintf("master-%d", i)
		}
		machineList, _ := createMachineObjectList(masterNames, test.clustername, "master", test.region, test.az)
		infraObj := infraObj(test.clustername, test.apiEndpoint, test.apiEndpoint, test.region)
		objs := []runtime.Object{aObj, infraObj, machineList}
		mocks := setupTests(t, objs)

		expectedPublicSubnetName := fmt.Sprintf("%s-public-%s", test.clustername, test.az)
		res, err := utils.GetMasterNodeSubnets(mocks.fakeKubeClient)
		if err != nil {
			t.Fatalf("Test %d Couldn't get the master node subnets: %v", testNumber, err)
		}
		if res["public"] != expectedPublicSubnetName {
			t.Fatalf("Subnet mismatch. Got %s, expected %s", res["public"], expectedPublicSubnetName)
		}
		s := []string{res["public"]}
		// now safe to mock the name -> id lookup
		mocks.mockAws.EXPECT().SubnetNameToSubnetIDLookup(test.mockSubnetIDInput).AnyTimes().Return(test.mockSubnetIDOutput0, test.mockSubnetIDOutput1)

		subnets, err := mocks.mockAws.SubnetNameToSubnetIDLookup(s)
		if err != nil {
			t.Fatalf("Couldn't lookup subnet IDs from name: %v", err)
		}

		if len(subnets) != len(test.mockSubnetIDInput) {
			t.Fatalf("Test %d Wrong number of subnets back. Got %d, expected %d", testNumber, len(subnets), 1)
		}
		for i := range subnets {
			if subnets[i] != test.mockSubnetIDOutput0[i] {
				t.Fatalf("Wrong subnet ID back for public. Got %s, expected %s", subnets[i], test.mockSubnetIDOutput0[i])
			}
		}
	}
}

func TestClusterBaseDomain(t *testing.T) {
	aObj := makeAPISchemeObj("rh-api", true, []string{"0.0.0.0/0"})
	masterNames := make([]string, 3)
	for i := 0; i < 3; i++ {
		masterNames[i] = fmt.Sprintf("master-%d", i)
	}
	machineList, _ := createMachineObjectList(masterNames, "basename", "master", defaultRegionName, defaultAzName)
	infraObj := infraObj("basename", defaultAPIEndpoint, defaultAPIEndpoint, defaultRegionName)
	objs := []runtime.Object{aObj, infraObj, machineList}
	mocks := setupTests(t, objs)

	base, err := utils.GetClusterBaseDomain(mocks.fakeKubeClient)
	if err != nil {
		t.Fatalf("Could not get cluster base domain name: %v", err)
	}
	if base != "unit.test" {
		t.Fatalf("Base domain mismatch. Expected %s, got %s", "unit.test", base)
	}
}

func TestMasterInstanceIds(t *testing.T) {
	aObj := makeAPISchemeObj("rh-api", true, []string{"0.0.0.0/0"})
	masterNames := make([]string, 3)
	for i := 0; i < 3; i++ {
		masterNames[i] = fmt.Sprintf("master-%d", i)
	}
	machineList, _ := createMachineObjectList(masterNames, "ids", "master", defaultRegionName, defaultAzName)
	infraObj := infraObj("ids", defaultAPIEndpoint, defaultAPIEndpoint, defaultRegionName)
	objs := []runtime.Object{aObj, infraObj, machineList}
	mocks := setupTests(t, objs)

	ids, err := utils.GetClusterMasterInstancesIDs(mocks.fakeKubeClient)
	if err != nil {
		t.Fatalf("Could not get cluster base domain name: %v", err)
	}
	if len(ids) != len(masterNames) {
		t.Fatalf("Master Instance ID length mismatch. Expected %d, got %d", len(masterNames), len(ids))
	}
	for _, id := range ids {
		if id[0] != 'i' {
			t.Fatalf("Expected AWS instance ID to begin with i, got %s", string(id[0]))
		}
	}
}

// TestAddAdminToAPIServerObject tests one of the two possible ways to add
// information to the APIServer object. In the controller proper, we use only
// "option 1" ("option 2" is briefly documented in the controller, with option
// 1's notes)
func TestAddAdminToAPIServerObject(t *testing.T) {
	aObj := makeAPISchemeObj("rh-api", true, []string{"0.0.0.0/0"})
	masterNames := make([]string, 3)
	for i := 0; i < 3; i++ {
		masterNames[i] = fmt.Sprintf("master-%d", i)
	}
	apiserver := createAPIServerObject("apiservertest", defaultClusterDomain)
	machineList, _ := createMachineObjectList(masterNames, "ids", "master", defaultRegionName, defaultAzName)
	infraObj := infraObj("ids", defaultAPIEndpoint, defaultAPIEndpoint, defaultRegionName)
	objs := []runtime.Object{aObj, infraObj, machineList, apiserver}
	mocks := setupTests(t, objs)
	r := &ReconcileAPIScheme{client: mocks.fakeKubeClient, scheme: mocks.scheme}
	domain, err := utils.GetClusterBaseDomain(mocks.fakeKubeClient)
	if err != nil {
		t.Fatalf("Couldn't get cluster base domain for test %v", err)
	}
	err = r.addAdminAPIToAPIServerObject(domain, aObj)
	if err != nil {
		t.Fatalf("Couldn't update APIServer object %v", err)
	}
	api := &configv1.APIServer{}
	ns := types.NamespacedName{
		Namespace: "",
		Name:      "cluster",
	}
	err = r.client.Get(context.TODO(), ns, api)
	if err != nil {
		t.Fatalf("Couldn't re-read the APIServer object %v", err)
	}
	expected := fmt.Sprintf("%s.%s", aObj.Spec.ManagementAPIServerIngress.DNSName, domain)
	match := false
	for _, cert := range api.Spec.ServingCerts.NamedCertificates {
		for _, name := range cert.Names {
			if name == expected {
				match = true
			}
		}
	}
	if !match {
		t.Fatalf("Never detected cert %s in named certs %+v", expected, api.Spec.ServingCerts.NamedCertificates)
	}
}

/*
 * Note: Having issues with this test where the Mock aws client isn't working
 * with the assignment, as Go claims it isn't implementing the entire Client
 * interface. Commented this out til it can be explored more.
func TestCreateApiScheme(t *testing.T) {
	tests := []struct {
		region              string
		az                  string
		clustername         string
		cidrs               []string
		endpointName        string
		masterCount         int
		apiEndpoint         string
		mockSubnetIDInput   []string
		mockSubnetIDOutput0 []string
		mockSubnetIDOutput1 error
	}{
		{
			region:       defaultRegionName,
			az:           defaultAzName,
			clustername:  "test1",
			cidrs:        []string{"0.0.0.0/0"},
			endpointName: "rh-api",
			masterCount:  3,
			apiEndpoint:  defaultAPIEndpoint,
		},
	}
	for _, test := range tests {
		aObj := makeAPISchemeObj(test.endpointName, true, test.cidrs)
		masterNames := make([]string, test.masterCount)
		for i := 0; i < test.masterCount; i++ {
			masterNames[i] = fmt.Sprintf("master-%d", i)
		}
		machineList, _ := createMachineObjectList(masterNames, test.clustername, "master", test.region, test.az)
		infraObj := infraObj(test.clustername, test.apiEndpoint, test.apiEndpoint, test.region)
		objs := []runtime.Object{aObj, infraObj, machineList}
		mocks := setupTests(t, objs)
		awsClient = mocks.mockAws

		r := &ReconcileAPIScheme{client: mocks.fakeKubeClient, scheme: mocks.scheme}
		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      test.endpointName,
				Namespace: config.OperatorNamespace,
			},
		}

		res, err := r.Reconcile(req)
		if err != nil {
			t.Fatalf("reconcile: (%v)", err)
		}
		fmt.Printf("res = %+v\n", res)
	}

}
*/
