// Package s3 implements a StateReader for Terraform state stored in Amazon S3.
package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"

	"github.com/esanchezm/terradrift/internal/core"
	"github.com/esanchezm/terradrift/internal/state"
)

// API is the minimal S3 interface used by Reader. Defining our own interface
// (instead of taking *s3.Client directly) enables unit tests to pass a mock
// without constructing a real AWS client.
type API interface {
	GetObject(ctx context.Context, params *awss3.GetObjectInput, optFns ...func(*awss3.Options)) (*awss3.GetObjectOutput, error)
}

// Reader implements state.StateReader for a Terraform state file stored at
// s3://bucket/key.
type Reader struct {
	client API
	bucket string
	key    string
}

// New constructs a Reader from an explicit bucket and key.
func New(client API, bucket, key string) *Reader {
	return &Reader{client: client, bucket: bucket, key: key}
}

// NewFromURI constructs a Reader from an s3://bucket/key URI.
func NewFromURI(client API, uri string) (*Reader, error) {
	bucket, key, err := parseURI(uri)
	if err != nil {
		return nil, err
	}
	return New(client, bucket, key), nil
}

// DefaultClient loads AWS configuration from the default credential chain
// (environment, shared config, EC2/ECS metadata) and returns a production
// S3 client.
func DefaultClient(ctx context.Context) (*awss3.Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}
	return awss3.NewFromConfig(cfg), nil
}

// Resources fetches the state object from S3 and parses it.
func (r *Reader) Resources(ctx context.Context) ([]core.Resource, error) {
	out, err := r.client.GetObject(ctx, &awss3.GetObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(r.key),
	})
	if err != nil {
		return nil, r.classifyError(err)
	}
	defer out.Body.Close()

	data, err := io.ReadAll(io.LimitReader(out.Body, state.MaxStateSize))
	if err != nil {
		return nil, fmt.Errorf("reading S3 object body s3://%s/%s: %w", r.bucket, r.key, err)
	}

	resources, err := state.ParseState(data)
	if err != nil {
		return nil, fmt.Errorf("parsing state from s3://%s/%s: %w", r.bucket, r.key, err)
	}
	return resources, nil
}

// Source returns the s3://bucket/key URI.
func (r *Reader) Source() string {
	return fmt.Sprintf("s3://%s/%s", r.bucket, r.key)
}

// classifyError wraps an S3 GetObject error with a user-oriented message that
// distinguishes missing object, missing bucket, and authentication failures.
func (r *Reader) classifyError(err error) error {
	var noKey *types.NoSuchKey
	if errors.As(err, &noKey) {
		return fmt.Errorf("S3 object not found s3://%s/%s: %w", r.bucket, r.key, err)
	}

	var noBucket *types.NoSuchBucket
	if errors.As(err, &noBucket) {
		return fmt.Errorf("S3 bucket not found %q: %w", r.bucket, err)
	}

	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "AccessDenied", "InvalidAccessKeyId", "ExpiredToken", "SignatureDoesNotMatch":
			return fmt.Errorf("S3 authentication error for s3://%s/%s: %w", r.bucket, r.key, err)
		}
	}

	return fmt.Errorf("fetching s3://%s/%s: %w", r.bucket, r.key, err)
}

// parseURI parses an s3://bucket/key URI. Returns an error for a malformed
// URI, empty bucket, or empty key.
func parseURI(uri string) (bucket, key string, err error) {
	if !strings.HasPrefix(uri, "s3://") {
		return "", "", fmt.Errorf("invalid S3 URI %q: must start with s3://", uri)
	}
	rest := strings.TrimPrefix(uri, "s3://")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 || parts[0] == "" {
		return "", "", fmt.Errorf("invalid S3 URI %q: expected s3://bucket/key", uri)
	}
	if parts[1] == "" {
		return "", "", fmt.Errorf("invalid S3 URI %q: key must not be empty", uri)
	}
	return parts[0], parts[1], nil
}

// Compile-time check that Reader satisfies the StateReader interface.
var _ state.StateReader = (*Reader)(nil)
