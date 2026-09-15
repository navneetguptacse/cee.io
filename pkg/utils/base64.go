package utils

import "encoding/base64"

// EncodeBase64 encodes a string to standard base64.
func EncodeBase64(s string) string {
	if s == "" {
		return ""
	}
	return base64.StdEncoding.EncodeToString([]byte(s))
}

// DecodeBase64 decodes a standard base64 string.
func DecodeBase64(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// DecodeIfNeeded decodes the string if base64Encoded is true, otherwise returns it as-is.
func DecodeIfNeeded(s string, base64Encoded bool) string {
	if !base64Encoded || s == "" {
		return s
	}
	decoded, err := DecodeBase64(s)
	if err != nil {
		return s // fallback to raw string if decoding fails
	}
	return decoded
}

// EncodeIfNeeded encodes the string if base64Encode is true, otherwise returns it as-is.
func EncodeIfNeeded(s *string, base64Encode bool) *string {
	if s == nil {
		return nil
	}
	if !base64Encode || *s == "" {
		return s
	}
	encoded := EncodeBase64(*s)
	return &encoded
}
