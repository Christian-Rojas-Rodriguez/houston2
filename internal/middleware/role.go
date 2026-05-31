package middleware

import "fmt"

// Role represents the RBAC role hierarchy. Higher integer = more permissions.
type Role int

const (
	RoleMember  Role = iota // "group:member"  — lowest privilege
	RoleManager             // "group:manager"
	RoleOwner               // "org:owner"     — highest privilege
)

// ParseRole converts a Supabase role string to a typed Role.
// Returns an error for unrecognised strings to guard against DB drift.
func ParseRole(s string) (Role, error) {
	switch s {
	case "group:member":
		return RoleMember, nil
	case "group:manager":
		return RoleManager, nil
	case "org:owner":
		return RoleOwner, nil
	default:
		return 0, fmt.Errorf("middleware: unknown role %q", s)
	}
}
