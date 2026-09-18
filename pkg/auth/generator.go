package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// HashKey calculates the SHA-256 hex digest of a raw API key.
func HashKey(plaintext string) string {
	digest := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(digest[:])
}

// GenerateRandomHex produces n cryptographically secure random bytes as a hex string.
func GenerateRandomHex(n int) (string, error) {
	bytes := make([]byte, n)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed generating cryptographic entropy: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// GenerateID produces a unique public key identifier, e.g. "key_3fa8c71b02de893a".
func GenerateID() (string, error) {
	randHex, err := GenerateRandomHex(8)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("key_%s", randHex), nil
}

// GenerateRawKey produces a cryptographically secure API key string.
// For AUTH: cee_live_<64-hex-chars> (256 bits entropy)
// For METRICS: cee_metrics_<64-hex-chars> (256 bits entropy)
func GenerateRawKey(cType CredentialType) (string, error) {
	entropy, err := GenerateRandomHex(32) // 256 bits of entropy
	if err != nil {
		return "", err
	}

	switch cType {
	case TypeMetrics:
		return fmt.Sprintf("cee_metrics_%s", entropy), nil
	default:
		return fmt.Sprintf("cee_live_%s", entropy), nil
	}
}

// ExtractPrefix produces a safe display prefix (e.g. "cee_live_ab12cd34...")
func ExtractPrefix(rawKey string) string {
	if len(rawKey) <= 18 {
		return rawKey
	}
	return fmt.Sprintf("%s...", rawKey[:18])
}

// GenerateKey creates a new Key record and its plaintext key string.
func GenerateKey(cType CredentialType, role Role, createdBy string, description string) (*Key, string, error) {
	if err := ValidateCombination(cType, role); err != nil {
		return nil, "", err
	}

	id, err := GenerateID()
	if err != nil {
		return nil, "", err
	}

	rawKey, err := GenerateRawKey(cType)
	if err != nil {
		return nil, "", err
	}

	keyHash := HashKey(rawKey)
	prefix := ExtractPrefix(rawKey)

	key := &Key{
		ID:          id,
		Prefix:      prefix,
		KeyHash:     keyHash,
		Type:        cType,
		Role:        role,
		Status:      StatusActive,
		CreatedAt:   time.Now().UTC(),
		CreatedBy:   createdBy,
		Description: description,
	}

	return key, rawKey, nil
}
