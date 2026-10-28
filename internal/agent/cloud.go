package agent

import "context"

// CloudClientFromFile shares credential rotation and transport, with a fixed separate audience.
func CloudClientFromFile(endpoint, path string, development bool) (*Client, error) {
	client, err := ClientFromFile(endpoint, path, development)
	if err == nil {
		client.cloud = true
	}
	return client, err
}

// CloudRPC uses only the provisioning endpoint family and its independent agent identity.
func (client *Client) CloudRPC(ctx context.Context, method, suffix string, input, output any) error {
	if !client.cloud {
		return APIError{Status: 403}
	}
	return client.request(ctx, method, "/v1/cloud/agent/"+suffix, input, output)
}
