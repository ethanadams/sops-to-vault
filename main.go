package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/getsops/sops/v3/decrypt"
	"gopkg.in/yaml.v3"
)

func main() {
	var (
		vaultAddr         = flag.String("vault-addr", "", "Vault server address (env: VAULT_ADDR)")
		vaultToken        = flag.String("vault-token", "", "Vault token (env: VAULT_TOKEN)")
		mountPath         = flag.String("mount", "secret", "Vault KV v2 mount path")
		dryRun            = flag.Bool("dry-run", false, "Print secrets without writing to Vault")
		appendName        = flag.Bool("append-name", false, "Append cleaned filename to vault path")
		nameOverride      = flag.String("name", "", "Override the derived name (use with --append-name)")
		updateCounterpart = flag.Bool("update-counterpart", false, "Update counterpart YAML file with vault_path")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags] <sops-file> <vault-path>\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Import secrets from a SOPS-encrypted YAML file to Vault KV v2.\n\n")
		fmt.Fprintf(os.Stderr, "Arguments:\n")
		fmt.Fprintf(os.Stderr, "  sops-file    Path to SOPS-encrypted YAML file\n")
		fmt.Fprintf(os.Stderr, "  vault-path   Destination path in Vault (under the mount)\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if flag.NArg() != 2 {
		flag.Usage()
		os.Exit(1)
	}

	sopsFile := flag.Arg(0)
	vaultPath := flag.Arg(1)

	// Append cleaned filename to vault path if requested
	if *appendName {
		name := *nameOverride
		if name == "" {
			name = cleanFilename(sopsFile)
		}
		vaultPath = vaultPath + "/" + name
	}

	// Resolve config with precedence: flags > env vars
	addr := resolveConfig(*vaultAddr, "VAULT_ADDR")
	token := resolveConfig(*vaultToken, "VAULT_TOKEN")

	// Validate required config (unless dry-run)
	if !*dryRun {
		if addr == "" {
			fmt.Fprintln(os.Stderr, "Error: Vault address required (--vault-addr or VAULT_ADDR)")
			os.Exit(1)
		}
		if token == "" {
			fmt.Fprintln(os.Stderr, "Error: Vault token required (--vault-token or VAULT_TOKEN)")
			os.Exit(1)
		}
	}

	// Decrypt SOPS file
	decrypted, err := decrypt.File(sopsFile, "yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error decrypting SOPS file: %v\n", err)
		os.Exit(1)
	}

	// Parse YAML
	var data map[string]interface{}
	if err := yaml.Unmarshal(decrypted, &data); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing YAML: %v\n", err)
		os.Exit(1)
	}

	// Flatten nested structure
	flattened := Flatten(data)

	// Extract sorted keys for counterpart updates
	keys := make([]string, 0, len(flattened))
	for k := range flattened {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	if *dryRun {
		printDryRun(vaultPath, *mountPath, flattened)
		if *updateCounterpart {
			counterpart := counterpartFilename(sopsFile)
			fullVaultPath := *mountPath + "/" + vaultPath
			if _, err := os.Stat(counterpart); err == nil {
				fmt.Printf("[dry-run] Would update %s with vault references:\n", counterpart)
				for _, k := range keys {
					fmt.Printf("  %s: ref+vault://%s/%s#value\n", k, fullVaultPath, k)
				}
			} else {
				fmt.Printf("[dry-run] Counterpart file %s does not exist, skipping\n", counterpart)
			}
		}
		return
	}

	// Write to Vault - each key gets its own path
	client, err := NewVaultClient(addr, token, *mountPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating Vault client: %v\n", err)
		os.Exit(1)
	}

	for _, key := range keys {
		secretPath := vaultPath + "/" + key
		if err := client.WriteKVv2(secretPath, flattened[key]); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing to Vault path %s: %v\n", secretPath, err)
			os.Exit(1)
		}
	}

	fmt.Printf("Successfully wrote %d secrets to %s/%s/*\n", len(flattened), *mountPath, vaultPath)

	// Update counterpart file if requested
	if *updateCounterpart {
		counterpart := counterpartFilename(sopsFile)
		absCounterpart, _ := filepath.Abs(counterpart)
		fullVaultPath := *mountPath + "/" + vaultPath
		updated, err := updateCounterpartFile(counterpart, fullVaultPath, keys)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to update counterpart file: %v\n", err)
		} else if updated {
			fmt.Printf("Updated %s with %d vault references\n", absCounterpart, len(keys))
		} else {
			fmt.Printf("Counterpart file %s does not exist, skipping\n", absCounterpart)
		}
	}
}

func resolveConfig(flagVal, envVar string) string {
	if flagVal != "" {
		return flagVal
	}
	return os.Getenv(envVar)
}

func printDryRun(path, mount string, data map[string]interface{}) {
	fmt.Printf("[dry-run] Would write to Vault path: %s/%s\n", mount, path)
	fmt.Printf("[dry-run] %d secrets:\n", len(data))

	// Sort keys for consistent output
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		v := data[k]
		// Mask values, show only type/length for security
		switch val := v.(type) {
		case string:
			fmt.Printf("  %s = <string, %d chars>\n", k, len(val))
		default:
			fmt.Printf("  %s = <%T>\n", k, v)
		}
	}
}

