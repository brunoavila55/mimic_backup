package alert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"gorm.io/gorm"
	"mimic/internal/models"
	"mimic/pkg/crypto"
)

type WebhookPayload struct {
	Text string `json:"text"`
}

func Dispatch(db *gorm.DB, settings models.AlertSettings, message string) {
	if !settings.Enabled {
		return
	}

	go func() { // Async dispatch to avoid blocking the scheduler
		var err error
		if settings.Provider == "webhook" && settings.WebhookURL != "" {
			url, decErr := crypto.Decrypt(settings.WebhookURL)
			if decErr != nil {
				log.Printf("[Alert] Failed to decrypt Webhook URL: %v", decErr)
				return
			}
			err = sendWebhook(url, message)
		} else if settings.Provider == "telegram" && settings.TelegramToken != "" && settings.TelegramChatID != "" {
			token, decErr1 := crypto.Decrypt(settings.TelegramToken)
			chatID, decErr2 := crypto.Decrypt(settings.TelegramChatID)
			
			if decErr1 != nil || decErr2 != nil {
				log.Printf("[Alert] Failed to decrypt Telegram credentials")
				return
			}
			err = sendTelegram(token, chatID, message)
		} else {
			log.Printf("[Alert] Enabled but invalid provider config. Provider=%s", settings.Provider)
			return
		}

		if err != nil {
			msg := fmt.Sprintf("Failed to dispatch alert via %s: %v", settings.Provider, err)
			log.Printf("[Alert] %s", msg)
			if db != nil {
				db.Create(&models.SystemLog{Level: "error", Category: "system", Message: msg})
			}
		} else {
			msg := fmt.Sprintf("Successfully dispatched alert via %s", settings.Provider)
			log.Printf("[Alert] %s", msg)
			if db != nil {
				db.Create(&models.SystemLog{Level: "info", Category: "system", Message: msg})
			}
		}
	}()
}

func sendWebhook(url string, message string) error {
	payload := WebhookPayload{Text: message}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}

func sendTelegram(token string, chatID string, message string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	
	payload := map[string]string{
		"chat_id": chatID,
		"text":    message,
		"parse_mode": "Markdown",
	}
	
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("telegram API returned status %d", resp.StatusCode)
	}

	return nil
}
