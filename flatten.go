package main

// Flatten converts a nested map structure into a flat map with dot-notation keys.
// For example: {"admin": {"oauth2": {"clientID": "x"}}} becomes {"admin.oauth2.clientID": "x"}
func Flatten(data map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	flattenRecursive(data, "", result)
	return result
}

func flattenRecursive(data map[string]interface{}, prefix string, result map[string]interface{}) {
	for key, value := range data {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}

		switch v := value.(type) {
		case map[string]interface{}:
			flattenRecursive(v, fullKey, result)
		default:
			result[fullKey] = value
		}
	}
}
