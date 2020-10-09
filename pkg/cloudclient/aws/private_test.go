package aws

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/elbv2/elbv2iface"
	"github.com/openshift/cloud-ingress-operator/pkg/testutils"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	awsproviderapi "sigs.k8s.io/cluster-api-provider-aws/pkg/apis/awsproviderconfig/v1beta1"
)

func TestUpdateAWSLBList(t *testing.T) {
	clustername := "lbupdatetest"
	sampleMachine := testutils.CreateMachineObj("master-0", clustername, "master", testutils.DefaultRegionName, testutils.DefaultAzName)
	objs := []runtime.Object{&sampleMachine}
	mocks := testutils.NewTestMock(t, objs)

	oldLBList := []awsproviderapi.LoadBalancerReference{
		{
			// <clustername>-<id>-ext
			Name: fmt.Sprintf("%s-%s-ext", clustername, "12345"),
			Type: awsproviderapi.NetworkLoadBalancerType,
		},
		{
			// <clustername>-<id>-int
			Name: fmt.Sprintf("%s-%s-int", clustername, "12345"),
			Type: awsproviderapi.NetworkLoadBalancerType,
		},
	}
	newLBList := []awsproviderapi.LoadBalancerReference{
		{
			// Just something random!
			Name: fmt.Sprintf("%s-%s-test", clustername, "12345"),
			Type: awsproviderapi.NetworkLoadBalancerType,
		},
	}
	// quickly make sure the test is going to measure an actual change
	if len(oldLBList) != 2 || len(newLBList) != 1 {
		t.Fatalf("Initial test conditions are unexpected. Old LB list should be 2 (got %d), New LB list should be 1 (got %d)", len(oldLBList), len(newLBList))
	}
	// decode spec
	codec, err := awsproviderapi.NewCodec()
	if err != nil {
		t.Fatalf("Can't create decoder codec for AWS Provider API %v", err)
	}
	awsconfig := &awsproviderapi.AWSMachineProviderConfig{}
	err = codec.DecodeProviderSpec(&sampleMachine.Spec.ProviderSpec, awsconfig)
	if err != nil {
		t.Fatalf("Can't decode sample Machine ProviderSpec %v", err)
	}

	err = updateAWSLBList(mocks.FakeKubeClient, oldLBList, newLBList, sampleMachine, awsconfig)
	if err != nil {
		t.Fatalf("Couldn't update AWS LoadBalancer List: %v", err)
	}

	machineInfo := types.NamespacedName{
		Name:      sampleMachine.GetName(),
		Namespace: sampleMachine.GetNamespace(),
	}
	// reload the object to make sure we're not just working with the "in-memory"
	// representation, that being, the un-saved one.
	err = mocks.FakeKubeClient.Get(context.TODO(), machineInfo, &sampleMachine)
	if err != nil {
		t.Fatalf("Couldn't reload the test machine: %v", err)
	}

	err = codec.DecodeProviderSpec(&sampleMachine.Spec.ProviderSpec, awsconfig)
	if err != nil {
		t.Fatalf("Can't decode sample Machine ProviderSpec %v", err)
	}

	if len(awsconfig.LoadBalancers) != 1 {
		t.Fatalf("Expected to have only 1 LoadBalancer, but got %d", len(awsconfig.LoadBalancers))
	}
	if awsconfig.LoadBalancers[0].Name != newLBList[0].Name {
		t.Fatalf("Expected LB name to be %s, but got %s", newLBList[0].Name, awsconfig.LoadBalancers[0].Name)
	}

	if awsconfig.LoadBalancers[0].Type != newLBList[0].Type {
		t.Fatalf("Expected LB type to be %s, but got %s", string(newLBList[0].Type), string(awsconfig.LoadBalancers[0].Type))
	}
}

