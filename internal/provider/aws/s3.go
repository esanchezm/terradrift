package aws

import (
	"context"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/esanchezm/terradrift/internal/core"
)

func (p *Provider) listS3Buckets(ctx context.Context) ([]core.Resource, error) {
	var resources []core.Resource

	output, err := p.s3Client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return nil, err
	}

	if output.Buckets == nil {
		return resources, nil
	}

	for _, bucket := range output.Buckets {
		resource, err := p.mapS3Bucket(ctx, bucket)
		if err != nil {
			log.Printf("failed to get bucket %s: %v", aws.ToString(bucket.Name), err)
			continue
		}
		resources = append(resources, resource)
	}

	return resources, nil
}

func (p *Provider) mapS3Bucket(ctx context.Context, bucket types.Bucket) (core.Resource, error) {
	bucketName := aws.ToString(bucket.Name)
	if bucketName == "" {
		return core.Resource{}, nil
	}

	data := make(map[string]interface{})
	data["name"] = bucketName

	region, err := p.getBucketRegion(ctx, bucketName)
	if err != nil {
		log.Printf("failed to get region for bucket %s: %v", bucketName, err)
	} else {
		data["region"] = region
	}

	versioning, err := p.getBucketVersioning(ctx, bucketName)
	if err != nil {
		log.Printf("failed to get versioning for bucket %s: %v", bucketName, err)
	} else {
		data["versioning_status"] = versioning
	}

	encryption, err := p.getBucketEncryption(ctx, bucketName)
	if err != nil {
		log.Printf("failed to get encryption for bucket %s: %v", bucketName, err)
	} else {
		data["encryption"] = encryption
	}

	publicAccessBlocked, err := p.getPublicAccessBlock(ctx, bucketName)
	if err != nil {
		log.Printf("failed to get public access block for bucket %s: %v", bucketName, err)
	} else {
		data["public_access_blocked"] = publicAccessBlocked
	}

	return core.Resource{
		ID:       bucketName,
		Type:     ResourceTypeS3Bucket,
		Name:     bucketName,
		Provider: "aws",
		Region:   region,
		Data:     data,
	}, nil
}

func (p *Provider) getBucketRegion(ctx context.Context, bucketName string) (string, error) {
	output, err := p.s3Client.GetBucketLocation(ctx, &s3.GetBucketLocationInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		return "", err
	}

	region := string(output.LocationConstraint)
	if region == "" {
		region = "us-east-1"
	}
	return region, nil
}

func (p *Provider) getBucketVersioning(ctx context.Context, bucketName string) (string, error) {
	output, err := p.s3Client.GetBucketVersioning(ctx, &s3.GetBucketVersioningInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		return "", err
	}

	if output.Status == types.BucketVersioningStatusEnabled {
		return "Enabled", nil
	}
	if output.Status == types.BucketVersioningStatusSuspended {
		return "Suspended", nil
	}
	return "Disabled", nil
}

func (p *Provider) getBucketEncryption(ctx context.Context, bucketName string) (string, error) {
	output, err := p.s3Client.GetBucketEncryption(ctx, &s3.GetBucketEncryptionInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		return "", err
	}

	if output.ServerSideEncryptionConfiguration == nil {
		return "Disabled", nil
	}

	rules := output.ServerSideEncryptionConfiguration.Rules
	if len(rules) == 0 || rules[0].ApplyServerSideEncryptionByDefault == nil {
		return "Disabled", nil
	}

	algo := rules[0].ApplyServerSideEncryptionByDefault.SSEAlgorithm
	if algo == types.ServerSideEncryptionAes256 {
		return "AES256", nil
	}
	if algo == types.ServerSideEncryptionAwsKms {
		return "aws:kms", nil
	}
	return string(algo), nil
}

func (p *Provider) getPublicAccessBlock(ctx context.Context, bucketName string) (bool, error) {
	output, err := p.s3Client.GetPublicAccessBlock(ctx, &s3.GetPublicAccessBlockInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		return false, err
	}

	if output.PublicAccessBlockConfiguration == nil {
		return false, nil
	}

	config := output.PublicAccessBlockConfiguration
	return config.BlockPublicAcls != nil && *config.BlockPublicAcls &&
		config.BlockPublicPolicy != nil && *config.BlockPublicPolicy &&
		config.IgnorePublicAcls != nil && *config.IgnorePublicAcls &&
		config.RestrictPublicBuckets != nil && *config.RestrictPublicBuckets, nil
}
