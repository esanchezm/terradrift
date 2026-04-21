package aws

import (
	"context"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/esanchezm/terradrift/internal/core"
)

const (
	maxRetries    = 3
	retryBackoff  = 300 * time.Millisecond
	pageSize     = 100
)

func (p *Provider) listEC2Instances(ctx context.Context) ([]core.Resource, error) {
	var resources []core.Resource
	var cursor *string

	for {
		input := &ec2.DescribeInstancesInput{
			MaxResults: aws.Int32(pageSize),
			Filters: []types.Filter{
				{
					Name:   aws.String("instance-state-code"),
					Values: []string{"16"},
				},
			},
		}
		if cursor != nil {
			input.NextToken = cursor
		}

		var output *ec2.DescribeInstancesOutput
		var err error
		for attempt := 0; attempt < maxRetries; attempt++ {
			output, err = p.ec2Client.DescribeInstances(ctx, input)
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

		for _, res := range output.Reservations {
			for _, instance := range res.Instances {
				resources = append(resources, p.mapEC2Instance(instance))
			}
		}

		cursor = output.NextToken
		if cursor == nil {
			break
		}
	}

	return resources, nil
}

func (p *Provider) mapEC2Instance(instance types.Instance) core.Resource {
	data := make(map[string]interface{})

	data["instance_id"] = aws.ToString(instance.InstanceId)
	data["instance_type"] = string(instance.InstanceType)
	data["state"] = string(instance.State.Name)
	data["vpc_id"] = aws.ToString(instance.VpcId)
	data["subnet_id"] = aws.ToString(instance.SubnetId)

	if instance.ImageId != nil {
		data["image_id"] = aws.ToString(instance.ImageId)
	}
	if instance.PrivateIpAddress != nil {
		data["private_ip"] = aws.ToString(instance.PrivateIpAddress)
	}
	if instance.PublicIpAddress != nil {
		data["public_ip"] = aws.ToString(instance.PublicIpAddress)
	}

	tags := make(map[string]string)
	for _, tag := range instance.Tags {
		if tag.Key != nil && tag.Value != nil {
			tags[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
		}
	}
	data["tags"] = tags

	name := ""
	if v, ok := tags["Name"]; ok {
		name = v
	}

	return core.Resource{
		ID:       aws.ToString(instance.InstanceId),
		Type:     ResourceTypeEC2,
		Name:     name,
		Provider: "aws",
		Region:   p.region,
		Data:     data,
	}
}

func isThrottleError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "Throttling") || strings.Contains(errStr, "RequestLimitExceeded")
}

func sleepWithBackoff(attempt int) {
	backoff := retryBackoff * time.Duration(attempt+1)
	time.Sleep(backoff)
}