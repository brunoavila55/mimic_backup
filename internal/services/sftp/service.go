package sftp

import (
	"fmt"
	"mimic/internal/models"
	"mimic/pkg/crypto"
	"path/filepath"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type SftpService struct{}

func (s *SftpService) Export(backup *models.NodeBackup, settings *models.SftpSettings) error {
	password, err := crypto.Decrypt(settings.Password)
	if err != nil {
		password = settings.Password
	}

	config := &ssh.ClientConfig{
		User:            settings.Username,
		Auth:            []ssh.AuthMethod{ssh.Password(password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
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

	filename := fmt.Sprintf("%s_%v.txt", backup.Node.Name, backup.CreatedAt.Format("20060102_1504"))
	remotePath := filepath.Join(settings.Path, filename)

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
