package access

import "testing"

func TestRolePermissions(t *testing.T) {
	tests := []struct {
		role       string
		permission Permission
		want       bool
	}{
		{RoleAdministrator, ManageUsers, true},
		{RoleAdministrator, ManageSystem, true},
		{RoleOperator, ManageNodes, true},
		{RoleOperator, RunBackups, true},
		{RoleOperator, ManageUsers, false},
		{RoleAuditor, ViewAudit, true},
		{RoleAuditor, ManageSystem, false},
		{RoleViewer, ViewAudit, false},
		{"Unknown", ManageNodes, false},
	}

	for _, tt := range tests {
		if got := Allows(tt.role, tt.permission); got != tt.want {
			t.Errorf("Allows(%q, %q) = %v, want %v", tt.role, tt.permission, got, tt.want)
		}
	}
}

func TestValidRole(t *testing.T) {
	for _, role := range []string{RoleAdministrator, RoleOperator, RoleAuditor, RoleViewer} {
		if !ValidRole(role) {
			t.Errorf("expected %q to be valid", role)
		}
	}
	if ValidRole("Owner") {
		t.Error("unexpected custom role accepted")
	}
}
