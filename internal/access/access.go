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
	ManagePolicies   Permission = "manage_policies"
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
