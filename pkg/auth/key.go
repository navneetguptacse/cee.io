package auth

import (
	"errors"
	"fmt"
	"time"
)

type CredentialType string

const (
	TypeAuth    CredentialType = "auth"
	TypeMetrics CredentialType = "metrics"
)

type Role string

const (
	RoleMaster Role = "master"
	RoleGuest  Role = "guest"
)

type Status string

const (
	StatusActive  Status = "active"
	StatusRevoked Status = "revoked"
)

// Standard permission constants
const (
	PermAPIAccess            = "api.access"
	PermTokenGenerateGuest   = "token.generate.guest"
	PermTokenGenerateMaster  = "token.generate.master"
	PermTokenGenerateMetrics = "token.generate.metrics"
	PermTokenRevoke          = "token.revoke"
	PermTokenList            = "token.list"
	PermMetricsAccess        = "metrics.access"
)

var (
	ErrInvalidTypeRole  = errors.New("invalid credential type and role combination: metrics credentials must be master-only")
	ErrPermissionDenied = errors.New("permission denied for requested credential operation")
	ErrLastMaster       = errors.New("cannot revoke the last active master AUTH API key")
	ErrKeyNotFound      = errors.New("api key not found")
	ErrKeyRevoked       = errors.New("api key has been revoked")
	ErrInvalidRequest   = errors.New("invalid request payload")
)

// Key represents a stored API credential record.
// KeyHash is stored as a SHA-256 digest and is never serialized in JSON responses.
type Key struct {
	ID          string         `json:"id"`
	Prefix      string         `json:"prefix"`
	KeyHash     string         `json:"-"`
	Type        CredentialType `json:"type"`
	Role        Role           `json:"role"`
	Status      Status         `json:"status"`
	CreatedAt   time.Time      `json:"created_at"`
	CreatedBy   string         `json:"created_by"`
	LastUsedAt  *time.Time     `json:"last_used_at,omitempty"`
	ExpiresAt   *time.Time     `json:"expires_at,omitempty"`
	Description string         `json:"description,omitempty"`
}

// GenerateKeyRequest represents payload to generate a new API key.
type GenerateKeyRequest struct {
	Type        CredentialType `json:"type"`
	Role        Role           `json:"role"`
	Description string         `json:"description,omitempty"`
}

// GenerateKeyResponse is returned once upon creation with the plaintext API key.
type GenerateKeyResponse struct {
	ID        string         `json:"id"`
	Type      CredentialType `json:"type"`
	Role      Role           `json:"role"`
	APIKey    string         `json:"api_key"`
	Prefix    string         `json:"prefix"`
	CreatedAt time.Time      `json:"created_at"`
	CreatedBy string         `json:"created_by"`
}

// CapabilityInfo represents safe metadata about the authenticated caller.
type CapabilityInfo struct {
	Authenticated  bool           `json:"authenticated"`
	KeyID          string         `json:"key_id"`
	Prefix         string         `json:"prefix"`
	CredentialType CredentialType `json:"credential_type"`
	Role           Role           `json:"role"`
	Permissions    []string       `json:"permissions"`
}

// ValidateCombination ensures the credential type and role are valid.
func ValidateCombination(cType CredentialType, role Role) error {
	if cType == TypeMetrics && role == RoleGuest {
		return ErrInvalidTypeRole
	}
	if cType != TypeAuth && cType != TypeMetrics {
		return fmt.Errorf("unknown credential type: %s", cType)
	}
	if role != RoleMaster && role != RoleGuest {
		return fmt.Errorf("unknown credential role: %s", role)
	}
	return nil
}

// ListPermissions returns the explicit list of allowed operations for a credential.
func ListPermissions(k *Key) []string {
	if k == nil || k.Status != StatusActive {
		return nil
	}

	switch {
	case k.Type == TypeAuth && k.Role == RoleMaster:
		return []string{
			PermAPIAccess,
			PermTokenGenerateGuest,
			PermTokenGenerateMaster,
			PermTokenGenerateMetrics,
			PermTokenRevoke,
			PermTokenList,
			PermMetricsAccess,
		}
	case k.Type == TypeAuth && k.Role == RoleGuest:
		return []string{
			PermAPIAccess,
			PermTokenGenerateGuest,
		}
	case k.Type == TypeMetrics && k.Role == RoleMaster:
		return []string{
			PermMetricsAccess,
		}
	default:
		return nil
	}
}

// HasPermission checks if the given key possesses the specific permission.
func HasPermission(k *Key, perm string) bool {
	perms := ListPermissions(k)
	for _, p := range perms {
		if p == perm {
			return true
		}
	}
	return false
}

// CanGenerate checks if caller can generate a target credential type and role.
func CanGenerate(caller *Key, targetType CredentialType, targetRole Role) bool {
	if caller == nil || caller.Status != StatusActive {
		return false
	}
	if err := ValidateCombination(targetType, targetRole); err != nil {
		return false
	}

	// Auth Master can generate everything (auth:guest, auth:master, metrics:master)
	if caller.Type == TypeAuth && caller.Role == RoleMaster {
		return true
	}

	// Auth Guest can ONLY generate another Auth Guest key
	if caller.Type == TypeAuth && caller.Role == RoleGuest {
		return targetType == TypeAuth && targetRole == RoleGuest
	}

	// Metrics Master cannot generate any keys
	return false
}
