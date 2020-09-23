package aws

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/elbv2/elbv2iface"
)

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
			Resp:          elbv2.CreateLoadBalancerOutput{},
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
