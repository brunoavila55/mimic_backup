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

func Dispatch(db *gorm.DB, rule models.AlertRule, message string) {
	if !rule.Enabled {
		return
	}

	go func() { // Async dispatch to avoid blocking the scheduler
		var err error
		if rule.Provider == "webhook" && rule.WebhookURL != "" {
			url, decErr := crypto.Decrypt(rule.WebhookURL)
			if decErr != nil {
				log.Printf("[Alert] Failed to decrypt Webhook URL: %v", decErr)
				return
			}
			err = SendWebhook(url, message)
		} else if rule.Provider == "telegram" && rule.TelegramToken != "" && rule.TelegramChatID != "" {
			token, decErr1 := crypto.Decrypt(rule.TelegramToken)
			chatID, decErr2 := crypto.Decrypt(rule.TelegramChatID)

			if decErr1 != nil || decErr2 != nil {
				log.Printf("[Alert] Failed to decrypt Telegram credentials")
				return
			}
			err = SendTelegram(token, chatID, message)
		} else {
			log.Printf("[Alert] Enabled but invalid provider config. Provider=%s", rule.Provider)
			return
		}

		if err != nil {
			msg := fmt.Sprintf("Failed to dispatch alert via %s: %v", rule.Provider, err)
			log.Printf("[Alert] %s", msg)
			if db != nil {
				db.Create(&models.SystemLog{Level: "error", Category: "system", Message: msg})
			}
		} else {
			msg := fmt.Sprintf("Successfully dispatched alert via %s", rule.Provider)
			log.Printf("[Alert] %s", msg)
			if db != nil {
				db.Create(&models.SystemLog{Level: "info", Category: "system", Message: msg})
			}
		}
	}()
}

func SendWebhook(url string, message string) error {
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

func SendTelegram(token string, chatID string, message string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)

	payload := map[string]string{
		"chat_id":    chatID,
		"text":       message,
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
