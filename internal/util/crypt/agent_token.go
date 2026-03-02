package crypt

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
)

const (
	agentTokenPrefix = "agt_"
)

// GenerateAgentToken creates a deterministic authentication token for an agent
// using HMAC-SHA256. The token format is: agt_<spaceId>_<signature>
//
// Parameters:
//   - spaceId: The ID of the space the agent is running in
//   - userId: The ID of the user who owns the space
//   - zone: The zone/region name where the server is located
//   - encryptionKey: The server's encryption key used as the HMAC secret
//
// Returns a token in the format: "agt_<spaceId>_<signature>"
func GenerateAgentToken(spaceId, userId, zone, encryptionKey string) (string, error) {
	if spaceId == "" || userId == "" || zone == "" || encryptionKey == "" {
		return "", fmt.Errorf("all parameters are required for token generation")
	}

	// Generate HMAC signature from spaceId + userId + zone
	h := hmac.New(sha256.New, []byte(encryptionKey))
	h.Write([]byte(fmt.Sprintf("%s|%s|%s", spaceId, userId, zone)))
	signature := h.Sum(nil)

	// Encode signature
	sigEncoded := base64.RawURLEncoding.EncodeToString(signature)

	// Format: agt_<spaceId>_<signature>
	token := fmt.Sprintf("%s%s_%s", agentTokenPrefix, spaceId, sigEncoded)

	return token, nil
}

// ValidateAgentToken validates an agent token and extracts the space ID.
// The caller must look up the space to get userId and zone for verification.
//
// Parameters:
//   - token: The agent token to validate
//   - spaceId: The space ID from the token (for DB lookup)
//   - userId: The user ID from the space record
//   - zone: The zone from server config
//   - encryptionKey: The server's encryption key used to verify the signature
//
// Returns: true if the signature is valid
func ValidateAgentToken(token, spaceId, userId, zone, encryptionKey string) bool {
	// Check prefix
	if !strings.HasPrefix(token, agentTokenPrefix) {
		return false
	}

	// Remove prefix
	token = strings.TrimPrefix(token, agentTokenPrefix)

	// Split into spaceId and signature
	parts := strings.SplitN(token, "_", 2)
	if len(parts) != 2 {
		return false
	}

	tokenSpaceId := parts[0]
	providedSig := parts[1]

	// Verify spaceId matches
	if tokenSpaceId != spaceId {
		return false
	}

	// Re-generate signature with provided parameters
	h := hmac.New(sha256.New, []byte(encryptionKey))
	h.Write([]byte(fmt.Sprintf("%s|%s|%s", spaceId, userId, zone)))
	expectedSig := base64.RawURLEncoding.EncodeToString(h.Sum(nil))

	// Compare signatures
	return hmac.Equal([]byte(expectedSig), []byte(providedSig))
}

// ExtractSpaceIdFromToken extracts the space ID from an agent token without validation.
// Returns empty string if token format is invalid.
func ExtractSpaceIdFromToken(token string) string {
	// Check prefix
	if !strings.HasPrefix(token, agentTokenPrefix) {
		return ""
	}

	// Remove prefix
	token = strings.TrimPrefix(token, agentTokenPrefix)

	// Split and return spaceId
	parts := strings.SplitN(token, "_", 2)
	if len(parts) != 2 {
		return ""
	}

	return parts[0]
}

// IsAgentToken checks if a token is an agent token (vs a regular API token)
func IsAgentToken(token string) bool {
	return strings.HasPrefix(token, agentTokenPrefix)
}
