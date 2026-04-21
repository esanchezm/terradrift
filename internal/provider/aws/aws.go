// Package aws provides an AWS cloud provider implementation.
package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/esanchezm/terradrift/internal/core"
	"github.com/esanchezm/terradrift/internal/provider"
)

const (
	ResourceTypeEC2            = "aws_instance"
	ResourceTypeS3Bucket        = "aws_s3_bucket"
	ResourceTypeSecurityGroup   = "aws_security_group"
	ResourceTypeIAMRole        = "aws_iam_role"
)

type Provider struct {
	region    string
	ec2Client *ec2.Client
	s3Client  *s3.Client
	sgClient  securityGroupAPI
	iamClient iamAPI
}

// New creates a new AWS provider with the specified region.
// If region is empty, uses the default region from the AWS config.
func New(ctx context.Context, region string) (*Provider, error) {
	var cfg aws.Config
	var err error

	if region != "" {
		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(region),
		)
	} else {
		cfg, err = config.LoadDefaultConfig(ctx)
	}
	if err != nil {
		return nil, err
	}

	ec2Client := ec2.NewFromConfig(cfg)
	s3Client := s3.NewFromConfig(cfg)
	sgClient := ec2.NewFromConfig(cfg)
	iamClient := iam.NewFromConfig(cfg)

	return &Provider{
		region:    region,
		ec2Client: ec2Client,
		s3Client:  s3Client,
		sgClient:  sgClient,
		iamClient: iamClient,
	}, nil
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return "aws"
}

// SupportedTypes returns the supported resource types.
func (p *Provider) SupportedTypes() []string {
	return []string{ResourceTypeEC2, ResourceTypeS3Bucket, ResourceTypeSecurityGroup, ResourceTypeIAMRole}
}

// Resources returns the resources of the specified types.
func (p *Provider) Resources(ctx context.Context, types []string) ([]core.Resource, error) {
	// If no types specified, return all supported types
	if len(types) == 0 {
		types = p.SupportedTypes()
	}

	var resources []core.Resource

	for _, resourceType := range types {
		switch resourceType {
		case ResourceTypeEC2:
			instances, err := p.listEC2Instances(ctx)
			if err != nil {
				return nil, err
			}
			resources = append(resources, instances...)
		case ResourceTypeS3Bucket:
			buckets, err := p.listS3Buckets(ctx)
			if err != nil {
				return nil, err
			}
			resources = append(resources, buckets...)
		case ResourceTypeSecurityGroup:
			sgs, err := p.listSecurityGroups(ctx)
			if err != nil {
				return nil, err
			}
			resources = append(resources, sgs...)
		case ResourceTypeIAMRole:
			roles, err := p.listIAMRoles(ctx)
			if err != nil {
				return nil, err
			}
			resources = append(resources, roles...)
		}
	}

	return resources, nil
}

// Ensure Provider implements provider.Provider.
var _ provider.Provider = (*Provider)(nil)