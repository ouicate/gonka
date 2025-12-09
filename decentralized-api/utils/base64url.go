package utils

import "strings"

// Base64ToBase64URL converts standard base64 to URL-safe base64 (RFC 4648).
// Replaces '+' with '-' and '/' with '_'.
// This is used when putting base64 IDs in URL paths to avoid routing issues.
func Base64ToBase64URL(s string) string {
	return strings.NewReplacer("+", "-", "/", "_").Replace(s)
}

// Base64URLToBase64 converts URL-safe base64 back to standard base64.
// Replaces '-' with '+' and '_' with '/'.
// This is used when extracting base64 IDs from URL paths.
func Base64URLToBase64(s string) string {
	return strings.NewReplacer("-", "+", "_", "/").Replace(s)
}
