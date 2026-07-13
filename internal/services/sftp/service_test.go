package sftp

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"mimic/internal/models"

	"golang.org/x/crypto/ssh"
)

func publicKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	key, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	public, err := ssh.NewPublicKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return public
}

func TestVerifyHostKeyRejectsMismatch(t *testing.T) {
	trusted := publicKey(t)
	settings := &models.SftpSettings{HostFingerprint: ssh.FingerprintSHA256(trusted)}
	service := &SftpService{}
	callback := service.verifyHostKey(settings)

	if err := callback("host", nil, trusted); err != nil {
		t.Fatalf("trusted host key was rejected: %v", err)
	}
	if err := callback("host", nil, publicKey(t)); err == nil {
		t.Fatal("mismatched host key should be rejected")
	}
}

func TestVerifyHostKeyRequiresPersistentTOFU(t *testing.T) {
	service := &SftpService{}
	callback := service.verifyHostKey(&models.SftpSettings{})
	if err := callback("host", nil, publicKey(t)); err == nil {
		t.Fatal("first-use trust without persistence should fail closed")
	}
}
