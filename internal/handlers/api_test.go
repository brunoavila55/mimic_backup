package handlers

import (
	"encoding/json"
	"strings"
	"testing"

	"mimic/internal/models"
)

func TestAPINodeDTOExcludesSecrets(t *testing.T) {
	node := models.Node{
		Name: "edge-01", Username: "operator", Password: "NODE_SECRET",
		SSHPrivateKey: "PRIVATE_KEY_SECRET", SSHPublicFingerprint: "fingerprint",
		Credential:  &models.Credential{Name: "core", Password: "CREDENTIAL_SECRET"},
		AccessAgent: &models.AccessAgent{Name: "jump", Password: "AGENT_SECRET"},
	}
	payload, err := json.Marshal(nodeDTO(node))
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(payload)
	for _, forbidden := range []string{"NODE_SECRET", "PRIVATE_KEY_SECRET", "CREDENTIAL_SECRET", "AGENT_SECRET", "password", "ssh_private_key"} {
		if strings.Contains(strings.ToLower(serialized), strings.ToLower(forbidden)) {
			t.Fatalf("sensitive value or field %q leaked in %s", forbidden, serialized)
		}
	}
}

func TestAPIBackupListDTOExcludesConfig(t *testing.T) {
	payload, err := json.Marshal(backupDTO(models.NodeBackup{Config: "SECRET_CONFIGURATION"}, false))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "SECRET_CONFIGURATION") || strings.Contains(string(payload), `"config"`) {
		t.Fatalf("backup config leaked in list DTO: %s", payload)
	}
}
