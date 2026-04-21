package aws

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func TestMapSecurityGroup(t *testing.T) {
	provider := &Provider{region: "us-east-1"}

	sg := types.SecurityGroup{
		GroupId:   aws.String("sg-0123456789abcdef0"),
		GroupName: aws.String("test-sg"),
		VpcId:    aws.String("vpc-12345"),
		OwnerId:  aws.String("123456789012"),
		Description: aws.String("Test security group"),
		IpPermissions: []types.IpPermission{
			{
				IpProtocol: aws.String("tcp"),
				FromPort:  aws.Int32(443),
				ToPort:    aws.Int32(443),
				IpRanges: []types.IpRange{
					{CidrIp: aws.String("10.0.0.0/8")},
				},
			},
		},
		IpPermissionsEgress: []types.IpPermission{
			{
				IpProtocol: aws.String("-1"),
				IpRanges: []types.IpRange{
					{CidrIp: aws.String("0.0.0.0/0")},
				},
			},
		},
	}

	resource := provider.mapSecurityGroup(sg)

	if resource.ID != "sg-0123456789abcdef0" {
		t.Errorf("expected ID sg-0123456789abcdef0, got %s", resource.ID)
	}
	if resource.Type != ResourceTypeSecurityGroup {
		t.Errorf("expected type %s, got %s", ResourceTypeSecurityGroup, resource.Type)
	}
	if resource.Name != "test-sg" {
		t.Errorf("expected name test-sg, got %s", resource.Name)
	}
	if resource.Provider != "aws" {
		t.Errorf("expected provider aws, got %s", resource.Provider)
	}
	if resource.Region != "us-east-1" {
		t.Errorf("expected region us-east-1, got %s", resource.Region)
	}
	if resource.Data["vpc_id"] != "vpc-12345" {
		t.Errorf("expected vpc_id vpc-12345, got %s", resource.Data["vpc_id"])
	}
	if resource.Data["description"] != "Test security group" {
		t.Errorf("expected description 'Test security group', got %s", resource.Data["description"])
	}
	if resource.Data["owner_id"] != "123456789012" {
		t.Errorf("expected owner_id 123456789012, got %s", resource.Data["owner_id"])
	}

	ingressRules, ok := resource.Data["ingress_rules"].([]map[string]interface{})
	if !ok {
		t.Fatal("ingress_rules should be []map[string]interface{}")
	}
	if len(ingressRules) != 1 {
		t.Fatalf("expected 1 ingress rule, got %d", len(ingressRules))
	}
	if ingressRules[0]["protocol"] != "tcp" {
		t.Errorf("expected protocol tcp, got %s", ingressRules[0]["protocol"])
	}
	if ingressRules[0]["from_port"] != int32(443) {
		t.Errorf("expected from_port 443, got %v", ingressRules[0]["from_port"])
	}
	if ingressRules[0]["to_port"] != int32(443) {
		t.Errorf("expected to_port 443, got %v", ingressRules[0]["to_port"])
	}

	cidrRanges, ok := ingressRules[0]["cidr_ranges"].([]string)
	if !ok {
		t.Fatal("cidr_ranges should be []string")
	}
	if len(cidrRanges) != 1 || cidrRanges[0] != "10.0.0.0/8" {
		t.Errorf("expected cidr_ranges [10.0.0.0/8], got %v", cidrRanges)
	}

	egressRules, ok := resource.Data["egress_rules"].([]map[string]interface{})
	if !ok {
		t.Fatal("egress_rules should be []map[string]interface{}")
	}
	if len(egressRules) != 1 {
		t.Fatalf("expected 1 egress rule, got %d", len(egressRules))
	}
	if egressRules[0]["protocol"] != "-1" {
		t.Errorf("expected protocol -1, got %s", egressRules[0]["protocol"])
	}
}

func TestMapSecurityGroupEmptyPermissions(t *testing.T) {
	provider := &Provider{region: "us-west-2"}

	sg := types.SecurityGroup{
		GroupId:              aws.String("sg-abc123"),
		GroupName:            aws.String("empty-sg"),
		VpcId:               aws.String("vpc-abc"),
		IpPermissions:       nil,
		IpPermissionsEgress: nil,
	}

	resource := provider.mapSecurityGroup(sg)

	if resource.Name != "empty-sg" {
		t.Errorf("expected name empty-sg, got %s", resource.Name)
	}

	ingressRules, _ := resource.Data["ingress_rules"].([]map[string]interface{})
	if len(ingressRules) != 0 {
		t.Errorf("expected empty ingress_rules, got %v", resource.Data["ingress_rules"])
	}
	egressRules, _ := resource.Data["egress_rules"].([]map[string]interface{})
	if len(egressRules) != 0 {
		t.Errorf("expected empty egress_rules, got %v", resource.Data["egress_rules"])
	}
}