func TestAddAWSLBToMasters(t *testing.T) {
	masterNames := make([]string, 3)
	for i := 0; i < 3; i++ {
		masterNames[i] = fmt.Sprintf("master-%d", i)
	}
	machineList, _ := testutils.CreateMachineObjectList(masterNames, "add-aws-elb", "master", testutils.DefaultRegionName, testutils.DefaultAzName)

	objs := []runtime.Object{machineList}
	mocks := testutils.NewTestMock(t, objs)

	// each Machine ought to have 2 NLBs at the start, so let's check
	for _, machine := range machineList.Items {
		machineInfo := types.NamespacedName{
			Name:      machine.GetName(),
			Namespace: machine.GetNamespace(),
		}
		// reload the object to make sure we're not just working with the "in-memory"
		// representation, that being, the un-saved one.
		err := mocks.FakeKubeClient.Get(context.TODO(), machineInfo, &machine)
		if err != nil {
			t.Fatalf("Couldn't reload the test machine (named %s): %v", machineInfo.Name, err)
		}
		l, _, _, err := testutils.ValidateMachineLB(&machine)
		if err != nil {
			t.Fatalf("Couldn't lookup the LB info: %v", err)
		}
		if l != 2 {
			t.Fatalf("Before the test we expect to have 2 load balancers, but got %d", l)
		}
	}

	elbname := "myelb"
	err := AddAWSLBToMasterMachines(mocks.FakeKubeClient, elbname, machineList)
	if err != nil {
		t.Fatalf("err %v", err)
	}

	// Now we should have 3 load balancers, so let's check -- and they should be NLBs
	for _, machine := range machineList.Items {
		machineInfo := types.NamespacedName{
			Name:      machine.GetName(),
			Namespace: machine.GetNamespace(),
		}
		// reload the object to make sure we're not just working with the "in-memory"
		// representation, that being, the un-saved one.
		err = mocks.FakeKubeClient.Get(context.TODO(), machineInfo, &machine)
		if err != nil {
			t.Fatalf("Couldn't reload the test machine (named %s): %v", machineInfo.Name, err)
		}
		l, lbNames, lbTypes, err := testutils.ValidateMachineLB(&machine)
		if err != nil {
			t.Fatalf("Can't lookup LB info: %v", err)
		}
		foundNewName := false
		for _, lbName := range lbNames {
			if lbName == elbname {
				foundNewName = true
			}
		}
		if !foundNewName {
			t.Fatalf("Tried to add a new load balancer named %s but didn't actually find it in the %d LoadBalancers for Machine %s", elbname, l, machine.GetName())
		}
		if l != 3 {
			t.Fatalf("After the test we expect to have 3 load balancers, but got %d", l)
		}
		for _, lbType := range lbTypes {
			if lbType != awsproviderapi.NetworkLoadBalancerType {
				t.Fatalf("Expected to have a NLB, but got %s instead", string(lbType))
			}
		}
	}
}

