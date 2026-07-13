package sftp

import (
	"errors"
	"fmt"
	"mimic/internal/models"
	"mimic/pkg/crypto"
	"net"
	"os"
	"path"
	"regexp"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"gorm.io/gorm"
)

type SftpService struct {
	DB *gorm.DB
}

func (s *SftpService) ListDir(settings *models.SftpSettings, remotePath string) ([]os.FileInfo, error) {
	config, err := s.getClientConfig(settings)
	if err != nil {
		return nil, err
	}

	addr := fmt.Sprintf("%s:%d", settings.Host, settings.Port)
	conn, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("failed to dial: %v", err)
	}
	defer conn.Close()

	client, err := sftp.NewClient(conn)
	if err != nil {
		return nil, fmt.Errorf("failed to create sftp client: %v", err)
	}
	defer client.Close()

	if remotePath == "" {
		remotePath = settings.Path
	}
	if remotePath == "" {
		remotePath = "/"
	}

	return client.ReadDir(remotePath)
}

func (s *SftpService) getClientConfig(settings *models.SftpSettings) (*ssh.ClientConfig, error) {
	password, err := crypto.Decrypt(settings.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt SFTP password: %w", err)
	}

	config := &ssh.ClientConfig{
		User:            settings.Username,
		Auth:            []ssh.AuthMethod{ssh.Password(password)},
		HostKeyCallback: s.verifyHostKey(settings),
		Timeout:         15 * time.Second,
	}
	return config, nil
}

// verifyHostKey implements trust-on-first-use for the configured SFTP server.
// The first key is persisted; subsequent connections must present the same key.
func (s *SftpService) verifyHostKey(settings *models.SftpSettings) ssh.HostKeyCallback {
	return func(_ string, _ net.Addr, key ssh.PublicKey) error {
		fingerprint := ssh.FingerprintSHA256(key)
		if settings.HostFingerprint != "" {
			if settings.HostFingerprint != fingerprint {
				return fmt.Errorf("SFTP host key mismatch: expected %s, got %s", settings.HostFingerprint, fingerprint)
			}
			return nil
		}

		if s.DB == nil || settings.ID == 0 {
			return errors.New("SFTP host fingerprint is not configured and cannot be persisted")
		}

		result := s.DB.Model(&models.SftpSettings{}).
			Where("id = ? AND (host_fingerprint IS NULL OR host_fingerprint = '')", settings.ID).
			Update("host_fingerprint", fingerprint)
		if result.Error != nil {
			return fmt.Errorf("failed to persist SFTP host fingerprint: %w", result.Error)
		}
		if result.RowsAffected == 1 {
			settings.HostFingerprint = fingerprint
			return nil
		}

		var current models.SftpSettings
		if err := s.DB.Select("host_fingerprint").First(&current, settings.ID).Error; err != nil {
			return fmt.Errorf("failed to verify persisted SFTP host fingerprint: %w", err)
		}
		if current.HostFingerprint != fingerprint {
			return fmt.Errorf("SFTP host key mismatch: expected %s, got %s", current.HostFingerprint, fingerprint)
		}
		settings.HostFingerprint = current.HostFingerprint
		return nil
	}
}

func (s *SftpService) TestConnection(settings *models.SftpSettings) error {
	config, err := s.getClientConfig(settings)
	if err != nil {
		return err
	}

	addr := fmt.Sprintf("%s:%d", settings.Host, settings.Port)
	conn, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return fmt.Errorf("failed to connect to SFTP server: %v", err)
	}
	defer conn.Close()

	client, err := sftp.NewClient(conn)
	if err != nil {
		return fmt.Errorf("failed to create SFTP session: %v", err)
	}
	defer client.Close()

	if settings.Path != "" {
		err = client.MkdirAll(settings.Path)
		if err != nil {
			return fmt.Errorf("failed to verify or create remote directory '%s': %v", settings.Path, err)
		}
	}

	return nil
}

func (s *SftpService) Export(backup *models.NodeBackup, settings *models.SftpSettings) error {
	config, err := s.getClientConfig(settings)
	if err != nil {
		return err
	}

	addr := fmt.Sprintf("%s:%d", settings.Host, settings.Port)
	conn, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return fmt.Errorf("failed to dial: %v", err)
	}
	defer conn.Close()

	client, err := sftp.NewClient(conn)
	if err != nil {
		return fmt.Errorf("failed to create sftp client: %v", err)
	}
	defer client.Close()

	// Ensure remote directory exists
	if settings.Path != "" {
		err = client.MkdirAll(settings.Path)
		if err != nil {
			return fmt.Errorf("failed to create remote directory: %v", err)
		}
	}

	// Sanitize Node Name to prevent Path Traversal
	safeName := regexp.MustCompile(`[^a-zA-Z0-9_-]`).ReplaceAllString(backup.Node.Name, "_")

	filename := fmt.Sprintf("%s_%v.txt", safeName, backup.CreatedAt.Format("20060102_1504"))
	remotePath := path.Join(settings.Path, filename) // Use path.Join to ensure /

	f, err := client.Create(remotePath)
	if err != nil {
		return fmt.Errorf("failed to create remote file: %v", err)
	}
	defer f.Close()

	if _, err := f.Write([]byte(backup.Config)); err != nil {
		return fmt.Errorf("failed to write to remote file: %v", err)
	}

	return nil
}
