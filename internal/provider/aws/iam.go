package aws

import (
	"context"
	"encoding/json"
	"log"
	"net/url"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/esanchezm/terradrift/internal/core"
)

type iamAPI interface {
	ListRoles(ctx context.Context, params *iam.ListRolesInput, optFns ...func(*iam.Options)) (*iam.ListRolesOutput, error)
	ListAttachedRolePolicies(ctx context.Context, params *iam.ListAttachedRolePoliciesInput, optFns ...func(*iam.Options)) (*iam.ListAttachedRolePoliciesOutput, error)
	ListRolePolicies(ctx context.Context, params *iam.ListRolePoliciesInput, optFns ...func(*iam.Options)) (*iam.ListRolePoliciesOutput, error)
	GetRolePolicy(ctx context.Context, params *iam.GetRolePolicyInput, optFns ...func(*iam.Options)) (*iam.GetRolePolicyOutput, error)
}

func (p *Provider) listIAMRoles(ctx context.Context) ([]core.Resource, error) {
	var resources []core.Resource
	var marker *string

	for {
		input := &iam.ListRolesInput{
			MaxItems: aws.Int32(pageSize),
		}
		if marker != nil {
			input.Marker = marker
		}

		var output *iam.ListRolesOutput
		var err error
		for attempt := 0; attempt < maxRetries; attempt++ {
			output, err = p.iamClient.ListRoles(ctx, input)
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

		for _, role := range output.Roles {
			resource, err := p.mapIAMRole(ctx, role)
			if err != nil {
				log.Printf("failed to enrich role %s: %v", aws.ToString(role.RoleName), err)
				continue
			}
			if resource.ID != "" {
				resources = append(resources, resource)
			}
		}

		if !output.IsTruncated {
			break
		}
		marker = output.Marker
	}

	return resources, nil
}

func (p *Provider) mapIAMRole(ctx context.Context, role types.Role) (core.Resource, error) {
	roleName := aws.ToString(role.RoleName)
	if roleName == "" {
		return core.Resource{}, nil
	}

	data := make(map[string]interface{})
	data["role_name"] = roleName
	data["path"] = aws.ToString(role.Path)
	data["arn"] = aws.ToString(role.Arn)
	data["unique_id"] = aws.ToString(role.RoleId)
	data["create_date"] = role.CreateDate
	data["description"] = aws.ToString(role.Description)

	if role.AssumeRolePolicyDocument != nil {
		doc, err := url.QueryUnescape(aws.ToString(role.AssumeRolePolicyDocument))
		if err != nil {
			log.Printf("failed to decode assume role policy for %s: %v", roleName, err)
		} else {
			var parsed map[string]interface{}
			if err := json.Unmarshal([]byte(doc), &parsed); err != nil {
				log.Printf("failed to parse assume role policy JSON for %s: %v", roleName, err)
			} else {
				data["assume_role_policy"] = parsed
			}
		}
	}

	managedPolicies, err := p.getManagedPolicies(ctx, roleName)
	if err != nil {
		log.Printf("failed to get managed policies for %s: %v", roleName, err)
	} else {
		data["managed_policies"] = managedPolicies
	}

	inlinePolicies, err := p.getInlinePolicies(ctx, roleName)
	if err != nil {
		log.Printf("failed to get inline policies for %s: %v", roleName, err)
	} else {
		data["inline_policies"] = inlinePolicies
	}

	return core.Resource{
		ID:       aws.ToString(role.RoleId),
		Type:     ResourceTypeIAMRole,
		Name:     roleName,
		Provider: "aws",
		Region:   p.region,
		Data:     data,
	}, nil
}

func (p *Provider) getManagedPolicies(ctx context.Context, roleName string) ([]map[string]interface{}, error) {
	var policies []map[string]interface{}
	var marker *string

	for {
		input := &iam.ListAttachedRolePoliciesInput{
			RoleName: aws.String(roleName),
			MaxItems: aws.Int32(pageSize),
		}
		if marker != nil {
			input.Marker = marker
		}

		output, err := p.iamClient.ListAttachedRolePolicies(ctx, input)
		if err != nil {
			return nil, err
		}

		for _, policy := range output.AttachedPolicies {
			policies = append(policies, map[string]interface{}{
				"policy_name": aws.ToString(policy.PolicyName),
				"policy_arn":  aws.ToString(policy.PolicyArn),
			})
		}

		if !output.IsTruncated {
			break
		}
		marker = output.Marker
	}

	return policies, nil
}

func (p *Provider) getInlinePolicies(ctx context.Context, roleName string) ([]map[string]interface{}, error) {
	var policies []map[string]interface{}

	output, err := p.iamClient.ListRolePolicies(ctx, &iam.ListRolePoliciesInput{
		RoleName: aws.String(roleName),
	})
	if err != nil {
		return nil, err
	}

	for _, policyName := range output.PolicyNames {
		policyDoc, err := p.getInlinePolicyDocument(ctx, roleName, policyName)
		if err != nil {
			log.Printf("failed to get inline policy %s for role %s: %v", policyName, roleName, err)
			policies = append(policies, map[string]interface{}{
				"policy_name":       policyName,
				"policy_document":   nil,
			})
			continue
		}

		policies = append(policies, map[string]interface{}{
			"policy_name":     policyName,
			"policy_document": policyDoc,
		})
	}

	return policies, nil
}

func (p *Provider) getInlinePolicyDocument(ctx context.Context, roleName, policyName string) (map[string]interface{}, error) {
	output, err := p.iamClient.GetRolePolicy(ctx, &iam.GetRolePolicyInput{
		RoleName:   aws.String(roleName),
		PolicyName: aws.String(policyName),
	})
	if err != nil {
		return nil, err
	}

	if output.PolicyDocument == nil {
		return nil, nil
	}

	doc, err := url.QueryUnescape(aws.ToString(output.PolicyDocument))
	if err != nil {
		return nil, err
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(doc), &parsed); err != nil {
		return nil, err
	}

	return parsed, nil
}