func TestRemoveAWSELB(t *testing.T) {
	clusterName := "test-remove"

	tests := []struct {
		nameToRemove       string // name of the load balancer to remove
		skipPreCheck       bool   // Skip the pre-test load balancer validation of name/types?
		loadBalancersAtEnd int    // how many load balancers should the machine object have when the test is done?
	}{
		{
			nameToRemove:       fmt.Sprintf("%s-%s-ext", clusterName, testutils.ClusterTokenId),
			skipPreCheck:       false,
			loadBalancersAtEnd: 1,
		},
		{
			nameToRemove:       "missing",
			skipPreCheck:       true, // the test doesn't want to check for the presence of this token since it's never meant to be there.
			loadBalancersAtEnd: 2,    // since "missing" is never there, we should still have 2
		},
	}
	for _, test := range tests {
		masterNames := make([]string, 3)
		for i := 0; i < 3; i++ {
			masterNames[i] = fmt.Sprintf("master-%d", i)
		}
		machineList, _ := testutils.CreateMachineObjectList(masterNames, clusterName, "master", testutils.DefaultRegionName, testutils.DefaultAzName)

		objs := []runtime.Object{machineList}
		mocks := testutils.NewTestMock(t, objs)

		// each Machine ought to have 2 NLBs at the start, so let's check
		for _, machine := range machineList.Items {
			machineInfo := types.NamespacedName{
				Name:      machine.GetName(),
				Namespace: machine.GetNamespace(),
			}
			// reload the object to make sure we're not just working with the "in-memory"
			// representation, that being, the un-saved one.
			err := mocks.FakeKubeClient.Get(context.TODO(), machineInfo, &machine)
			if err != nil {
				t.Fatalf("Couldn't reload the test machine (named %s): %v", machineInfo.Name, err)
			}
			l, lbNames, lbTypes, err := testutils.ValidateMachineLB(&machine)
			if err != nil {
				t.Fatalf("Couldn't lookup the LB info: %v", err)
			}
			if l != 2 {
				t.Fatalf("Before the test we expect to have 2 load balancers, but got %d", l)
			}
			// Pre-check: check for our LB by name and type unless we need it to be missing
			if !test.skipPreCheck {
				found := false
				for i := 0; i < l; i++ {
					if lbNames[i] == test.nameToRemove && lbTypes[i] == awsproviderapi.NetworkLoadBalancerType {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("Machine %s doesn't have a network load balancer named %s. It has %s", machine.GetName(), test.nameToRemove, lbNames)
				}
			}
			// End of pre-check
		}

		// Make change
		err := RemoveAWSLBFromMasterMachines(mocks.FakeKubeClient, test.nameToRemove, machineList)
		if err != nil {
			t.Fatalf("Unexpected test couldn't remove LB %s from Machine: %v", test.nameToRemove, err)
		}

		// Validate test.nameToRemove is missing
		for _, machine := range machineList.Items {
			// reload the object to make sure we're not just working with the "in-memory"
			// representation, that being, the un-saved one.
			machineInfo := types.NamespacedName{
				Name:      machine.GetName(),
				Namespace: machine.GetNamespace(),
			}

			err = mocks.FakeKubeClient.Get(context.TODO(), machineInfo, &machine)
			if err != nil {
				t.Fatalf("Couldn't reload the test machine (named %s): %v", machineInfo.Name, err)
			}
			l, lbNames, _, err := testutils.ValidateMachineLB(&machine)
			if err != nil {
				t.Fatalf("Couldn't load the LB info for %s: %v", machineInfo.Name, err)
			}
			if l != test.loadBalancersAtEnd {
				t.Fatalf("Test to remove load balancer named '%s': Expected to have %d load balancers afterwards, but got %d. Load balancers = %s", test.nameToRemove, test.loadBalancersAtEnd, l, lbNames)
			}
			for _, lbName := range lbNames {
				if lbName == test.nameToRemove {
					t.Fatalf("Machine %s still has load balancer named %s", machineInfo.Name, test.nameToRemove)
				}
			}
		}
	}
}

func TestAWSProviderDecode(t *testing.T) {
	machine := testutils.CreateMachineObj("master-0", "decode", "master", testutils.DefaultRegionName, testutils.DefaultAzName)

	objs := []runtime.Object{&machine}
	mocks := testutils.NewTestMock(t, objs)
	machineInfo := types.NamespacedName{
		Name:      machine.GetName(),
		Namespace: machine.GetNamespace(),
	}

	err := mocks.FakeKubeClient.Get(context.TODO(), machineInfo, &machine)
	if err != nil {
		t.Fatalf("Couldn't reload machine %s: %v", machine.GetName(), err)
	}

	_, err = getAWSDecodedProviderSpec(machine)
	if err != nil {
		t.Fatalf("Failed to decode machine %s: %v", machine.GetName(), err)
	}

}

type mockDescribeELBv2LoadBalancers struct {
	elbv2iface.ELBV2API
	Resp    elbv2.DescribeLoadBalancersOutput
	ErrResp string
}

func (m mockDescribeELBv2LoadBalancers) DescribeLoadBalancers(_ *elbv2.DescribeLoadBalancersInput) (*elbv2.DescribeLoadBalancersOutput, error) {
	var e awserr.Error
	if m.ErrResp != "" {
		e = awserr.New(m.ErrResp, m.ErrResp, fmt.Errorf("Error raised by test"))
	}
	return &m.Resp, e
}

func TestListAllNLBs(t *testing.T) {
	tests := []struct {
		// Resp is the mocked response
		Resp elbv2.DescribeLoadBalancersOutput
		// Expected is what the test wants to see given the input
		Expected      []loadBalancerV2
		ErrResp       string
		ErrorExpected bool
	}{
		{
			// Nothing back from Amazon
			ErrorExpected: false,
			Resp:          elbv2.DescribeLoadBalancersOutput{LoadBalancers: []*elbv2.LoadBalancer{}},
			Expected:      []loadBalancerV2{},
		},
		{
			// Return one NLB
			ErrResp: "",
			Resp: elbv2.DescribeLoadBalancersOutput{
				LoadBalancers: []*elbv2.LoadBalancer{
					{
						CanonicalHostedZoneId: aws.String("/test/ABC123"),
						DNSName:               aws.String("test.example.com"),
						LoadBalancerArn:       aws.String("arn:123456"),
						LoadBalancerName:      aws.String("testlb-ext"),
						Scheme:                aws.String("internal-facing"),
						VpcId:                 aws.String("vpc-123456"),
						IpAddressType:         aws.String("ipv4"),
						State:                 &elbv2.LoadBalancerState{Code: aws.String("active")},
						Type:                  aws.String("network"),

						AvailabilityZones: []*elbv2.AvailabilityZone{
							{
								LoadBalancerAddresses: []*elbv2.LoadBalancerAddress{
									{
										AllocationId: aws.String("foo"),
										IpAddress:    aws.String("10.10.10.10"),
									},
								},
							},
						},
					},
				},
			},
			Expected: []loadBalancerV2{
				{
					canonicalHostedZoneNameID: "/test/ABC123",
					dnsName:                   "test.example.com",
					loadBalancerArn:           "arn:123456",
					loadBalancerName:          "testlb-ext",
					scheme:                    "internal-facing",
					vpcID:                     "vpc-123456",
				},
			},
		},
	}

	for _, test := range tests {
		client := &Client{
			elbv2Client: mockDescribeELBv2LoadBalancers{Resp: test.Resp, ErrResp: test.ErrResp},
		}
		resp, err := client.listAllNLBs()
		if err == nil && test.ErrorExpected || err != nil && !test.ErrorExpected {
			t.Fatalf("Test return mismatch. Expect error? %t: Return %+v", test.ErrorExpected, err)
		}
		if len(resp) != len(test.Expected) {
			t.Fatalf("Expected %d loadBalancerV2 objects, got %d", len(test.Expected), len(resp))
		}
		if !reflect.DeepEqual(resp, test.Expected) {
			t.Fatalf("Return from listAllNLBs does not match expectation. Expected %+v. Got %+v", test.Expected, resp)
		}
	}
}

type mockDeleteLoadBalancer struct {
	elbv2iface.ELBV2API
	Resp    elbv2.DeleteLoadBalancerOutput
	ErrResp string // elbv2 error code constant
}

func (m mockDeleteLoadBalancer) DeleteLoadBalancer(_ *elbv2.DeleteLoadBalancerInput) (*elbv2.DeleteLoadBalancerOutput, error) {
	var e awserr.Error
	if m.ErrResp != "" {
		e = awserr.New(m.ErrResp, m.ErrResp, fmt.Errorf("Error raised by test"))
	}
	return &m.Resp, e
}

func TestDeleteExternalV2LoadBalancer(t *testing.T) {
	tests := []struct {
		Resp          elbv2.DeleteLoadBalancerOutput
		ErrResp       string
		Arn           string
		ErrorExpected bool // should the test expect an error?
	}{
		{
			Resp:          elbv2.DeleteLoadBalancerOutput{},
			ErrResp:       "",
			Arn:           "test-delete-elbv2-lb",
			ErrorExpected: false,
		},
		{
			Resp:          elbv2.DeleteLoadBalancerOutput{},
			ErrResp:       "ErrCodeLoadBalancerNotFoundException",
			Arn:           "test-delete-elbv2-lb",
			ErrorExpected: true,
		},
	}
	for _, test := range tests {
		client := &Client{
			elbv2Client: mockDeleteLoadBalancer{Resp: test.Resp, ErrResp: test.ErrResp},
		}
		resp := client.deleteExternalLoadBalancer(test.Arn)
		if resp == nil && test.ErrorExpected || resp != nil && !test.ErrorExpected {
			t.Fatalf("Test return mismatch. Expect error? %t: Return %+v", test.ErrorExpected, resp)
		}
	}
}

type mockCreateLoadBalancer struct {
	elbv2iface.ELBV2API
	Resp    elbv2.CreateLoadBalancerOutput
	ErrResp string
}

func (m mockCreateLoadBalancer) CreateLoadBalancer(i *elbv2.CreateLoadBalancerInput) (*elbv2.CreateLoadBalancerOutput, error) {
	var e awserr.Error
	if m.ErrResp != "" {
		e = awserr.New(m.ErrResp, m.ErrResp, fmt.Errorf("Error raised by test"))
	}
	return &m.Resp, e
}

func TestCreateNetworkLoadBalancer(t *testing.T) {
	// TODO: Fill this in
	tests := []struct {
		Resp          elbv2.CreateLoadBalancerOutput
		ErrResp       string
		ErrorExpected bool
		Expected      []loadBalancerV2
		LbName        string
		Scheme        string
		Subnet        string
	}{
		{
			LbName: "test-lb",
			Scheme: "internal",
			Subnet: "subnet-12345",
			Expected: []loadBalancerV2{
				{},
			},

			ErrorExpected: false,
			ErrResp:       "",
			Resp: elbv2.CreateLoadBalancerOutput{
				LoadBalancers: []*elbv2.LoadBalancer{
					{},
				},
			},
		},
	}
	for _, test := range tests {
		client := &Client{
			elbv2Client: mockCreateLoadBalancer{Resp: test.Resp, ErrResp: test.ErrResp},
		}
		resp, err := client.createNetworkLoadBalancer(test.LbName, test.Scheme, test.Subnet)
		if err == nil && test.ErrorExpected || err != nil && !test.ErrorExpected {
			t.Fatalf("Test return mismatch. Expect error? %t: Return %+v", test.ErrorExpected, err)
		}
		if len(resp) != 1 {
			t.Fatalf("Expected exactly 1 loadBalancerV2 from createNetworkLoadBalancer, but got %d", len(resp))
		}
		if len(resp) != len(test.Expected) {
			t.Fatalf("Mismatch. Expected %d loadBalancerV2, got %d", len(test.Expected), len(resp))
		}

	}
}
