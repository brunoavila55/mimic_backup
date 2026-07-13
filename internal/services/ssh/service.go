package ssh

import (
	"bytes"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"mimic/internal/models"
	"mimic/internal/services/ssh/vendors"
	"mimic/pkg/crypto"

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
		allowLegacyPlaintext := false
		// Priority: Credential (modern) > AccessAgent (legacy) > direct node credentials
		if node.CredentialID != nil && node.Credential != nil {
			encPass = node.Credential.Password
			if node.Credential.Username != "" {
				username = node.Credential.Username
			}
		} else if node.AccessAgentID != nil && node.AccessAgent != nil {
			encPass = node.AccessAgent.Password
			allowLegacyPlaintext = true
			if node.AccessAgent.Username != "" {
				username = node.AccessAgent.Username
			}
		}

		password, err := crypto.Decrypt(encPass)
		if err != nil {
			if !allowLegacyPlaintext {
				return nil, fmt.Errorf("failed to decrypt SSH password: %w", err)
			}
			// AccessAgent is a legacy model whose existing rows may still be plaintext.
			password = encPass
		}
		authMethods = append(authMethods, ssh.Password(password))
	}

	hostKeyCallback := func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		fingerprint := ssh.FingerprintSHA256(key)
		if node.SSHPublicFingerprint == "" {
			// TOFU: Trust on First Use
			if s.DB == nil || node.ID == 0 {
				return fmt.Errorf("SSH host fingerprint is not configured and cannot be persisted")
			}
			if s.DB != nil {
				result := s.DB.Model(&models.Node{}).
					Where("id = ? AND (ssh_public_fingerprint IS NULL OR ssh_public_fingerprint = '')", node.ID).
					Update("ssh_public_fingerprint", fingerprint)
				if result.Error != nil {
					return fmt.Errorf("failed to persist SSH host fingerprint: %w", result.Error)
				}
				if result.RowsAffected == 0 {
					var persisted models.Node
					if err := s.DB.Select("ssh_public_fingerprint").First(&persisted, node.ID).Error; err != nil {
						return fmt.Errorf("failed to reload SSH host fingerprint: %w", err)
					}
					if persisted.SSHPublicFingerprint != fingerprint {
						return fmt.Errorf("host key mismatch: expected %s, got %s", persisted.SSHPublicFingerprint, fingerprint)
					}
				}
				node.SSHPublicFingerprint = fingerprint
			}
			return nil
		}

		if node.SSHPublicFingerprint != fingerprint {
			return fmt.Errorf("host key mismatch: expected %s, got %s", node.SSHPublicFingerprint, fingerprint)
		}
		return nil
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
		return nil, translateSSHError(fmt.Errorf("failed to dial: %v", err))
	}

	return client, nil
}

func (s *SSHService) RunInteractiveCommand(client *ssh.Client, prepCommands []string, mainCommand string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	// Request PTY to emulate a real terminal
	modes := ssh.TerminalModes{
		ssh.ECHO:          0,     // Disable echoing
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}
	if err := session.RequestPty("vt100", 80, 24, modes); err != nil {
		return "", fmt.Errorf("request for pseudo terminal failed: %s", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		return "", err
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		return "", err
	}

	if err := session.Shell(); err != nil {
		return "", fmt.Errorf("failed to start shell: %s", err)
	}

	// Send preparation commands
	for _, cmd := range prepCommands {
		fmt.Fprintf(stdin, "%s\n", cmd)
		time.Sleep(500 * time.Millisecond) // short wait to process prep command
	}

	// Send main command
	fmt.Fprintf(stdin, "%s\n", mainCommand)

	// Read output interactively until it idles for 2 seconds
	var output bytes.Buffer
	var outputMu sync.Mutex
	buf := make([]byte, 4096)
	done := make(chan struct{})

	go func() {
		for {
			n, readErr := stdout.Read(buf)
			if n > 0 {
				outputMu.Lock()
				output.Write(buf[:n])
				outputMu.Unlock()
			}
			if readErr != nil {
				break
			}
		}
		close(done)
	}()

	idleTimeout := 2 * time.Second
	timer := time.NewTimer(idleTimeout)
	lastLen := 0

	for {
		select {
		case <-done:
			outputMu.Lock()
			result := output.String()
			outputMu.Unlock()
			return result, nil
		case <-timer.C:
			outputMu.Lock()
			currentLen := output.Len()
			result := output.String()
			outputMu.Unlock()
			// If we have some output and it hasn't grown in the idle period, we are done
			if currentLen > 0 && currentLen == lastLen {
				return result, nil
			}
			lastLen = currentLen
			timer.Reset(idleTimeout)
		}
	}
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

	var raw string
	if driver.RequiresPTY() {
		raw, err = s.RunInteractiveCommand(client, driver.GetPrepCommands(), driver.GetBackupCommand())
	} else {
		raw, err = s.RunExecCommand(client, driver.GetPrepCommands(), driver.GetBackupCommand())
	}
	if err != nil {
		return "", translateSSHError(fmt.Errorf("failed to run command: %v", err))
	}

	return driver.NormalizeConfig(raw), nil
}

func (s *SSHService) RunExecCommand(client *ssh.Client, prepCommands []string, mainCommand string) (string, error) {
	cmdStr := mainCommand
	if len(prepCommands) > 0 {
		cmdStr = strings.Join(prepCommands, "; ") + "; " + mainCommand
	}

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	out, err := session.CombinedOutput(cmdStr)
	return string(out), err
}

func translateSSHError(err error) error {
	if err == nil {
		return nil
	}
	errStr := err.Error()

	if strings.Contains(errStr, "unable to authenticate") || strings.Contains(errStr, "handshake failed") {
		return fmt.Errorf("Authentication failed: invalid username or password for this device.")
	}
	if strings.Contains(errStr, "connection refused") {
		return fmt.Errorf("Connection refused: the SSH port may be closed on the device.")
	}
	if strings.Contains(errStr, "i/o timeout") || strings.Contains(errStr, "timed out") {
		return fmt.Errorf("Timeout exceeded: the device did not respond (may be offline).")
	}
	if strings.Contains(errStr, "no route to host") {
		return fmt.Errorf("Route not found: the device IP is incorrect or unreachable.")
	}
	if strings.Contains(errStr, "host key mismatch") {
		return fmt.Errorf("Security alert: the device's SSH key has changed (possible replacement or attack).")
	}
	if strings.Contains(errStr, "network is unreachable") {
		return fmt.Errorf("Network unreachable: no network route to reach the device.")
	}

	// Caso não reconheça, retorna o erro original (ou uma versão simplificada)
	return err
}
