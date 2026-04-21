package aws

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
)

func TestMapIAMRole(t *testing.T) {
	provider := &Provider{
		region: "us-east-1",
		iamClient: &mockIAMClient{
			attachedPolicies:  []types.AttachedPolicy{},
			inlinePolicyNames: []string{},
		},
	}

	createDate := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	assumePolicy := url.QueryEscape(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"sts:AssumeRole","Principal":{"Service":"ec2.amazonaws.com"}}]}`)

	role := types.Role{
		RoleId:                   aws.String("AROA1234567890ABCDEFG"),
		RoleName:                aws.String("test-role"),
		Path:                   aws.String("/path/to/role/"),
		Arn:                    aws.String("arn:aws:iam::123456789012:role/path/to/role/test-role"),
		CreateDate:             &createDate,
		Description:            aws.String("Test role description"),
		AssumeRolePolicyDocument: aws.String(assumePolicy),
	}

	resource, err := provider.mapIAMRole(context.Background(), role)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resource.ID != "AROA1234567890ABCDEFG" {
		t.Errorf("expected ID AROA1234567890ABCDEFG, got %s", resource.ID)
	}
	if resource.Type != ResourceTypeIAMRole {
		t.Errorf("expected type %s, got %s", ResourceTypeIAMRole, resource.Type)
	}
	if resource.Name != "test-role" {
		t.Errorf("expected name test-role, got %s", resource.Name)
	}
	if resource.Provider != "aws" {
		t.Errorf("expected provider aws, got %s", resource.Provider)
	}
	if resource.Region != "us-east-1" {
		t.Errorf("expected region us-east-1, got %s", resource.Region)
	}
	if resource.Data["path"] != "/path/to/role/" {
		t.Errorf("expected path /path/to/role/, got %s", resource.Data["path"])
	}
	if resource.Data["arn"] != "arn:aws:iam::123456789012:role/path/to/role/test-role" {
		t.Errorf("expected arn, got %s", resource.Data["arn"])
	}
	if resource.Data["unique_id"] != "AROA1234567890ABCDEFG" {
		t.Errorf("expected unique_id AROA1234567890ABCDEFG, got %s", resource.Data["unique_id"])
	}
	if resource.Data["description"] != "Test role description" {
		t.Errorf("expected description, got %s", resource.Data["description"])
	}

	assumePolicyData, ok := resource.Data["assume_role_policy"].(map[string]interface{})
	if !ok {
		t.Fatal("assume_role_policy should be map[string]interface{}")
	}
	if assumePolicyData["Version"] != "2012-10-17" {
		t.Errorf("expected version 2012-10-17, got %v", assumePolicyData["Version"])
	}
}

func TestMapIAMRoleEmpty(t *testing.T) {
	provider := &Provider{region: "us-west-2"}

	role := types.Role{
		RoleId:   aws.String("AROA123"),
		RoleName: aws.String(""),
	}

	resource, err := provider.mapIAMRole(context.Background(), role)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resource.ID != "" {
		t.Errorf("expected empty ID, got %s", resource.ID)
	}
}

func TestMapIAMRoleWithPolicies(t *testing.T) {
	provider := &Provider{
		region: "us-east-1",
		iamClient: &mockIAMClient{
			attachedPolicies: []types.AttachedPolicy{
				{
					PolicyName: aws.String("AmazonS3FullAccess"),
					PolicyArn:  aws.String("arn:aws:iam::aws:policy/AmazonS3FullAccess"),
				},
			},
			inlinePolicyNames: []string{"InlinePolicy1"},
			inlinePolicies: map[string]string{
				"InlinePolicy1": `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]}`,
			},
		},
	}

	role := types.Role{
		RoleId:   aws.String("AROA1234567890ABCDEFG"),
		RoleName: aws.String("role-with-policies"),
	}

	resource, err := provider.mapIAMRole(context.Background(), role)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	managedPolicies, ok := resource.Data["managed_policies"].([]map[string]interface{})
	if !ok {
		t.Fatal("managed_policies should be []map[string]interface{}")
	}
	if len(managedPolicies) != 1 {
		t.Fatalf("expected 1 managed policy, got %d", len(managedPolicies))
	}
	if managedPolicies[0]["policy_name"] != "AmazonS3FullAccess" {
		t.Errorf("expected policy_name AmazonS3FullAccess, got %s", managedPolicies[0]["policy_name"])
	}
	if managedPolicies[0]["policy_arn"] != "arn:aws:iam::aws:policy/AmazonS3FullAccess" {
		t.Errorf("expected policy_arn, got %s", managedPolicies[0]["policy_arn"])
	}

	inlinePolicies, ok := resource.Data["inline_policies"].([]map[string]interface{})
	if !ok {
		t.Fatal("inline_policies should be []map[string]interface{}")
	}
	if len(inlinePolicies) != 1 {
		t.Fatalf("expected 1 inline policy, got %d", len(inlinePolicies))
	}
	if inlinePolicies[0]["policy_name"] != "InlinePolicy1" {
		t.Errorf("expected policy_name InlinePolicy1, got %s", inlinePolicies[0]["policy_name"])
	}

	policyDoc, ok := inlinePolicies[0]["policy_document"].(map[string]interface{})
	if !ok {
		t.Fatal("policy_document should be map[string]interface{}")
	}
	if policyDoc["Version"] != "2012-10-17" {
		t.Errorf("expected version 2012-10-17, got %v", policyDoc["Version"])
	}
}

func TestListIAMRoles(t *testing.T) {
	provider := &Provider{
		region: "us-east-1",
		iamClient: &mockIAMClient{
			roles: []types.Role{
				{
					RoleId:   aws.String("AROA001"),
					RoleName: aws.String("role-one"),
				},
				{
					RoleId:   aws.String("AROA002"),
					RoleName: aws.String("role-two"),
				},
			},
		},
	}

	resources, err := provider.listIAMRoles(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resources) != 2 {
		t.Errorf("expected 2 resources, got %d", len(resources))
	}

	if resources[0].ID != "AROA001" {
		t.Errorf("expected first resource ID AROA001, got %s", resources[0].ID)
	}
	if resources[1].ID != "AROA002" {
		t.Errorf("expected second resource ID AROA002, got %s", resources[1].ID)
	}
}

func TestListIAMRolesPagination(t *testing.T) {
	provider := &Provider{
		region: "us-east-1",
		iamClient: &mockIAMClient{
			roles: []types.Role{
				{RoleId: aws.String("AROA001"), RoleName: aws.String("role-one")},
				{RoleId: aws.String("AROA002"), RoleName: aws.String("role-two")},
				{RoleId: aws.String("AROA003"), RoleName: aws.String("role-three")},
			},
			pageSize: 2,
		},
	}

	resources, err := provider.listIAMRoles(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resources) != 3 {
		t.Errorf("expected 3 resources, got %d", len(resources))
	}
}

func TestListIAMRolesEmpty(t *testing.T) {
	provider := &Provider{
		region: "us-east-1",
		iamClient: &mockIAMClient{
			roles: nil,
		},
	}

	resources, err := provider.listIAMRoles(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resources) != 0 {
		t.Errorf("expected 0 resources, got %d", len(resources))
	}
}

func TestGetManagedPolicies(t *testing.T) {
	provider := &Provider{
		region: "us-east-1",
		iamClient: &mockIAMClient{
			attachedPolicies: []types.AttachedPolicy{
				{PolicyName: aws.String("Policy1"), PolicyArn: aws.String("arn:1")},
				{PolicyName: aws.String("Policy2"), PolicyArn: aws.String("arn:2")},
			},
		},
	}

	policies, err := provider.getManagedPolicies(context.Background(), "test-role")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(policies) != 2 {
		t.Errorf("expected 2 policies, got %d", len(policies))
	}
}

func TestGetInlinePolicies(t *testing.T) {
	provider := &Provider{
		region: "us-east-1",
		iamClient: &mockIAMClient{
			inlinePolicyNames: []string{"InlinePolicy1"},
			inlinePolicies: map[string]string{
				"InlinePolicy1": `{"Version":"2012-10-17"}`,
			},
		},
	}

	policies, err := provider.getInlinePolicies(context.Background(), "test-role")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(policies) != 1 {
		t.Errorf("expected 1 policy, got %d", len(policies))
	}

	if policies[0]["policy_name"] != "InlinePolicy1" {
		t.Errorf("expected InlinePolicy1, got %s", policies[0]["policy_name"])
	}
}

func TestGetInlinePolicyDocument(t *testing.T) {
	provider := &Provider{
		region: "us-east-1",
		iamClient: &mockIAMClient{
			inlinePolicies: map[string]string{
				"MyPolicy": `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`,
			},
		},
	}

	doc, err := provider.getInlinePolicyDocument(context.Background(), "test-role", "MyPolicy")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if doc["Version"] != "2012-10-17" {
		t.Errorf("expected version 2012-10-17, got %v", doc["Version"])
	}
}

func TestGetInlinePolicyDocumentNotFound(t *testing.T) {
	provider := &Provider{
		region: "us-east-1",
		iamClient: &mockIAMClient{
			err: &testError{msg: "NoSuchEntityPolicy: policy not found"},
		},
	}

	doc, err := provider.getInlinePolicyDocument(context.Background(), "test-role", "NonExistent")
	if err == nil {
		t.Fatal("expected error for non-existent policy")
	}

	if doc != nil {
		t.Errorf("expected nil for error case, got %v", doc)
	}
}

func TestGetInlinePolicyDocumentError(t *testing.T) {
	provider := &Provider{
		region: "us-east-1",
		iamClient: &mockIAMClient{
			err: &testError{msg: "GetRolePolicy failed"},
		},
	}

	_, err := provider.getInlinePolicyDocument(context.Background(), "test-role", "MyPolicy")
	if err == nil {
		t.Fatal("expected error for GetRolePolicy failure")
	}
}

type mockIAMClient struct {
	roles              []types.Role
	nextMarker        *string
	isTruncated       bool
	attachedPolicies  []types.AttachedPolicy
	inlinePolicyNames []string
	inlinePolicies   map[string]string
	err             error
	pageSize        int32
}

func (m *mockIAMClient) ListRoles(ctx context.Context, params *iam.ListRolesInput, optFns ...func(*iam.Options)) (*iam.ListRolesOutput, error) {
	if m.err != nil {
		return nil, m.err
	}

	if len(m.roles) == 0 {
		return &iam.ListRolesOutput{
			Roles:      []types.Role{},
			IsTruncated: false,
		}, nil
	}

	pageSize := int32(100)
	if m.pageSize > 0 {
		pageSize = m.pageSize
	}
	if params.MaxItems != nil {
		pageSize = *params.MaxItems
	}
	start := 0
	if params.Marker != nil {
		for i, r := range m.roles {
			if aws.ToString(r.RoleId) == *params.Marker {
				start = i + 1
				break
			}
		}
	}

	end := start + int(pageSize)
	if end > len(m.roles) {
		end = len(m.roles)
	}

	page := m.roles[start:end]
	isTruncated := end < len(m.roles)

	var nextMarker *string
	if isTruncated && len(page) > 0 {
		nextMarker = page[len(page)-1].RoleId
	}

	return &iam.ListRolesOutput{
		Roles:      page,
		IsTruncated: isTruncated,
		Marker:    nextMarker,
	}, nil
}

func (m *mockIAMClient) ListAttachedRolePolicies(ctx context.Context, params *iam.ListAttachedRolePoliciesInput, optFns ...func(*iam.Options)) (*iam.ListAttachedRolePoliciesOutput, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &iam.ListAttachedRolePoliciesOutput{
		AttachedPolicies: m.attachedPolicies,
		IsTruncated:       false,
	}, nil
}

func (m *mockIAMClient) ListRolePolicies(ctx context.Context, params *iam.ListRolePoliciesInput, optFns ...func(*iam.Options)) (*iam.ListRolePoliciesOutput, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &iam.ListRolePoliciesOutput{
		PolicyNames: m.inlinePolicyNames,
	}, nil
}

func (m *mockIAMClient) GetRolePolicy(ctx context.Context, params *iam.GetRolePolicyInput, optFns ...func(*iam.Options)) (*iam.GetRolePolicyOutput, error) {
	if m.err != nil {
		return nil, m.err
	}

	policyName := aws.ToString(params.PolicyName)
	doc, exists := m.inlinePolicies[policyName]
	if !exists {
		return nil, &testError{msg: "NoSuchPolicy: policy not found"}
	}

	encodedDoc := url.QueryEscape(doc)
	return &iam.GetRolePolicyOutput{
		PolicyDocument: aws.String(encodedDoc),
	}, nil
}

var _ iamAPI = (*mockIAMClient)(nil)

func TestAssumeRolePolicyParsing(t *testing.T) {
	originalDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":["ec2.amazonaws.com","lambda.amazonaws.com"]}}]}`
	encoded := url.QueryEscape(originalDoc)

	provider := &Provider{
		region: "us-east-1",
		iamClient: &mockIAMClient{
			attachedPolicies:   []types.AttachedPolicy{},
			inlinePolicyNames: []string{},
		},
	}
	role := types.Role{
		RoleId:                   aws.String("AROA123"),
		RoleName:                aws.String("test-role"),
		AssumeRolePolicyDocument: aws.String(encoded),
	}

	resource, err := provider.mapIAMRole(context.Background(), role)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assumePolicy, ok := resource.Data["assume_role_policy"].(map[string]interface{})
	if !ok {
		t.Fatal("assume_role_policy parsing failed")
	}

	stmt, ok := assumePolicy["Statement"].([]interface{})
	if !ok {
		t.Fatal("Statement should be array")
	}

	principal := stmt[0].(map[string]interface{})["Principal"].(map[string]interface{})
	services := principal["Service"].([]interface{})

	if len(services) != 2 {
		t.Errorf("expected 2 services, got %d", len(services))
	}
}

var _, _ = json.Marshal, url.QueryEscape