// cleanFilename extracts a clean name from a SOPS filename.
// Examples:
//   - "app-secrets.enc.yaml" -> "app"
//   - "myapp.sops.yaml" -> "myapp"
//   - "/path/to/config-secrets.yaml" -> "config"
func cleanFilename(path string) string {
	// Get base filename without directory
	name := filepath.Base(path)

	// Strip "-secrets" and everything after
	if idx := strings.Index(name, "-secrets"); idx != -1 {
		return name[:idx]
	}

	// Otherwise strip from first "."
	if idx := strings.Index(name, "."); idx != -1 {
		return name[:idx]
	}

	return name
}

// counterpartFilename derives the counterpart filename from a SOPS file path.
// Examples:
//   - "app-secrets.enc.yaml" -> "app.yaml"
//   - "/path/to/config-secrets.yaml" -> "/path/to/config.yaml"
func counterpartFilename(sopsPath string) string {
	dir := filepath.Dir(sopsPath)
	name := cleanFilename(sopsPath)
	return filepath.Join(dir, name+".yaml")
}

// updateCounterpartFile updates the counterpart YAML file with vault references.
// For each key in sopsKeys, it sets the value to ref+vault://<vaultPath>#<key>.
// If the key exists nested in counterpart, it updates nested. Otherwise adds as flat key.
// Only updates if the file exists. Preserves original formatting and indentation.
// Returns (updated bool, error).
func updateCounterpartFile(path, vaultPath string, sopsKeys []string) (bool, error) {
	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false, nil // File doesn't exist, skip silently
	}

	// Read existing file
	content, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("reading file: %w", err)
	}

	// Detect original indentation (default to 2)
	indent := detectIndent(content)

	// Parse YAML into Node to preserve ordering
	var doc yaml.Node
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return false, fmt.Errorf("parsing YAML: %w", err)
	}

	// Find the root mapping node
	var root *yaml.Node
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		root = doc.Content[0]
	} else if doc.Kind == yaml.MappingNode {
		root = &doc
	}

	if root == nil || root.Kind != yaml.MappingNode {
		return false, fmt.Errorf("expected YAML mapping at root, got kind %v", doc.Kind)
	}

	// Update or add each SOPS key
	for _, key := range sopsKeys {
		vaultRef := fmt.Sprintf("ref+vault://%s/%s#value", vaultPath, key)
		keyPath := strings.Split(key, ".")

		// Try to find and update the key, or add at deepest matching path
		upsertNestedKey(root, keyPath, vaultRef)
	}

	// Write back with original indentation
	var buf strings.Builder
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(indent)
	if err := encoder.Encode(&doc); err != nil {
		return false, fmt.Errorf("marshaling YAML: %w", err)
	}
	encoder.Close()

	if err := os.WriteFile(path, []byte(buf.String()), 0644); err != nil {
		return false, fmt.Errorf("writing file: %w", err)
	}

	return true, nil
}

// detectIndent detects the indentation used in YAML content.
func detectIndent(content []byte) int {
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " ")
		if len(trimmed) > 0 && len(trimmed) < len(line) {
			indent := len(line) - len(trimmed)
			if indent > 0 {
				return indent
			}
		}
	}
	return 2 // default
}

// upsertNestedKey finds the deepest matching nested path and either updates
// an existing key or adds a new one at the appropriate level.
// If the current level has flat keys (keys with dots), adds as flat key.
// Otherwise, creates nested structure.
func upsertNestedKey(node *yaml.Node, keyPath []string, value string) {
	if node.Kind != yaml.MappingNode || len(keyPath) == 0 {
		return
	}

	// First, try to find an exact match for the full flattened key at this level
	flatKey := strings.Join(keyPath, ".")
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == flatKey {
			// Found exact flat key match, update it
			node.Content[i+1].Value = value
			node.Content[i+1].Kind = yaml.ScalarNode
			node.Content[i+1].Tag = ""
			node.Content[i+1].Content = nil
			return
		}
	}

	// Try to find the first path segment as a nested mapping
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == keyPath[0] {
			if len(keyPath) == 1 {
				// Found the leaf key, update its value
				node.Content[i+1].Value = value
				node.Content[i+1].Kind = yaml.ScalarNode
				node.Content[i+1].Tag = ""
				node.Content[i+1].Content = nil
				return
			}
			// More path segments - if this is a mapping, recurse
			if node.Content[i+1].Kind == yaml.MappingNode {
				upsertNestedKey(node.Content[i+1], keyPath[1:], value)
				return
			}
			// Not a mapping, can't go deeper - shouldn't happen for well-formed data
			return
		}
	}

	// Key not found at this level
	// Check if this level has any flat keys (keys containing dots)
	if hasFlatKeys(node) {
		// Add as flat key
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: flatKey},
			&yaml.Node{Kind: yaml.ScalarNode, Value: value},
		)
	} else {
		// Create nested structure
		addNestedKey(node, keyPath, value)
	}
}

// hasFlatKeys checks if a mapping node has any keys containing dots
func hasFlatKeys(node *yaml.Node) bool {
	for i := 0; i < len(node.Content); i += 2 {
		if strings.Contains(node.Content[i].Value, ".") {
			return true
		}
	}
	return false
}

// addNestedKey creates nested structure for the key path
func addNestedKey(node *yaml.Node, keyPath []string, value string) {
	if len(keyPath) == 0 {
		return
	}

	if len(keyPath) == 1 {
		// Leaf node - add scalar value
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: keyPath[0]},
			&yaml.Node{Kind: yaml.ScalarNode, Value: value},
		)
		return
	}

	// Create nested mapping
	newMapping := &yaml.Node{Kind: yaml.MappingNode}
	node.Content = append(node.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: keyPath[0]},
		newMapping,
	)
	addNestedKey(newMapping, keyPath[1:], value)
}
