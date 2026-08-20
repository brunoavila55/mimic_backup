package alert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	neturl "net/url"
	"time"

	"gorm.io/gorm"
	"mimic/internal/models"
	"mimic/pkg/crypto"
)

type WebhookPayload struct {
	Text string `json:"text"`
}

var dispatchSlots = make(chan struct{}, 20)

func Dispatch(db *gorm.DB, rule models.AlertRule, message string) {
	if !rule.Enabled {
		return
	}

	select {
	case dispatchSlots <- struct{}{}:
	default:
		msg := fmt.Sprintf("Alert queue is full; skipped dispatch via %s", rule.Provider)
		log.Printf("[Alert] %s", msg)
		if db != nil {
			db.Create(&models.SystemLog{Level: "error", Category: "system", Message: msg})
		}
		return
	}

	go func() { // Async dispatch to avoid blocking the scheduler
		defer func() {
			<-dispatchSlots
			if recovered := recover(); recovered != nil {
				log.Printf("[Alert] Recovered from dispatch panic: %v", recovered)
			}
		}()
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

// ValidateWebhookURL rejects malformed URLs, non-HTTP(S) schemes, and hosts that
// resolve to loopback, link-local, or private network addresses. This prevents
// the webhook feature from being used to reach internal services (SSRF).
func ValidateWebhookURL(rawURL string) error {
	parsed, err := neturl.ParseRequestURI(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("enter a valid webhook URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("webhook URL must start with http:// or https://")
	}
	if isBlockedWebhookHost(parsed.Host) {
		return fmt.Errorf("webhook URL must not point to a private or internal network address")
	}
	return nil
}

func isBlockedWebhookHost(host string) bool {
	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}
	hostname = trimHostBrackets(hostname)

	ips, err := net.LookupIP(hostname)
	if err != nil || len(ips) == 0 {
		// Unable to resolve: fail safe and block rather than risk an internal target.
		return true
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return true
		}
	}
	return false
}

// trimHostBrackets strips the surrounding [] from an IPv6 literal host, if present.
func trimHostBrackets(host string) string {
	if len(host) >= 2 && host[0] == '[' && host[len(host)-1] == ']' {
		return host[1 : len(host)-1]
	}
	return host
}

func SendWebhook(url string, message string) error {
	if err := ValidateWebhookURL(url); err != nil {
		return err
	}

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
