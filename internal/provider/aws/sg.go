package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/esanchezm/terradrift/internal/core"
)

type securityGroupAPI interface {
	DescribeSecurityGroups(ctx context.Context, params *ec2.DescribeSecurityGroupsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error)
}

func (p *Provider) listSecurityGroups(ctx context.Context) ([]core.Resource, error) {
	var resources []core.Resource
	var cursor *string

	for {
		input := &ec2.DescribeSecurityGroupsInput{
			MaxResults: aws.Int32(pageSize),
		}
		if cursor != nil {
			input.NextToken = cursor
		}

		var output *ec2.DescribeSecurityGroupsOutput
		var err error
		for attempt := 0; attempt < maxRetries; attempt++ {
			output, err = p.sgClient.DescribeSecurityGroups(ctx, input)
			if err != nil {
				if isThrottleError(err) {
					sleepWithBackoff(attempt)
					continue
				}
				return nil, err
			}
			break
		}
		if output == nil {
			break
		}

		for _, sg := range output.SecurityGroups {
			resources = append(resources, p.mapSecurityGroup(sg))
		}

		cursor = output.NextToken
		if cursor == nil {
			break
		}
	}

	return resources, nil
}

func (p *Provider) mapSecurityGroup(sg types.SecurityGroup) core.Resource {
	data := make(map[string]interface{})

	data["group_name"] = aws.ToString(sg.GroupName)
	data["vpc_id"] = aws.ToString(sg.VpcId)
	data["description"] = aws.ToString(sg.Description)
	data["owner_id"] = aws.ToString(sg.OwnerId)

	data["ingress_rules"] = mapIPPermissions(sg.IpPermissions)
	data["egress_rules"] = mapIPPermissions(sg.IpPermissionsEgress)

	return core.Resource{
		ID:       aws.ToString(sg.GroupId),
		Type:     ResourceTypeSecurityGroup,
		Name:     aws.ToString(sg.GroupName),
		Provider: "aws",
		Region:   p.region,
		Data:     data,
	}
}

func mapIPPermissions(perms []types.IpPermission) []map[string]interface{} {
	if len(perms) == 0 {
		return nil
	}

	var rules []map[string]interface{}
	for _, perm := range perms {
		rule := make(map[string]interface{})

		rule["protocol"] = aws.ToString(perm.IpProtocol)

		if perm.FromPort != nil {
			rule["from_port"] = aws.ToInt32(perm.FromPort)
		}
		if perm.ToPort != nil {
			rule["to_port"] = aws.ToInt32(perm.ToPort)
		}

		var cidrRanges []string
		for _, ipRange := range perm.IpRanges {
			cidrRanges = append(cidrRanges, aws.ToString(ipRange.CidrIp))
		}
		for _, ipv6Range := range perm.Ipv6Ranges {
			cidrRanges = append(cidrRanges, aws.ToString(ipv6Range.CidrIpv6))
		}
		rule["cidr_ranges"] = cidrRanges

		rules = append(rules, rule)
	}

	return rules
}