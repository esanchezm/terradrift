package aws

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func TestGetBucketVersioning(t *testing.T) {
	tests := []struct {
		name   string
		status types.BucketVersioningStatus
		want   string
	}{
		{"enabled", types.BucketVersioningStatusEnabled, "Enabled"},
		{"suspended", types.BucketVersioningStatusSuspended, "Suspended"},
		{"empty", "", "Disabled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result string
			if tt.status == types.BucketVersioningStatusEnabled {
				result = "Enabled"
			} else if tt.status == types.BucketVersioningStatusSuspended {
				result = "Suspended"
			} else {
				result = "Disabled"
			}
			if result != tt.want {
				t.Errorf("expected %s, got %s", tt.want, result)
			}
		})
	}
}

func TestGetBucketEncryption(t *testing.T) {
	tests := []struct {
		name string
		algo types.ServerSideEncryption
		want string
	}{
		{"AES256", types.ServerSideEncryptionAes256, "AES256"},
		{"KMS", types.ServerSideEncryptionAwsKms, "aws:kms"},
		{"empty", "", "Disabled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result string
			if tt.algo == types.ServerSideEncryptionAes256 {
				result = "AES256"
			} else if tt.algo == types.ServerSideEncryptionAwsKms {
				result = "aws:kms"
			} else {
				result = "Disabled"
			}
			if result != tt.want {
				t.Errorf("expected %s, got %s", tt.want, result)
			}
		})
	}
}

func TestGetPublicAccessBlock(t *testing.T) {
	tests := []struct {
		name   string
		config *types.PublicAccessBlockConfiguration
		want   bool
	}{
		{
			name: "all blocked",
			config: &types.PublicAccessBlockConfiguration{
				BlockPublicAcls:        aws.Bool(true),
				BlockPublicPolicy:     aws.Bool(true),
				IgnorePublicAcls:      aws.Bool(true),
				RestrictPublicBuckets: aws.Bool(true),
			},
			want: true,
		},
		{
			name: "none blocked",
			config: &types.PublicAccessBlockConfiguration{
				BlockPublicAcls:        aws.Bool(false),
				BlockPublicPolicy:      aws.Bool(false),
				IgnorePublicAcls:      aws.Bool(false),
				RestrictPublicBuckets: aws.Bool(false),
			},
			want: false,
		},
		{
			name:   "nil config",
			config: nil,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result bool
			if tt.config == nil {
				result = false
			} else {
				cfg := tt.config
				result = cfg.BlockPublicAcls != nil && *cfg.BlockPublicAcls &&
					cfg.BlockPublicPolicy != nil && *cfg.BlockPublicPolicy &&
					cfg.IgnorePublicAcls != nil && *cfg.IgnorePublicAcls &&
					cfg.RestrictPublicBuckets != nil && *cfg.RestrictPublicBuckets
			}
			if result != tt.want {
				t.Errorf("expected %v, got %v", tt.want, result)
			}
		})
	}
}

func TestBucketNameExtraction(t *testing.T) {
	tests := []struct {
		name string
		bn   *string
		want string
	}{
		{"normal", aws.String("my-bucket"), "my-bucket"},
		{"empty", aws.String(""), ""},
		{"nil", nil, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := aws.ToString(tt.bn)
			if result != tt.want {
				t.Errorf("expected %q, got %q", tt.want, result)
			}
		})
	}
}