package bsnutil

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

const (
	tokenPrefix        = "token-"
	pseudonymPrefix    = "ps-"
	minTokenLength     = len(tokenPrefix) + 1     // "token-" + at least 1 char
	minPseudonymLength = len(pseudonymPrefix) + 1 // "ps-" + at least 1 char

	// Input validation limits
	maxAudienceLength = 255
	minBSN            = 100000000
	maxBSN            = 999999999
)

// CreateTransportToken creates a transport token from BSN and audience using simple XOR transformation.
// Each transport token is unique (includes random nonce) but contains the same encrypted BSN.
// This ensures transport tokens cannot be tracked while the NVI can always generate the same pseudonym.
//
// Parameters:
//   - bsn: The Dutch social security number (BSN)
//   - audience: The identifier for the organization/audience receiving the token
//
// Returns a transport token in format: "token-{audience}-{transformedBSN}-{nonce}"
func CreateTransportToken(bsn int, audience string) (string, error) {
	// Validate inputs
	if err := validateBSN(bsn); err != nil {
		return "", err
	}
	if err := validateAudience(audience); err != nil {
		return "", err
	}

	// For now, use a simple transformation - later this will use proper encryption
	// Using SHA256 hash of the combination as a simple key derivation
	key := generateSimpleKey(audience)
	transformedBSN := simpleXOR(bsn, key)

	// Add random nonce to make each token unique
	nonce, err := generateRandomNonce()
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("token-%s-%d-%s", audience, transformedBSN, nonce), nil
}

// BSNFromTransportToken extracts the original BSN from a transport token.
// This reverses the XOR transformation applied in CreateTransportToken.
//
// Parameters:
//   - token: The transport token in format "token-{audience}-{transformedBSN}-{nonce}"
//
// Returns the original BSN or an error if the token format is invalid.
func BSNFromTransportToken(token string) (int, error) {
	// Parse token components
	audience, transformedBSN, _, err := parseTokenComponents(token)
	if err != nil {
		return 0, err
	}

	// Generate the same key used for encryption
	key := generateSimpleKey(audience)

	// Reverse the XOR transformation (XOR is self-inverse)
	originalBSN := simpleXOR(transformedBSN, key)

	return originalBSN, nil
}

// generateSimpleKey creates a simple numeric key from the audience string.
// In future versions, this will be replaced with proper key derivation functions.
func generateSimpleKey(audience string) int {
	// Use SHA256 hash and take first 4 bytes as integer
	hash := sha256.Sum256([]byte(audience))

	// Convert first 4 bytes to uint32, then ensure positive int range
	hashValue := binary.BigEndian.Uint32(hash[:4])
	key := int(hashValue >> 1) // Shift right 1 bit to ensure positive int (31 bits)

	return key
}

// generateRandomNonce creates a random nonce to make transport tokens unique.
func generateRandomNonce() (string, error) {
	// Generate 4 random bytes and convert to hex string
	bytes := make([]byte, 4)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", fmt.Errorf("failed to generate random nonce: %w", err)
	}
	return fmt.Sprintf("%x", bytes), nil
}

// simpleXOR applies XOR transformation to the BSN using the generated key.
// This is a placeholder for more sophisticated encryption in future versions.
func simpleXOR(bsn, key int) int {
	return bsn ^ key
}

// TODO: Remove this later - this logic will be implemented in a HAPI Interceptor at the NVI - only here to prove the concept
// TransportTokenToPseudonym converts a transport token to a pseudonym format.
// This extracts the core BSN information and creates a consistent pseudonym (ignoring timestamp/nonce).
func TransportTokenToPseudonym(token string) (string, error) {
	// Parse token components
	audience, transformedBSN, _, err := parseTokenComponents(token)
	if err != nil {
		return "", fmt.Errorf("invalid token format")
	}

	// Generate consistent pseudonym using the transformed BSN and audience (deterministic)
	return fmt.Sprintf("ps-%s-%d", audience, transformedBSN), nil
}

// TODO: Remove this later - this logic will be implemented in a HAPI Interceptor at the NVI - only here to prove the concept
// PseudonymToTransportToken converts a pseudonym back to transport token format.
// This reverses the TransportTokenToPseudonym transformation.
func PseudonymToTransportToken(pseudonym string, audience string) (string, error) {
	// Parse the pseudonym format manually to handle audience names with hyphens
	if len(pseudonym) < minPseudonymLength || !strings.HasPrefix(pseudonym, pseudonymPrefix) {
		return "", fmt.Errorf("invalid pseudonym format")
	}

	// Find the last hyphen to separate audience from transformedBSN
	lastHyphen := strings.LastIndex(pseudonym[len(pseudonymPrefix):], "-")
	if lastHyphen != -1 {
		lastHyphen += len(pseudonymPrefix) // Adjust for the "ps-" prefix offset
	}

	if lastHyphen == -1 || lastHyphen == len(pseudonymPrefix) {
		return "", fmt.Errorf("invalid pseudonym format")
	}

	pseudonymHolder := pseudonym[len(pseudonymPrefix):lastHyphen]
	transformedBSNStr := pseudonym[lastHyphen+1:]

	transformedBSN, err := strconv.Atoi(transformedBSNStr)
	if err != nil {
		return "", fmt.Errorf("invalid pseudonym format")
	}

	// Reverse the XOR to get original BSN
	key := generateSimpleKey(pseudonymHolder)
	originalBSN := simpleXOR(transformedBSN, key)

	// Create new token with the target audience
	return CreateTransportToken(originalBSN, audience)
}

// validateBSN checks if the BSN is within valid range for Dutch social security numbers.
func validateBSN(bsn int) error {
	if bsn < minBSN || bsn > maxBSN {
		return fmt.Errorf("invalid BSN: must be between %d and %d", minBSN, maxBSN)
	}
	return nil
}

// validateAudience checks if the audience string is valid and safe to use.
func validateAudience(audience string) error {
	if len(audience) == 0 {
		return fmt.Errorf("invalid audience: cannot be empty")
	}
	if len(audience) > maxAudienceLength {
		return fmt.Errorf("invalid audience: exceeds maximum length of %d characters", maxAudienceLength)
	}
	return nil
}

// parseTokenComponents extracts audience, transformedBSN, and nonce from a transport token.
func parseTokenComponents(token string) (audience string, transformedBSN int, nonce string, err error) {
	// Parse the token format: "token-{audience}-{transformedBSN}-{nonce}"
	if len(token) < minTokenLength || !strings.HasPrefix(token, tokenPrefix) {
		return "", 0, "", fmt.Errorf("invalid token format")
	}

	// Split by hyphens and parse components
	parts := strings.Split(token[len(tokenPrefix):], "-") // Skip "token-" prefix

	// We need at least 3 parts: audience, transformedBSN, and nonce
	if len(parts) < 3 {
		return "", 0, "", fmt.Errorf("invalid token format")
	}

	// Parse transformedBSN (second-to-last part)
	transformedBSNValue, parseErr := strconv.Atoi(parts[len(parts)-2])
	if parseErr != nil {
		return "", 0, "", fmt.Errorf("invalid token format")
	}

	// Reconstruct audience (all parts except the last two: transformedBSN and nonce)
	audienceValue := strings.Join(parts[:len(parts)-2], "-")

	// Extract nonce (last part)
	nonceValue := parts[len(parts)-1]

	return audienceValue, transformedBSNValue, nonceValue, nil
}
