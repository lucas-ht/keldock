// Package awsprobe verifies actual AWS state using short-lived, exact service-account identities.
// It never loads a local operator profile or static installation access keys.
package awsprobe

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
	"github.com/lucas-ht/keldock/internal/cloud"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
)

// Client obtains projected tokens only for configured provisioning subjects.
type Client struct{ Kubernetes kubernetes.Interface }

// Config exchanges an exact Kubernetes subject for AWS credentials kept in memory only.
func (client *Client) Config(ctx context.Context, connection cloud.Connection, identity cloud.IdentityProfile, service string) (aws.Config, error) {
	role, ok := identity.Roles[service]
	if !ok || identity.Account != connection.Account {
		return aws.Config{}, fmt.Errorf("unapproved cloud identity")
	}
	token, err := client.Kubernetes.CoreV1().ServiceAccounts(identity.Namespace).CreateToken(ctx, identity.ServiceAccounts[service], &authenticationv1.TokenRequest{Spec: authenticationv1.TokenRequestSpec{Audiences: []string{"sts.amazonaws.com"}, ExpirationSeconds: ptr.To(int64(900))}}, metav1.CreateOptions{})
	if err != nil {
		return aws.Config{}, fmt.Errorf("provider subject token unavailable")
	}
	configuration := aws.Config{Region: connection.Region, Credentials: aws.AnonymousCredentials{}}
	session, err := sts.NewFromConfig(configuration).AssumeRoleWithWebIdentity(ctx, &sts.AssumeRoleWithWebIdentityInput{RoleArn: aws.String(role), RoleSessionName: aws.String("keldock-observation"), WebIdentityToken: aws.String(token.Status.Token), DurationSeconds: aws.Int32(900)})
	if err != nil {
		return aws.Config{}, err
	}
	if session.Credentials == nil || session.Credentials.Expiration == nil || time.Until(*session.Credentials.Expiration) < time.Minute {
		return aws.Config{}, fmt.Errorf("invalid temporary AWS session")
	}
	configuration.Credentials = aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(aws.ToString(session.Credentials.AccessKeyId), aws.ToString(session.Credentials.SecretAccessKey), aws.ToString(session.Credentials.SessionToken)))
	caller, err := sts.NewFromConfig(configuration).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return aws.Config{}, err
	}
	if aws.ToString(caller.Account) != connection.Account {
		return aws.Config{}, fmt.Errorf("unexpected AWS account")
	}
	return configuration, nil
}

// Reason bounds failure details and never reflects request bodies, tokens or SDK logs.
func Reason(err error) string {
	var api smithy.APIError
	if errors.As(err, &api) {
		code := api.ErrorCode()
		if len(code) > 0 && len(code) < 80 {
			return "AWS_" + code
		}
	}
	return "CloudObservationUnavailable"
}
