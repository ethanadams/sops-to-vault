package main

import (
	"fmt"

	"github.com/hashicorp/vault/api"
)

type VaultClient struct {
	client     *api.Client
	mountPath  string
}

// NewVaultClient creates a new Vault client configured for KV v2.
func NewVaultClient(addr, token, mountPath string) (*VaultClient, error) {
	config := api.DefaultConfig()
	config.Address = addr

	client, err := api.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create vault client: %w", err)
	}

	client.SetToken(token)

	return &VaultClient{
		client:    client,
		mountPath: mountPath,
	}, nil
}

// WriteKVv2 writes a single secret value to a KV v2 path.
// The value is stored under the "value" key as a string.
func (v *VaultClient) WriteKVv2(path string, value interface{}) error {
	// Convert value to string - vals and other tools expect string values
	strValue := fmt.Sprintf("%v", value)

	secretData := map[string]interface{}{
		"data": map[string]interface{}{
			"value": strValue,
		},
	}

	fullPath := fmt.Sprintf("%s/data/%s", v.mountPath, path)
	_, err := v.client.Logical().Write(fullPath, secretData)
	if err != nil {
		return fmt.Errorf("failed to write to vault path %s: %w", path, err)
	}

	return nil
}
