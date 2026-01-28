# sops-to-vault

CLI tool to import secrets from a SOPS-encrypted YAML file to HashiCorp Vault KV v2.

## Installation

```bash
go install github.com/ethanadams/sops-to-vault@latest
```

Or build from source:

```bash
git clone https://github.com/ethanadams/sops-to-vault.git
cd sops-to-vault
go build -o sops-to-vault .
```

## Usage

```bash
sops-to-vault [flags] <sops-file> <vault-path>
```

### Arguments

- `sops-file` - Path to SOPS-encrypted YAML file
- `vault-path` - Destination path in Vault (under the mount)

### Flags

| Flag | Env Var | Description |
|------|---------|-------------|
| `--vault-addr` | `VAULT_ADDR` | Vault server address |
| `--vault-token` | `VAULT_TOKEN` | Vault authentication token |
| `--mount` | - | KV v2 mount path (default: `secret`) |
| `--dry-run` | - | Preview without writing to Vault |
| `--append-name` | - | Append cleaned filename to vault path |
| `--name` | - | Override the derived name (use with `--append-name`) |
| `--update-counterpart` | - | Update counterpart YAML file with vault references |

### Examples

```bash
# Dry run to preview
./sops-to-vault --dry-run app-secrets.enc.yaml myproject

# Write to Vault using environment variables
export VAULT_ADDR=https://vault.example.com
export VAULT_TOKEN=s.xxxxxxx
./sops-to-vault app-secrets.enc.yaml myproject

# Append cleaned filename to path (app-secrets.enc.yaml -> app)
./sops-to-vault --append-name app-secrets.enc.yaml myproject
# Writes to: secret/myproject/app/*

# Update counterpart file with vault references
./sops-to-vault --append-name --update-counterpart app-secrets.enc.yaml myproject
# Also updates app.yaml with ref+vault:// references
```

## How It Works

1. Decrypts the SOPS file using GCP KMS (via Application Default Credentials)
2. Flattens nested YAML keys into dot-notation (e.g., `admin.oauth2.clientID`)
3. Writes each secret to its own Vault KV v2 path

### Vault Storage

Each flattened key is stored at its own path with the value under a `value` key:

```
secret/myproject/app/image.dockerauth       -> {"value": "secret1"}
secret/myproject/app/admin.oauth2.clientID  -> {"value": "secret2"}
```

### Counterpart File Updates

With `--update-counterpart`, the tool updates the corresponding YAML file (e.g., `app-secrets.enc.yaml` -> `app.yaml`) with vault references:

```yaml
# Before (app.yaml)
image:
  dockerauth: placeholder
admin:
  oauth2:
    clientID: placeholder

# After
image:
  dockerauth: ref+vault://secret/myproject/app/image.dockerauth#value
admin:
  oauth2:
    clientID: ref+vault://secret/myproject/app/admin.oauth2.clientID#value
```

The tool preserves the original YAML structure:
- Existing nested keys are updated in place
- New keys are added as nested if no flat keys (keys with dots) exist at that level
- New keys are added as flat if flat keys already exist at that level
- Original indentation (2-space, 4-space, etc.) is preserved

### Filename Cleaning

The `--append-name` flag derives a clean name from the SOPS filename:

| Input | Output |
|-------|--------|
| `app-secrets.enc.yaml` | `app` |
| `myapp.sops.yaml` | `myapp` |
| `config-secrets.yaml` | `config` |

Use `--name` to override: `--append-name --name=custom`

## Requirements

- Go 1.21+
- GCP Application Default Credentials configured (for SOPS decryption)
- Vault token with write access to the target path

## License

MIT
