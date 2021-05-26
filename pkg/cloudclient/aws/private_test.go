package aws

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/openshift/cloud-ingress-operator/pkg/testutils"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	awsproviderapi "sigs.k8s.io/cluster-api-provider-aws/pkg/apis/awsproviderconfig/v1beta1"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/elbv2/elbv2iface"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/route53/route53iface"
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
		err := removeAWSLBFromMasterMachines(mocks.FakeKubeClient, test.nameToRemove, machineList)
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
	Resp        elbv2.DescribeLoadBalancersOutput
	ErrResp     string
	TagsResp    elbv2.DescribeTagsOutput
	TagsErrResp string
}

func (m mockDescribeELBv2LoadBalancers) DescribeLoadBalancers(_ *elbv2.DescribeLoadBalancersInput) (*elbv2.DescribeLoadBalancersOutput, error) {
	var e awserr.Error
	if m.ErrResp != "" {
		e = awserr.New(m.ErrResp, m.ErrResp, fmt.Errorf("Error raised by test"))
	}
	return &m.Resp, e
}

func (m mockDescribeELBv2LoadBalancers) DescribeLoadBalancersPages(input *elbv2.DescribeLoadBalancersInput, fn func(*elbv2.DescribeLoadBalancersOutput, bool) bool) error {
	out, e := m.DescribeLoadBalancers(input)
	if e != nil {
		return e
	}
	// Simulate multiple output pages by sending the callback function
	// one LoadBalancer at a time.
	for index, loadBalancer := range out.LoadBalancers {
		fakeOut := &elbv2.DescribeLoadBalancersOutput{
			LoadBalancers: []*elbv2.LoadBalancer{loadBalancer},
		}
		lastPage := index+1 == len(out.LoadBalancers)
		if !fn(fakeOut, lastPage) {
			break
		}
	}
	return nil
}

func (m mockDescribeELBv2LoadBalancers) DescribeTags(input *elbv2.DescribeTagsInput) (*elbv2.DescribeTagsOutput, error) {
	var e awserr.Error
	if m.TagsErrResp != "" {
		e = awserr.New(m.TagsErrResp, m.TagsErrResp, fmt.Errorf("Error raised by test"))
	}
	return &m.TagsResp, e
}

