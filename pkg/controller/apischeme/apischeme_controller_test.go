package apischeme

import (
	"context"
	"fmt"
	"testing"

	"github.com/openshift/cloud-ingress-operator/pkg/testutils"

	configv1 "github.com/openshift/api/config/v1"
	utils "github.com/openshift/cloud-ingress-operator/pkg/controller/utils"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

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
			region:       testutils.DefaultRegionName,
			az:           testutils.DefaultAzName,
			clustername:  "test1",
			cidrs:        []string{"0.0.0.0/0"},
			endpointName: "rh-api",
			masterCount:  3,
			apiEndpoint:  testutils.DefaultAPIEndpoint,

			mockSubnetIDInput:   []string{"test1-public-" + testutils.DefaultAzName},
			mockSubnetIDOutput0: []string{"subnet-12345"},
			mockSubnetIDOutput1: nil,
		},
		{
			region:       testutils.DefaultRegionName,
			az:           testutils.DefaultAzName,
			clustername:  "test2",
			cidrs:        []string{"0.0.0.0/0"},
			endpointName: "rh-api",
			masterCount:  3,
			apiEndpoint:  testutils.DefaultAPIEndpoint,

			mockSubnetIDInput:   []string{"test2-public-" + testutils.DefaultAzName},
			mockSubnetIDOutput0: []string{"subnet-abcdef"},
			mockSubnetIDOutput1: nil,
		},
	}

	for testNumber, test := range tests {
		aObj := testutils.CreateAPISchemeObject(test.endpointName, true, test.cidrs)
		masterNames := make([]string, test.masterCount)
		for i := 0; i < test.masterCount; i++ {
			masterNames[i] = fmt.Sprintf("master-%d", i)
		}
		machineList, _ := testutils.CreateMachineObjectList(masterNames, test.clustername, "master", test.region, test.az)
		infraObj := testutils.CreateInfraObject(test.clustername, test.apiEndpoint, test.apiEndpoint, test.region)
		objs := []runtime.Object{aObj, infraObj, machineList}
		mocks := testutils.NewTestMock(t, objs)

		expectedPublicSubnetName := fmt.Sprintf("%s-public-%s", test.clustername, test.az)
		res, err := utils.GetMasterNodeSubnets(mocks.FakeKubeClient)
		if err != nil {
			t.Fatalf("Test %d Couldn't get the master node subnets: %v", testNumber, err)
		}
		if res["public"] != expectedPublicSubnetName {
			t.Fatalf("Subnet mismatch. Got %s, expected %s", res["public"], expectedPublicSubnetName)
		}
		s := []string{res["public"]}
		// now safe to mock the name -> id lookup
		mocks.MockAws.EXPECT().SubnetNameToSubnetIDLookup(test.mockSubnetIDInput).AnyTimes().Return(test.mockSubnetIDOutput0, test.mockSubnetIDOutput1)

		subnets, err := mocks.MockAws.SubnetNameToSubnetIDLookup(s)
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
	aObj := testutils.CreateAPISchemeObject("rh-api", true, []string{"0.0.0.0/0"})
	masterNames := make([]string, 3)
	for i := 0; i < 3; i++ {
		masterNames[i] = fmt.Sprintf("master-%d", i)
	}
	machineList, _ := testutils.CreateMachineObjectList(masterNames, "basename", "master", testutils.DefaultRegionName, testutils.DefaultAzName)
	infraObj := testutils.CreateInfraObject("basename", testutils.DefaultAPIEndpoint, testutils.DefaultAPIEndpoint, testutils.DefaultRegionName)
	objs := []runtime.Object{aObj, infraObj, machineList}
	mocks := testutils.NewTestMock(t, objs)

	base, err := utils.GetClusterBaseDomain(mocks.FakeKubeClient)
	if err != nil {
		t.Fatalf("Could not get cluster base domain name: %v", err)
	}
	if base != "unit.test" {
		t.Fatalf("Base domain mismatch. Expected %s, got %s", "unit.test", base)
	}
}

func TestMasterInstanceIds(t *testing.T) {
	aObj := testutils.CreateAPISchemeObject("rh-api", true, []string{"0.0.0.0/0"})
	masterNames := make([]string, 3)
	for i := 0; i < 3; i++ {
		masterNames[i] = fmt.Sprintf("master-%d", i)
	}
	machineList, _ := testutils.CreateMachineObjectList(masterNames, "ids", "master", testutils.DefaultRegionName, testutils.DefaultAzName)
	infraObj := testutils.CreateInfraObject("ids", testutils.DefaultAPIEndpoint, testutils.DefaultAPIEndpoint, testutils.DefaultRegionName)
	objs := []runtime.Object{aObj, infraObj, machineList}
	mocks := testutils.NewTestMock(t, objs)

	ids, err := utils.GetClusterMasterInstancesIDs(mocks.FakeKubeClient)
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
	aObj := testutils.CreateAPISchemeObject("rh-api", true, []string{"0.0.0.0/0"})
	masterNames := make([]string, 3)
	for i := 0; i < 3; i++ {
		masterNames[i] = fmt.Sprintf("master-%d", i)
	}
	apiserver := testutils.CreateAPIServerObject("apiservertest", testutils.DefaultClusterDomain)
	machineList, _ := testutils.CreateMachineObjectList(masterNames, "ids", "master", testutils.DefaultRegionName, testutils.DefaultAzName)
	infraObj := testutils.CreateInfraObject("ids", testutils.DefaultAPIEndpoint, testutils.DefaultAPIEndpoint, testutils.DefaultRegionName)
	objs := []runtime.Object{aObj, infraObj, machineList, apiserver}
	mocks := testutils.NewTestMock(t, objs)
	r := &ReconcileAPIScheme{client: mocks.FakeKubeClient, scheme: mocks.Scheme}
	domain, err := utils.GetClusterBaseDomain(mocks.FakeKubeClient)
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
			region:       testutils.DefaultRegionName,
			az:           testutils.DefaultAzName,
			clustername:  "test1",
			cidrs:        []string{"0.0.0.0/0"},
			endpointName: "rh-api",
			masterCount:  3,
			apiEndpoint:  testutils.DefaultAPIEndpoint,
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
