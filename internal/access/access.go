package access

const (
	RoleAdministrator = "Administrator"
	RoleOperator      = "Operator"
	RoleAuditor       = "Auditor"
	RoleViewer        = "Viewer"
)

type Permission string

const (
	ManageUsers      Permission = "manage_users"
	ManageNodes      Permission = "manage_nodes"
	RunBackups       Permission = "run_backups"
	ManageOperations Permission = "manage_operations"
	ManageSystem     Permission = "manage_system"
	ExportBackups    Permission = "export_backups"
	ViewAudit        Permission = "view_audit"
)

func ValidRole(role string) bool {
	switch role {
	case RoleAdministrator, RoleOperator, RoleAuditor, RoleViewer:
		return true
	default:
		return false
	}
}

func Allows(role string, permission Permission) bool {
	if role == RoleAdministrator {
		return true
	}

	switch role {
	case RoleOperator:
		return permission == ManageNodes ||
			permission == RunBackups ||
			permission == ManageOperations ||
			permission == ExportBackups
	case RoleAuditor:
		return permission == ViewAudit
	default:
		return false
	}
}

// PermissionsForRole returns the public capability list used by browser
// clients. Authorization still happens on the server for every request.
func PermissionsForRole(role string) []Permission {
	permissions := []Permission{
		ManageUsers,
		ManageNodes,
		RunBackups,
		ManageOperations,
		ManageSystem,
		ExportBackups,
		ViewAudit,
	}
	allowed := make([]Permission, 0, len(permissions))
	for _, permission := range permissions {
		if Allows(role, permission) {
			allowed = append(allowed, permission)
		}
	}
	return allowed
}
