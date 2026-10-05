package native

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"gorm.io/gorm"
)

// This file is the part of the kernel's TelegramBotService the native routes
// use: the Telegram Bot API calls, with the same URLs, payloads, client
// timeout and error handling. Like the kernel, it does not look at the "ok"
// field of the Bot API's answer: any JSON answer counts as success.

// telegramAPIBase is the Bot API endpoint the kernel calls.
const telegramAPIBase = "https://api.telegram.org"

// botAPIClient is the kernel service's HTTP client: a 30 second timeout and
// no request context.
// The Bot API host is fixed; a redirect is never followed.
var botAPIClient = &http.Client{
	Timeout:       30 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// firstBot is TelegramBotService.GetBot: the first bot row.
func firstBot(db *gorm.DB) (*Bot, error) {
	var bot Bot
	err := db.First(&bot).Error
	if err != nil {
		return nil, err
	}
	return &bot, nil
}

// sendMessage is TelegramBotService.SendMessage: a Markdown message to one
// chat with the configured bot.
func sendMessage(db *gorm.DB, chatID int64, text string) error {
	bot, err := firstBot(db)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/bot%s/sendMessage", telegramAPIBase, bot.Token)
	payload := map[string]any{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "Markdown",
	}

	_, err = botAPIRequest(url, payload)
	return err
}

// setWebhook is TelegramBotService.SetWebhook: it points the bot's webhook
// at webhookURL and records it.
func setWebhook(db *gorm.DB, webhookURL string) error {
	bot, err := firstBot(db)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/bot%s/setWebhook", telegramAPIBase, bot.Token)
	payload := map[string]any{
		"url": webhookURL,
	}

	_, err = botAPIRequest(url, payload)
	if err != nil {
		return err
	}

	bot.WebhookURL = webhookURL
	bot.WebhookSet = true
	return db.Save(bot).Error
}

// deleteWebhook is TelegramBotService.DeleteWebhook: it removes the bot's
// webhook and records that none is set.
func deleteWebhook(db *gorm.DB) error {
	bot, err := firstBot(db)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/bot%s/deleteWebhook", telegramAPIBase, bot.Token)
	_, err = botAPIRequest(url, nil)
	if err != nil {
		return err
	}

	bot.WebhookSet = false
	return db.Save(bot).Error
}

// botAPIRequest is TelegramBotService.apiRequest: a JSON POST whose answer
// must decode as a JSON object.
func botAPIRequest(url string, payload any) (map[string]any, error) {
	var body bytes.Buffer
	if payload != nil {
		if err := json.NewEncoder(&body).Encode(payload); err != nil {
			return nil, fmt.Errorf("encode telegram API request: %w", err)
		}
	}

	resp, err := botAPIClient.Post(url, "application/json", &body)
	if err != nil {
		// err is a *url.Error whose text quotes the URL, and with it the bot token.
		return nil, telegramTransportError(err)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		if closeErr := resp.Body.Close(); closeErr != nil {
			return nil, fmt.Errorf("decode telegram API response: %w; close response body: %v", err, closeErr)
		}
		return nil, err
	}
	if err := resp.Body.Close(); err != nil {
		return nil, fmt.Errorf("close telegram API response body: %w", err)
	}

	return result, nil
}

// telegramTransportError is the error the Bot API calls return when the HTTP
// client fails: the reason class only, never the client's error text, which
// quotes the request URL and so the bot token. It is the same text as the
// kernel's (internal/service telegramTransportError), which the route-mode
// shadow comparison checks byte for byte.
func telegramTransportError(err error) error {
	return fmt.Errorf("telegram API request failed: %s", telegramNetworkReason(err))
}

// telegramNetworkReason reduces a transport error to a reason from a fixed
// list (a copy of the kernel's helper of the same name).
func telegramNetworkReason(err error) string {
	var (
		dnsError     *net.DNSError
		netError     net.Error
		opError      *net.OpError
		unknownCA    x509.UnknownAuthorityError
		hostname     x509.HostnameError
		invalid      x509.CertificateInvalidError
		verification *tls.CertificateVerificationError
		recordHeader tls.RecordHeaderError
	)
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.As(err, &dnsError):
		return "dns"
	case errors.As(err, &unknownCA), errors.As(err, &hostname), errors.As(err, &invalid), errors.As(err, &verification), errors.As(err, &recordHeader):
		return "tls"
	case errors.As(err, &netError) && netError.Timeout():
		return "timeout"
	case errors.As(err, &opError):
		return "connect"
	default:
		return "other"
	}
}
