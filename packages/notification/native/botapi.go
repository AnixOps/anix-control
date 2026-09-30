package native

import (
	"bytes"
	"encoding/json"
	"fmt"
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
var botAPIClient = &http.Client{Timeout: 30 * time.Second}

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
		return nil, err
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
