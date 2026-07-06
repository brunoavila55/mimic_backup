package alert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"mimic/internal/models"
)

type WebhookPayload struct {
	Text string `json:"text"`
}

func Dispatch(settings models.AlertSettings, message string) {
	if !settings.Enabled {
		return
	}

	go func() { // Async dispatch to avoid blocking the scheduler
		var err error
		if settings.Provider == "webhook" && settings.WebhookURL != "" {
			err = sendWebhook(settings.WebhookURL, message)
		} else if settings.Provider == "telegram" && settings.TelegramToken != "" && settings.TelegramChatID != "" {
			err = sendTelegram(settings.TelegramToken, settings.TelegramChatID, message)
		} else {
			log.Printf("[Alert] Enabled but invalid provider config. Provider=%s", settings.Provider)
			return
		}

		if err != nil {
			log.Printf("[Alert] Failed to dispatch alert via %s: %v", settings.Provider, err)
		} else {
			log.Printf("[Alert] Successfully dispatched alert via %s.", settings.Provider)
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
