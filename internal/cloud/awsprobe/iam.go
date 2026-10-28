package awsprobe

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
)

// Role verifies the current boundary, exact trust and inline policy, independently of Ready conditions.
func Role(ctx context.Context, configuration aws.Config, arn, boundary, trust, policyName, policy string) error {
	name := arn[strings.LastIndex(arn, "/")+1:]
	client := iam.NewFromConfig(configuration)
	role, err := client.GetRole(ctx, &iam.GetRoleInput{RoleName: aws.String(name)})
	if err != nil {
		return err
	}
	if role.Role == nil || aws.ToString(role.Role.Arn) != arn || role.Role.PermissionsBoundary == nil || aws.ToString(role.Role.PermissionsBoundary.PermissionsBoundaryArn) != boundary {
		return fmt.Errorf("consumer role identity or boundary differs")
	}
	if !sameDocument(aws.ToString(role.Role.AssumeRolePolicyDocument), trust) {
		return fmt.Errorf("consumer trust has not converged")
	}
	inline, err := client.GetRolePolicy(ctx, &iam.GetRolePolicyInput{RoleName: aws.String(name), PolicyName: aws.String(policyName)})
	if err != nil {
		return err
	}
	if !sameDocument(aws.ToString(inline.PolicyDocument), policy) {
		return fmt.Errorf("consumer policy has not converged")
	}
	return nil
}

func sameDocument(actual, expected string) bool {
	if decoded, err := url.QueryUnescape(actual); err == nil && strings.HasPrefix(decoded, "{") {
		actual = decoded
	}
	var a, b any
	if json.Unmarshal([]byte(actual), &a) != nil || json.Unmarshal([]byte(expected), &b) != nil {
		return false
	}
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return string(left) == string(right)
}