func TestMapIPPermissions(t *testing.T) {
	tests := []struct {
		name  string
		perms []types.IpPermission
		want  int
	}{
		{
			name:  "empty",
			perms: nil,
			want:  0,
		},
		{
			name: "single rule",
			perms: []types.IpPermission{
				{
					IpProtocol: aws.String("tcp"),
					FromPort:  aws.Int32(22),
					ToPort:    aws.Int32(22),
					IpRanges: []types.IpRange{
						{CidrIp: aws.String("0.0.0.0/0")},
					},
				},
			},
			want: 1,
		},
		{
			name: "multiple rules",
			perms: []types.IpPermission{
				{IpProtocol: aws.String("tcp"), FromPort: aws.Int32(80), ToPort: aws.Int32(80)},
				{IpProtocol: aws.String("tcp"), FromPort: aws.Int32(443), ToPort: aws.Int32(443)},
			},
			want: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapIPPermissions(tt.perms)
			if tt.want == 0 {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
			} else if len(result) != tt.want {
				t.Errorf("expected %d rules, got %d", tt.want, len(result))
			}
		})
	}
}

func TestMapIPPermissionsIPv6(t *testing.T) {
	provider := &Provider{region: "us-east-1"}

	sg := types.SecurityGroup{
		GroupId:   aws.String("sg-ipv6"),
		GroupName: aws.String("ipv6-sg"),
		IpPermissions: []types.IpPermission{
			{
				IpProtocol: aws.String("tcp"),
				FromPort:   aws.Int32(443),
				ToPort:      aws.Int32(443),
				IpRanges: []types.IpRange{
					{CidrIp: aws.String("10.0.0.0/8")},
				},
				Ipv6Ranges: []types.Ipv6Range{
					{CidrIpv6: aws.String("2001:db8::/32")},
				},
			},
		},
	}

	resource := provider.mapSecurityGroup(sg)

	ingressRules := resource.Data["ingress_rules"].([]map[string]interface{})
	cidrRanges := ingressRules[0]["cidr_ranges"].([]string)

	if len(cidrRanges) != 2 {
		t.Errorf("expected 2 cidr ranges, got %d", len(cidrRanges))
	}
	if cidrRanges[0] != "10.0.0.0/8" {
		t.Errorf("expected first cidr 10.0.0.0/8, got %s", cidrRanges[0])
	}
	if cidrRanges[1] != "2001:db8::/32" {
		t.Errorf("expected second cidr 2001:db8::/32, got %s", cidrRanges[1])
	}
}

type mockSecurityGroupClient struct {
	securityGroups []types.SecurityGroup
	nextToken    *string
	err         error
}

func (m *mockSecurityGroupClient) DescribeSecurityGroups(ctx context.Context, params *ec2.DescribeSecurityGroupsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &ec2.DescribeSecurityGroupsOutput{
		SecurityGroups: m.securityGroups,
		NextToken:    m.nextToken,
	}, nil
}

func TestListSecurityGroups(t *testing.T) {
	provider := &Provider{
		region:   "us-east-1",
		sgClient: &mockSecurityGroupClient{
			securityGroups: []types.SecurityGroup{
				{
					GroupId:     aws.String("sg-001"),
					GroupName:  aws.String("sg-one"),
					VpcId:      aws.String("vpc-1"),
					Description: aws.String("First SG"),
				},
				{
					GroupId:     aws.String("sg-002"),
					GroupName:  aws.String("sg-two"),
					VpcId:      aws.String("vpc-1"),
					Description: aws.String("Second SG"),
				},
			},
		},
	}

	resources, err := provider.listSecurityGroups(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resources) != 2 {
		t.Errorf("expected 2 resources, got %d", len(resources))
	}

	if resources[0].ID != "sg-001" {
		t.Errorf("expected first resource ID sg-001, got %s", resources[0].ID)
	}
	if resources[1].ID != "sg-002" {
		t.Errorf("expected second resource ID sg-002, got %s", resources[1].ID)
	}
}

func TestListSecurityGroupsEmpty(t *testing.T) {
	provider := &Provider{
		region:   "us-east-1",
		sgClient: &mockSecurityGroupClient{
			securityGroups: nil,
		},
	}

	resources, err := provider.listSecurityGroups(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resources) != 0 {
		t.Errorf("expected 0 resources, got %d", len(resources))
	}
}