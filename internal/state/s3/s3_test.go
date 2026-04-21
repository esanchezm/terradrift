package s3

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

type fakeS3 struct {
	getObject func(ctx context.Context, params *awss3.GetObjectInput, optFns ...func(*awss3.Options)) (*awss3.GetObjectOutput, error)
}

func (f *fakeS3) GetObject(ctx context.Context, params *awss3.GetObjectInput, optFns ...func(*awss3.Options)) (*awss3.GetObjectOutput, error) {
	return f.getObject(ctx, params, optFns...)
}

func objectOutput(body []byte) *awss3.GetObjectOutput {
	return &awss3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(body))}
}

func TestResources_ValidState(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "valid.tfstate"))
	if err != nil {
		t.Fatalf("reading testdata: %v", err)
	}

	mock := &fakeS3{
		getObject: func(ctx context.Context, params *awss3.GetObjectInput, optFns ...func(*awss3.Options)) (*awss3.GetObjectOutput, error) {
			return objectOutput(data), nil
		},
	}

	r := New(mock, "my-bucket", "terraform.tfstate")
	resources, err := r.Resources(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := len(resources); got != 5 {
		t.Fatalf("expected 5 resources, got %d", got)
	}

	want := []struct {
		id string
	}{
		{"i-1234567890abcdef0"},
		{"my-assets-bucket"},
		{"sg-0123456789abcdef0"},
		{"vpc-abcdef01"},
		{"projects/my-project/zones/us-central1-a/instances/vm"},
	}

	for i, w := range want {
		if got := resources[i].ID; got != w.id {
			t.Errorf("resource[%d].ID = %q, want %q", i, got, w.id)
		}
	}
}

func TestResources_NoSuchKey(t *testing.T) {
	mock := &fakeS3{
		getObject: func(ctx context.Context, params *awss3.GetObjectInput, optFns ...func(*awss3.Options)) (*awss3.GetObjectOutput, error) {
			return nil, &types.NoSuchKey{}
		},
	}

	r := New(mock, "my-bucket", "missing.tfstate")
	_, err := r.Resources(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "S3 object not found") {
		t.Errorf("error %q does not contain %q", err.Error(), "S3 object not found")
	}
}

func TestResources_NoSuchBucket(t *testing.T) {
	mock := &fakeS3{
		getObject: func(ctx context.Context, params *awss3.GetObjectInput, optFns ...func(*awss3.Options)) (*awss3.GetObjectOutput, error) {
			return nil, &types.NoSuchBucket{}
		},
	}

	r := New(mock, "no-bucket", "terraform.tfstate")
	_, err := r.Resources(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "S3 bucket not found") {
		t.Errorf("error %q does not contain %q", err.Error(), "S3 bucket not found")
	}
}

func TestResources_AccessDenied(t *testing.T) {
	mock := &fakeS3{
		getObject: func(ctx context.Context, params *awss3.GetObjectInput, optFns ...func(*awss3.Options)) (*awss3.GetObjectOutput, error) {
			return nil, &smithy.GenericAPIError{Code: "AccessDenied", Message: "access denied"}
		},
	}

	r := New(mock, "my-bucket", "terraform.tfstate")
	_, err := r.Resources(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "authentication error") {
		t.Errorf("error %q does not contain %q", err.Error(), "authentication error")
	}
}

func TestResources_MalformedJSON(t *testing.T) {
	mock := &fakeS3{
		getObject: func(ctx context.Context, params *awss3.GetObjectInput, optFns ...func(*awss3.Options)) (*awss3.GetObjectOutput, error) {
			return objectOutput([]byte("{not json")), nil
		},
	}

	r := New(mock, "my-bucket", "terraform.tfstate")
	_, err := r.Resources(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "parsing state") {
		t.Errorf("error %q does not contain %q", err.Error(), "parsing state")
	}
}

func TestResources_ContextCanceled(t *testing.T) {
	mock := &fakeS3{
		getObject: func(ctx context.Context, params *awss3.GetObjectInput, optFns ...func(*awss3.Options)) (*awss3.GetObjectOutput, error) {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return objectOutput([]byte("{}")), nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	r := New(mock, "my-bucket", "terraform.tfstate")
	_, err := r.Resources(ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestParseURI(t *testing.T) {
	cases := []struct {
		uri        string
		wantBucket string
		wantKey    string
		wantErr    bool
	}{
		{"s3://b/k", "b", "k", false},
		{"s3://b/path/to/k", "b", "path/to/k", false},
		{"http://b/k", "", "", true},
		{"s3://", "", "", true},
		{"s3:///k", "", "", true},
		{"s3://b", "", "", true},
		{"s3://b/", "", "", true},
	}

	for _, tc := range cases {
		bucket, key, err := parseURI(tc.uri)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseURI(%q): expected error, got nil", tc.uri)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseURI(%q): unexpected error: %v", tc.uri, err)
			continue
		}
		if bucket != tc.wantBucket {
			t.Errorf("parseURI(%q): bucket = %q, want %q", tc.uri, bucket, tc.wantBucket)
		}
		if key != tc.wantKey {
			t.Errorf("parseURI(%q): key = %q, want %q", tc.uri, key, tc.wantKey)
		}
	}
}

func TestNewFromURI(t *testing.T) {
	mock := &fakeS3{}

	r, err := NewFromURI(mock, "s3://my-bucket/path/to/state.tfstate")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.bucket != "my-bucket" {
		t.Errorf("bucket = %q, want %q", r.bucket, "my-bucket")
	}
	if r.key != "path/to/state.tfstate" {
		t.Errorf("key = %q, want %q", r.key, "path/to/state.tfstate")
	}

	_, err = NewFromURI(mock, "not-an-s3-uri")
	if err == nil {
		t.Error("expected error for invalid URI, got nil")
	}
}

func TestSource(t *testing.T) {
	mock := &fakeS3{}
	r := New(mock, "b", "path/x")

	if got := r.Source(); got != "s3://b/path/x" {
		t.Errorf("Source() = %q, want %q", got, "s3://b/path/x")
	}
}
