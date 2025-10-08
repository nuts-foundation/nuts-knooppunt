package bsnutil

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	tokenPrefix        = "token-"
	pseudonymPrefix    = "ps-"
	minTokenLength     = len(tokenPrefix) + 1     // "token-" + at least 1 char
	minPseudonymLength = len(pseudonymPrefix) + 1 // "ps-" + at least 1 char

	// Input validation limits
	maxAudienceLength = 255
)

// CreateTransportToken creates a transport token from BSN and audience using simple XOR transformation.
// Each transport token is unique (includes random nonce) but contains the same encrypted BSN.
// This ensures transport tokens cannot be tracked while the NVI can always generate the same pseudonym.
//
// Parameters:
//   - bsn: The social security number or other identifier (as string to preserve format)
//   - audience: The identifier for the organization/audience receiving the token
//
// Returns a transport token in format: "token-{audience}-{transformedBSN}-{nonce}"
func CreateTransportToken(bsn string, audience string) (string, error) {
	// Validate inputs
	if err := validateAudience(audience); err != nil {
		return "", err
	}

	// For now, use a simple transformation - later this will use proper encryption
	// XOR the BSN string directly with audience-derived key
	key := generateSimpleKey(audience)
	transformedBSN := encodeXOR(bsn, key)

	// Add random nonce to make each token unique
	nonce, err := generateRandomNonce()
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("token-%s-%s-%s", audience, transformedBSN, nonce), nil
}

// BSNFromTransportToken extracts the original BSN from a transport token.
// This reverses the XOR transformation applied in CreateTransportToken.
//
// Parameters:
//   - token: The transport token in format "token-{audience}-{transformedBSN}-{nonce}"
//
// Returns the original BSN or an error if the token format is invalid.
func BSNFromTransportToken(token string) (string, error) {
	// Parse token components
	audience, transformedBSN, err := parseTokenComponents(token)
	if err != nil {
		return "", err
	}

	// Generate the same key used for encryption
	key := generateSimpleKey(audience)

	// Reverse the XOR transformation
	return decodeXOR(transformedBSN, key)
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

// encodeXOR takes a plaintext string and returns a hex-encoded XOR result
func encodeXOR(plaintext string, key int) string {
	if plaintext == "" {
		return ""
	}

	// Convert key to bytes
	keyBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(keyBytes, uint32(key))

	inputBytes := []byte(plaintext)
	result := make([]byte, len(inputBytes))

	// Create expanded key for subtle.XORBytes (which requires equal length slices)
	expandedKey := make([]byte, len(inputBytes))
	for i := range expandedKey {
		expandedKey[i] = keyBytes[i%4]
	}

	subtle.XORBytes(result, inputBytes, expandedKey)
	return fmt.Sprintf("%x", result)
}

// decodeXOR takes a hex-encoded string and returns the plaintext result
func decodeXOR(hexEncoded string, key int) (string, error) {
	if hexEncoded == "" {
		return "", nil
	}

	inputBytes, err := hex.DecodeString(hexEncoded)
	if err != nil {
		return "", fmt.Errorf("invalid hex encoding: %w", err)
	}

	// Convert key to bytes
	keyBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(keyBytes, uint32(key))

	result := make([]byte, len(inputBytes))

	// Create expanded key for subtle.XORBytes (which requires equal length slices)
	expandedKey := make([]byte, len(inputBytes))
	for i := range expandedKey {
		expandedKey[i] = keyBytes[i%4]
	}

	subtle.XORBytes(result, inputBytes, expandedKey)
	return string(result), nil
}

// TODO: Remove this later - this logic will be implemented in a HAPI Interceptor at the NVI - only here to prove the concept
// TransportTokenToPseudonym converts a transport token to a pseudonym format.
// This extracts the core BSN information and creates a consistent pseudonym (ignoring nonce).
// NOTE: The pseudonym uses the token's audience because the transformedBSN is encrypted with that audience's key.
func TransportTokenToPseudonym(token string) (string, error) {
	// Parse token components
	audience, transformedBSN, err := parseTokenComponents(token)
	if err != nil {
		return "", fmt.Errorf("invalid token format")
	}

	// Generate consistent pseudonym using the transformed BSN and the token's audience (deterministic)
	// We use the token's audience (not a parameter) because the transformedBSN is encrypted with that key
	return fmt.Sprintf("ps-%s-%s", audience, transformedBSN), nil
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
	transformedBSN := pseudonym[lastHyphen+1:]

	// Reverse the XOR to get original BSN
	key := generateSimpleKey(pseudonymHolder)
	originalBSN, err := decodeXOR(transformedBSN, key)
	if err != nil {
		return "", fmt.Errorf("invalid pseudonym format: %w", err)
	}

	// Create new token with the target audience
	return CreateTransportToken(originalBSN, audience)
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

// parseTokenComponents extracts audience and transformedBSN from a transport token.
func parseTokenComponents(token string) (audience string, transformedBSN string, err error) {
	// Parse the token format: "token-{audience}-{transformedBSN}-{nonce}"
	if len(token) < minTokenLength || !strings.HasPrefix(token, tokenPrefix) {
		return "", "", fmt.Errorf("invalid token format")
	}

	// Split by hyphens and parse components
	parts := strings.Split(token[len(tokenPrefix):], "-") // Skip "token-" prefix

	// We need at least 3 parts: audience, transformedBSN, and nonce
	if len(parts) < 3 {
		return "", "", fmt.Errorf("invalid token format")
	}

	// Get transformedBSN (second-to-last part)
	transformedBSNValue := parts[len(parts)-2]

	// Reconstruct audience (all parts except the last two: transformedBSN and nonce)
	audienceValue := strings.Join(parts[:len(parts)-2], "-")

	return audienceValue, transformedBSNValue, nil
}
