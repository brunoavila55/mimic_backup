package ssh

import (
	"bytes"
	"fmt"
	"net"
	"mimic/internal/models"
	"mimic/internal/services/ssh/vendors"
	"mimic/pkg/crypto"
	"time"

	"golang.org/x/crypto/ssh"
	"gorm.io/gorm"
)

type SSHService struct {
	DB *gorm.DB
}

func (s *SSHService) Connect(node *models.Node) (*ssh.Client, error) {
	var authMethods []ssh.AuthMethod

	username := node.Username
	if node.SSHPrivateKey != "" {
		signer, err := ssh.ParsePrivateKey([]byte(node.SSHPrivateKey))
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %v", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	} else {
		encPass := node.Password
		if node.AccessAgentID != nil && node.AccessAgent != nil {
			encPass = node.AccessAgent.Password
			username = node.AccessAgent.Username
		}
		
		password, err := crypto.Decrypt(encPass)
		if err != nil {
			// If decryption fails, try raw password (fallback for old data)
			password = encPass
		}
		authMethods = append(authMethods, ssh.Password(password))
	}

	var hostKeyCallback ssh.HostKeyCallback

	if !node.VerifyHostKey {
		hostKeyCallback = ssh.InsecureIgnoreHostKey()
	} else {
		hostKeyCallback = func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			fingerprint := ssh.FingerprintSHA256(key)
			if node.SSHPublicFingerprint == "" {
				// TOFU: Trust on First Use
				if s.DB != nil {
					node.SSHPublicFingerprint = fingerprint
					s.DB.Save(node)
				}
				return nil
			}

			if node.SSHPublicFingerprint != fingerprint {
				return fmt.Errorf("host key mismatch: expected %s, got %s", node.SSHPublicFingerprint, fingerprint)
			}
			return nil
		}
	}

	config := &ssh.ClientConfig{
		User:            username,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         30 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", node.IP, node.Port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("failed to dial: %v", err)
	}

	return client, nil
}

func (s *SSHService) RunCommand(client *ssh.Client, command string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	var b bytes.Buffer
	session.Stdout = &b
	if err := session.Run(command); err != nil {
		return "", fmt.Errorf("failed to run command: %v", err)
	}

	return b.String(), nil
}

func (s *SSHService) PerformBackup(node *models.Node) (string, error) {
	client, err := s.Connect(node)
	if err != nil {
		return "", err
	}
	defer client.Close()

	driver, err := vendors.GetDriver(node.Vendor)
	if err != nil {
		return "", err
	}

	raw, err := s.RunCommand(client, driver.GetBackupCommand())
	if err != nil {
		return "", err
	}

	return driver.NormalizeConfig(raw), nil
}
