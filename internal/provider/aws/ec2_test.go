package aws

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func TestMapEC2Instance(t *testing.T) {
	provider := &Provider{region: "us-east-1"}

	instance := types.Instance{
		InstanceId:     aws.String("i-1234567890abcdef0"),
		InstanceType:  types.InstanceTypeT2Micro,
		State:          &types.InstanceState{Name: types.InstanceStateNameRunning},
		VpcId:          aws.String("vpc-12345"),
		SubnetId:       aws.String("subnet-12345"),
		ImageId:        aws.String("ami-12345678"),
		PrivateIpAddress: aws.String("10.0.0.10"),
		Tags: []types.Tag{
			{Key: aws.String("Name"), Value: aws.String("test-instance")},
			{Key: aws.String("Environment"), Value: aws.String("test")},
		},
	}

	resource := provider.mapEC2Instance(instance)

	if resource.ID != "i-1234567890abcdef0" {
		t.Errorf("expected ID i-1234567890abcdef0, got %s", resource.ID)
	}
	if resource.Type != ResourceTypeEC2 {
		t.Errorf("expected type %s, got %s", ResourceTypeEC2, resource.Type)
	}
	if resource.Name != "test-instance" {
		t.Errorf("expected name test-instance, got %s", resource.Name)
	}
	if resource.Provider != "aws" {
		t.Errorf("expected provider aws, got %s", resource.Provider)
	}
	if resource.Region != "us-east-1" {
		t.Errorf("expected region us-east-1, got %s", resource.Region)
	}
	if resource.Data["instance_type"] != "t2.micro" {
		t.Errorf("expected instance_type t2.micro, got %s", resource.Data["instance_type"])
	}
	if resource.Data["state"] != "running" {
		t.Errorf("expected state running, got %s", resource.Data["state"])
	}
	if resource.Data["vpc_id"] != "vpc-12345" {
		t.Errorf("expected vpc_id vpc-12345, got %s", resource.Data["vpc_id"])
	}

	tags, ok := resource.Data["tags"].(map[string]string)
	if !ok {
		t.Fatal("tags should be map[string]string")
	}
	if tags["Name"] != "test-instance" {
		t.Errorf("expected tag Name=test-instance, got %s", tags["Name"])
	}
	if tags["Environment"] != "test" {
		t.Errorf("expected tag Environment=test, got %s", tags["Environment"])
	}
}

func TestMapEC2InstanceNoNameTag(t *testing.T) {
	provider := &Provider{region: "us-west-2"}

	instance := types.Instance{
		InstanceId:    aws.String("i-abc123"),
		InstanceType: types.InstanceTypeT3Medium,
		State:        &types.InstanceState{Name: types.InstanceStateNameStopped},
		Tags:         []types.Tag{},
	}

	resource := provider.mapEC2Instance(instance)

	if resource.Name != "" {
		t.Errorf("expected empty name, got %s", resource.Name)
	}
	if resource.Data["state"] != "stopped" {
		t.Errorf("expected state stopped, got %s", resource.Data["state"])
	}
	if resource.Data["instance_type"] != "t3.medium" {
		t.Errorf("expected instance_type t3.medium, got %s", resource.Data["instance_type"])
	}
}

func TestIsThrottleError(t *testing.T) {
	tests := []struct {
		name     string
		err     error
		expected bool
	}{
		{"nil", nil, false},
		{"throttling", &testError{msg: "ThrottlingError: Rate exceeded"}, true},
		{"request limit", &testError{msg: "RequestLimitExceeded"}, true},
		{"other error", &testError{msg: "Some other error"}, false},
		{"empty", &testError{msg: ""}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isThrottleError(tt.err)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}