func TestListOwnedNLBs(t *testing.T) {
	clusterName := "list-owned-nlbs-test"
	infraObj := testutils.CreateInfraObject(clusterName, testutils.DefaultAPIEndpoint, testutils.DefaultAPIEndpoint, testutils.DefaultRegionName)
	objs := []runtime.Object{infraObj}
	mocks := testutils.NewTestMock(t, objs)

	ownedTag := &elbv2.Tag{
		Key:   aws.String("kubernetes.io/cluster/" + clusterName),
		Value: aws.String("owned"),
	}
	sharedTag := &elbv2.Tag{
		Key:   aws.String("kubernetes.io/cluster/" + clusterName),
		Value: aws.String("shared"),
	}

	tests := []struct {
		// Resp is the mocked DescribeLoadBalancers response
		Resp elbv2.DescribeLoadBalancersOutput
		// TagsResp is the mocked DescribeTags response
		TagsResp elbv2.DescribeTagsOutput
		// Expected is what the test wants to see given the input
		Expected      []loadBalancerV2
		ErrResp       string
		ErrorExpected bool
	}{
		{
			// Nothing back from Amazon
			ErrorExpected: false,
			Resp:          elbv2.DescribeLoadBalancersOutput{LoadBalancers: []*elbv2.LoadBalancer{}},
			TagsResp:      elbv2.DescribeTagsOutput{},
			Expected:      []loadBalancerV2{},
		},
		{
			// One NLB, but not owned by the cluster
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
			TagsResp: elbv2.DescribeTagsOutput{
				TagDescriptions: []*elbv2.TagDescription{
					{
						ResourceArn: aws.String("arn:123456"),
						Tags:        []*elbv2.Tag{},
					},
				},
			},
			Expected: []loadBalancerV2{},
		},
		{
			// One NLB, owned by the cluster
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
			TagsResp: elbv2.DescribeTagsOutput{
				TagDescriptions: []*elbv2.TagDescription{
					{
						ResourceArn: aws.String("arn:123456"),
						Tags:        []*elbv2.Tag{ownedTag},
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
		{
			// Two NLBs, but only one is owned by the cluster
			ErrResp: "",
			Resp: elbv2.DescribeLoadBalancersOutput{
				LoadBalancers: []*elbv2.LoadBalancer{
					{
						CanonicalHostedZoneId: aws.String("/test/ABC123"),
						DNSName:               aws.String("test1.example.com"),
						LoadBalancerArn:       aws.String("arn:123456"),
						LoadBalancerName:      aws.String("testlb1-ext"),
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
					{
						CanonicalHostedZoneId: aws.String("/test/ABC123"),
						DNSName:               aws.String("test2.example.com"),
						LoadBalancerArn:       aws.String("arn:654321"),
						LoadBalancerName:      aws.String("testlb2-ext"),
						Scheme:                aws.String("internal-facing"),
						VpcId:                 aws.String("vpc-654321"),
						IpAddressType:         aws.String("ipv4"),
						State:                 &elbv2.LoadBalancerState{Code: aws.String("active")},
						Type:                  aws.String("network"),

						AvailabilityZones: []*elbv2.AvailabilityZone{
							{
								LoadBalancerAddresses: []*elbv2.LoadBalancerAddress{
									{
										AllocationId: aws.String("bar"),
										IpAddress:    aws.String("20.20.20.20"),
									},
								},
							},
						},
					},
				},
			},
			TagsResp: elbv2.DescribeTagsOutput{
				TagDescriptions: []*elbv2.TagDescription{
					{
						ResourceArn: aws.String("arn:123456"),
						Tags:        []*elbv2.Tag{sharedTag},
					},
					{
						ResourceArn: aws.String("arn:654321"),
						Tags:        []*elbv2.Tag{ownedTag},
					},
				},
			},
			Expected: []loadBalancerV2{
				{
					canonicalHostedZoneNameID: "/test/ABC123",
					dnsName:                   "test2.example.com",
					loadBalancerArn:           "arn:654321",
					loadBalancerName:          "testlb2-ext",
					scheme:                    "internal-facing",
					vpcID:                     "vpc-654321",
				},
			},
		},
	}

	for _, test := range tests {
		client := &Client{
			elbv2Client: mockDescribeELBv2LoadBalancers{
				Resp:        test.Resp,
				ErrResp:     test.ErrResp,
				TagsResp:    test.TagsResp,
				TagsErrResp: "",
			},
		}
		resp, err := client.listOwnedNLBs(mocks.FakeKubeClient)
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

// BZ https://bugzilla.redhat.com/show_bug.cgi?id=1814332
func TestOldClusterNoInfrastructureBackfill(t *testing.T) {
	clustername := "oldtest"
	// legacy infra obj has this extra bits, but the configmap does not
	extrabits := "cmld7"
	oldInfraObj := testutils.CreatOldInfraObject("oldtest-cmld7", testutils.DefaultAPIEndpoint, testutils.DefaultAPIEndpoint, testutils.DefaultRegionName)
	oldCM := testutils.CreateLegacyClusterConfig(fmt.Sprintf("%s.%s", clustername, testutils.DefaultClusterDomain),
		fmt.Sprintf("%s-%s", clustername, extrabits), testutils.DefaultRegionName, 0, 0)
	objs := []runtime.Object{oldInfraObj, oldCM}
	mocks := testutils.NewTestMock(t, objs)
	region, err := getClusterRegion(mocks.FakeKubeClient)
	if err != nil {
		t.Fatalf("Error: Couldn't get region. Expected to get %s: %v", testutils.DefaultRegionName, err)
	}
	if region != testutils.DefaultRegionName {
		t.Fatalf("Expected region to be %s, but got %s", testutils.DefaultRegionName, region)
	}
}

func TestGetMasterSubnetNames(t *testing.T) {
	clustername := "subnet-test"
	masterNames := make([]string, 3)
	for i := 0; i < 3; i++ {
		masterNames[i] = fmt.Sprintf("master-%d", i)
	}

	machineList, _ := testutils.CreateMachineObjectList(masterNames, "ids", "master", testutils.DefaultRegionName, testutils.DefaultAzName)
	infraObj := testutils.CreateInfraObject(clustername, testutils.DefaultAPIEndpoint, testutils.DefaultAPIEndpoint, testutils.DefaultRegionName)
	objs := []runtime.Object{infraObj, machineList}
	mocks := testutils.NewTestMock(t, objs)
	subnetmap, err := getMasterNodeSubnets(mocks.FakeKubeClient)

	if err != nil {
		t.Fatalf("Couldn't get subnet map: %v", err)
	}
	if len(subnetmap) != 2 {
		t.Fatalf("Expected Subnet map to have exactly two keys, but got %d keys back", len(subnetmap))
	}
	if _, ok := subnetmap["public"]; !ok {
		t.Fatalf("Expected Subnet map to have a 'public' key, and it did not")
	}
	if _, ok := subnetmap["private"]; !ok {
		t.Fatalf("Expected Subnet map to have a 'private' key, and it did not")
	}
	expected := fmt.Sprintf("%s-public-%s", clustername, testutils.DefaultAzName)
	if subnetmap["public"] != expected {
		t.Fatalf("Expected public subnet to be %s, but got %s instead", expected, subnetmap["public"])
	}
	expected = fmt.Sprintf("%s-private-%s", clustername, testutils.DefaultAzName)
	if subnetmap["private"] != expected {
		t.Fatalf("Expected private subnet to be %s, but got %s instead", expected, subnetmap["private"])
	}
}

func TestGetMasterSubnetNamesNoMasters(t *testing.T) {
	clustername := "subnet-test"
	masterNames := make([]string, 0)
	machineList, _ := testutils.CreateMachineObjectList(masterNames, "ids", "master", testutils.DefaultRegionName, testutils.DefaultAzName)
	infraObj := testutils.CreateInfraObject(clustername, testutils.DefaultAPIEndpoint, testutils.DefaultAPIEndpoint, testutils.DefaultRegionName)
	objs := []runtime.Object{infraObj, machineList}
	mocks := testutils.NewTestMock(t, objs)

	_, err := getMasterNodeSubnets(mocks.FakeKubeClient)
	if err == nil {
		t.Fatalf("Expected to see an exception when trying to get subnets for 0 master nodes ")
	}
}

func TestGetClusterRegion(t *testing.T) {
	infraObj := testutils.CreateInfraObject("region-test", testutils.DefaultAPIEndpoint, testutils.DefaultAPIEndpoint, testutils.DefaultRegionName)
	objs := []runtime.Object{infraObj}
	mocks := testutils.NewTestMock(t, objs)

	region, err := getClusterRegion(mocks.FakeKubeClient)
	if err != nil {
		t.Fatalf("Could not get cluster region: %v", err)
	}
	if region != testutils.DefaultRegionName {
		t.Fatalf("Region mismatch. Expected %s, got %s", region, testutils.DefaultRegionName)
	}
}

// None of these should ever occur, but if they did, it'd be nice to know they return an error
func TestNoInfraObj(t *testing.T) {
	masterNames := make([]string, 3)
	for i := 0; i < 3; i++ {
		masterNames[i] = fmt.Sprintf("master-%d", i)
	}
	machineList, _ := testutils.CreateMachineObjectList(masterNames, "ids", "master", testutils.DefaultRegionName, testutils.DefaultAzName)
	objs := []runtime.Object{machineList}
	mocks := testutils.NewTestMock(t, objs)

	_, err := getClusterRegion(mocks.FakeKubeClient)
	if err == nil {
		t.Fatalf("Expected to get an error from not having an Infrastructure object")
	}
	_, err = getMasterNodeSubnets(mocks.FakeKubeClient)
	if err == nil {
		t.Fatalf("Expected to get an error from not having an Infrastructure object")
	}
}

type mockRoute53Client struct {
	route53iface.Route53API
}

func (m mockRoute53Client) ListResourceRecordSetsPages(input *route53.ListResourceRecordSetsInput, fn func(*route53.ListResourceRecordSetsOutput, bool) bool) error {
	resps := []*route53.ListResourceRecordSetsOutput{
		{
			ResourceRecordSets: []*route53.ResourceRecordSet{
				{
					Name: aws.String("rh-api.osd-cluster.org."),
					Type: aws.String("A"),
					AliasTarget: &route53.AliasTarget{
						DNSName:              aws.String("abcdefgh.us-east-1.elb.amazon.com."),
						EvaluateTargetHealth: aws.Bool(false),
						HostedZoneId:         aws.String("AAAAAAAAAA"),
					},
				},
				{
					Name: aws.String("api-osd-cluster.org."),
					Type: aws.String("A"),
					AliasTarget: &route53.AliasTarget{
						DNSName:              aws.String("0123456.elb.us-east-1.amazonaws.com."),
						EvaluateTargetHealth: aws.Bool(false),
						HostedZoneId:         aws.String("BBBBBBBBBB"),
					},
				},
			},
			IsTruncated: aws.Bool(false),
			MaxItems:    aws.String("100"),
		},
	}
	for _, resp := range resps {
		if !fn(resp, true) {
			break
		}
	}
	return nil
}

func TestRecordExists(t *testing.T) {
	tests := []struct {
		Name          string
		Record        *route53.ResourceRecordSet // the record to check
		Resp          bool
		ErrResp       string
		ErrorExpected bool
	}{
		{
			Name: "Record should exist",
			Record: &route53.ResourceRecordSet{
				AliasTarget: &route53.AliasTarget{
					DNSName:              aws.String("abcdefgh.us-east-1.elb.amazon.com."),
					EvaluateTargetHealth: aws.Bool(false),
					HostedZoneId:         aws.String("AAAAAAAAAA"),
				},
				Name: aws.String("rh-api.osd-cluster.org."),
				Type: aws.String("A"),
			},
			Resp:          true,
			ErrResp:       "",
			ErrorExpected: false,
		},
		{
			Name: "Record with non FDQN name should still exist",
			Record: &route53.ResourceRecordSet{
				AliasTarget: &route53.AliasTarget{
					DNSName:              aws.String("abcdefgh.us-east-1.elb.amazon.com"),
					EvaluateTargetHealth: aws.Bool(false),
					HostedZoneId:         aws.String("AAAAAAAAAA"),
				},
				Name: aws.String("rh-api.osd-cluster.org"),
				Type: aws.String("A"),
			},
			Resp:          true,
			ErrResp:       "",
			ErrorExpected: false,
		},
		{
			Name: "Record shouldn't exist",
			Record: &route53.ResourceRecordSet{
				AliasTarget: &route53.AliasTarget{
					DNSName:              aws.String("abcdefgh.us-east-1.elb.amazon.com."),
					EvaluateTargetHealth: aws.Bool(false),
					HostedZoneId:         aws.String("AAAAAAAAAA"),
				},
				Name: aws.String("rh-ssh.osd-cluster.org."),
				Type: aws.String("A"),
			},
			Resp:          false,
			ErrResp:       "",
			ErrorExpected: false,
		},
		{
			Name: "Record with matching Name but missmatched Type shouldn't exist",
			Record: &route53.ResourceRecordSet{
				AliasTarget: &route53.AliasTarget{
					DNSName:              aws.String("abcdefgh.us-east-1.elb.amazon.com."),
					EvaluateTargetHealth: aws.Bool(false),
					HostedZoneId:         aws.String("AAAAAAAAAA"),
				},
				Name: aws.String("rh-ssh.osd-cluster.org."),
				Type: aws.String("AAAA"),
			},
			Resp:          false,
			ErrResp:       "",
			ErrorExpected: false,
		},
		{
			Name: "Record with matching Name and Type, but no AliasTarget, shoudn't exist",
			Record: &route53.ResourceRecordSet{
				Name: aws.String("rh-ssh.osd-cluster.org."),
				Type: aws.String("A"),
			},
			Resp:          false,
			ErrResp:       "",
			ErrorExpected: false,
		},
		{
			Name: "Record with matching Name and Type, but missmatched AliasTarget.EvaluateTargetHealth , shoudn't exist",
			Record: &route53.ResourceRecordSet{
				AliasTarget: &route53.AliasTarget{
					DNSName:              aws.String("abcdefgh.us-east-1.elb.amazon.com."),
					EvaluateTargetHealth: aws.Bool(true),
					HostedZoneId:         aws.String("AAAAAAAAAA"),
				},
				Name: aws.String("rh-ssh.osd-cluster.org."),
				Type: aws.String("A"),
			},
			Resp:          false,
			ErrResp:       "",
			ErrorExpected: false,
		},
		{
			Name:          "nil Record should error",
			Record:        nil,
			Resp:          false,
			ErrResp:       "resourceRecordSet can't be nil",
			ErrorExpected: true,
		},
		{
			Name: "nil Record.Name should error",
			Record: &route53.ResourceRecordSet{
				AliasTarget: &route53.AliasTarget{
					DNSName:              aws.String("abcdefgh.us-east-1.elb.amazon.com."),
					EvaluateTargetHealth: aws.Bool(false),
					HostedZoneId:         aws.String("AAAAAAAAAA"),
				},
				Name: nil,
				Type: aws.String("A"),
			},
			Resp:          false,
			ErrResp:       "resourceRecordSet Name is required",
			ErrorExpected: true,
		},
	}

	for _, test := range tests {
		client := &Client{
			route53Client: mockRoute53Client{},
		}
		resp, err := client.recordExists(test.Record, "publicHostedZoneID")

		if err == nil && test.ErrorExpected || err != nil && !test.ErrorExpected {
			t.Fatalf("Test [%v] return mismatch. Expect error? %t: Return %+v", test.Name, test.ErrorExpected, err)
		}
		if err != nil && test.ErrorExpected && err.Error() != test.ErrResp {
			t.Fatalf("Test [%v] FAILED. Excepted Error %v. Got %v", test.Name, test.ErrResp, err.Error())
		}
		if resp != test.Resp {
			t.Fatalf("Test [%v] FAILED. Excepted Response %v. Got %v", test.Name, test.Resp, resp)
		}

	}

}
