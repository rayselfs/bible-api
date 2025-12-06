package utils

import "strings"

// HasPermission checks if the user has the required permission
// permissionsHeader: comma-separated list of permissions from X-Permissions header
// permission: the required permission to check
// Returns true if user has the permission or "*" (all permissions), false otherwise
func HasPermission(permissionsHeader string, permission string) bool {
	if permissionsHeader == "" {
		return false
	}

	// Check if user has the required permission or "*" (all permissions)
	permList := strings.SplitSeq(permissionsHeader, ",")
	for perm := range permList {
		trimmed := strings.TrimSpace(perm)
		if trimmed == permission || trimmed == "*" {
			return true
		}
	}

	return false
}
