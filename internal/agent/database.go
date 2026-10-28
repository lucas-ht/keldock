package agent

import "context"

// DatabaseClientFromFile shares only transport and rotation, not cloud/registrar authority.
func DatabaseClientFromFile(endpoint, path string, development bool) (*Client, error) {
	client, err := ClientFromFile(endpoint, path, development)
	if err == nil {
		client.database = true
	}
	return client, err
}

// DatabaseRPC is fixed to the separately authenticated database agent routes.
func (client *Client) DatabaseRPC(ctx context.Context, method, suffix string, input, output any) error {
	if !client.database {
		return APIError{Status: 403}
	}
	return client.request(ctx, method, "/v1/database/agent/"+suffix, input, output)
}
