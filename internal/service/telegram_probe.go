package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
)

// The Telegram probe sends one fixed message through the configured bot and
// classifies what the Bot API answered, for the administrator's "test the
// bot" action. It is separate from SendMessage and apiRequest on purpose:
// those return Go errors, and an error from net/http carries the whole
// request URL, which for the Bot API contains the bot token
// (https://api.telegram.org/bot<TOKEN>/sendMessage). The probe never returns,
// logs or wraps an error value from the HTTP client: a transport failure is
// reduced to a class and a reason from a fixed list.

// TelegramProbeClass is the outcome class of a probe.
type TelegramProbeClass string

// The classes a probe answers.
const (
	// TelegramProbeOK: Telegram accepted the message.
	TelegramProbeOK TelegramProbeClass = "ok"
	// TelegramProbeInvalidToken: the token is malformed or Telegram refused it
	// (401 Unauthorized, or 404 Not Found on the bot path).
	TelegramProbeInvalidToken TelegramProbeClass = "invalid_token"
	// TelegramProbeChatNotFound: the chat does not exist for this bot.
	TelegramProbeChatNotFound TelegramProbeClass = "chat_not_found"
	// TelegramProbeBotBlocked: Telegram answered 403, for example the user
	// blocked the bot or never started it.
	TelegramProbeBotBlocked TelegramProbeClass = "bot_blocked"
	// TelegramProbeRateLimited: Telegram answered 429.
	TelegramProbeRateLimited TelegramProbeClass = "rate_limited"
	// TelegramProbeNotConfigured: there is no bot row or it holds no token.
	TelegramProbeNotConfigured TelegramProbeClass = "not_configured"
	// TelegramProbeNetworkError: Telegram could not be reached (see Reason).
	TelegramProbeNetworkError TelegramProbeClass = "network_error"
	// TelegramProbeUnknown: any other answer.
	TelegramProbeUnknown TelegramProbeClass = "unknown"
)

// The reasons of a network_error, from a fixed list.
const (
	TelegramReasonTimeout  = "timeout"
	TelegramReasonDNS      = "dns"
	TelegramReasonTLS      = "tls"
	TelegramReasonConnect  = "connect"
	TelegramReasonCanceled = "canceled"
	TelegramReasonOther    = "other"
)

// TelegramProbeResult is the classified outcome of a probe. It holds no
// token, no URL and no text from Telegram or from a Go error.
type TelegramProbeResult struct {
	Class TelegramProbeClass `json:"class"`
	// ErrorCode is the Bot API's error_code (else the HTTP status) of a
	// refusal; 0 when there was none.
	ErrorCode int `json:"error_code,omitempty"`
	// Reason refines a network_error.
	Reason string `json:"reason,omitempty"`
}

// OK reports whether Telegram accepted the message.
func (r TelegramProbeResult) OK() bool { return r.Class == TelegramProbeOK }

// Message is a fixed sentence per class.
func (r TelegramProbeResult) Message() string {
	switch r.Class {
	case TelegramProbeOK:
		return "Telegram accepted the test message."
	case TelegramProbeInvalidToken:
		return "The bot token is not valid: Telegram refused it, or it is not shaped like a bot token."
	case TelegramProbeChatNotFound:
		return "Telegram does not know this chat: check the chat id."
	case TelegramProbeBotBlocked:
		return "Telegram refused delivery: the chat blocked the bot or has not started it."
	case TelegramProbeRateLimited:
		return "Telegram is rate limiting this bot: try again later."
	case TelegramProbeNotConfigured:
		return "No Telegram bot token is configured."
	case TelegramProbeNetworkError:
		return "Telegram could not be reached from this server."
	default:
		return "Telegram answered something this check does not recognise."
	}
}

// telegramAPIHost is the Bot API endpoint. Only tests change it
// (SetAPIBase); no request input reaches it.
const telegramAPIHost = "https://api.telegram.org"

// telegramProbeTimeout bounds a probe, connection and answer.
const telegramProbeTimeout = 10 * time.Second

// maxTelegramProbeAnswer bounds the answer a probe reads.
const maxTelegramProbeAnswer = 64 << 10

// telegramTokenShape is what a bot token looks like: the bot's numeric id, a
// colon and a secret of URL-safe characters. A token that does not match is
// never put in a URL, so it cannot change the request path.
var telegramTokenShape = regexp.MustCompile(`^[0-9]{1,20}:[A-Za-z0-9_-]{10,128}$`)

// SetAPIBase points the service's probe at another Bot API endpoint. It is
// for tests; the address comes from code, never from a request.
func (s *TelegramBotService) SetAPIBase(base string) {
	s.apiBase = strings.TrimRight(base, "/")
}

func (s *TelegramBotService) probeBase() string {
	if s.apiBase != "" {
		return s.apiBase
	}
	return telegramAPIHost
}

// ProbeSend sends text to chatID with bot's token and classifies the
// answer. It follows no redirect and gives up after ten seconds or when ctx
// ends. bot may be nil (no bot row).
func (s *TelegramBotService) ProbeSend(ctx context.Context, bot *model.TelegramBot, chatID int64, text string) TelegramProbeResult {
	if bot == nil || strings.TrimSpace(bot.Token) == "" {
		return TelegramProbeResult{Class: TelegramProbeNotConfigured}
	}
	token := strings.TrimSpace(bot.Token)
	if !telegramTokenShape.MatchString(token) {
		return TelegramProbeResult{Class: TelegramProbeInvalidToken}
	}
	body, err := json.Marshal(map[string]any{"chat_id": chatID, "text": text})
	if err != nil {
		return TelegramProbeResult{Class: TelegramProbeUnknown}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.probeBase()+"/bot"+token+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		// The error would quote the URL, and with it the token.
		return TelegramProbeResult{Class: TelegramProbeUnknown}
	}
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{
		Timeout:       telegramProbeTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	// The host is the fixed Bot API address (probeBase); only the shape-checked
	// token and the chat id vary.
	response, err := client.Do(request)
	if err != nil {
		return TelegramProbeResult{Class: TelegramProbeNetworkError, Reason: telegramNetworkReason(err)}
	}
	defer func() { _ = response.Body.Close() }()
	answer, err := io.ReadAll(io.LimitReader(response.Body, maxTelegramProbeAnswer))
	if err != nil {
		return TelegramProbeResult{Class: TelegramProbeNetworkError, Reason: telegramNetworkReason(err)}
	}
	return classifyTelegramAnswer(response.StatusCode, answer)
}

// classifyTelegramAnswer maps the HTTP status and the JSON answer of
// sendMessage to a class. The description is read only to tell the 400
// answers apart; it is not kept.
func classifyTelegramAnswer(status int, answer []byte) TelegramProbeResult {
	var parsed struct {
		OK          bool   `json:"ok"`
		ErrorCode   int    `json:"error_code"`
		Description string `json:"description"`
	}
	decoded := json.Unmarshal(answer, &parsed) == nil
	if decoded && parsed.OK && status >= 200 && status < 300 {
		return TelegramProbeResult{Class: TelegramProbeOK}
	}
	code := status
	if decoded && parsed.ErrorCode != 0 {
		code = parsed.ErrorCode
	}
	result := TelegramProbeResult{ErrorCode: code}
	switch {
	case code == http.StatusUnauthorized || code == http.StatusNotFound:
		result.Class = TelegramProbeInvalidToken
	case code == http.StatusForbidden:
		result.Class = TelegramProbeBotBlocked
	case code == http.StatusTooManyRequests:
		result.Class = TelegramProbeRateLimited
	case code == http.StatusBadRequest && telegramChatMissing(parsed.Description):
		result.Class = TelegramProbeChatNotFound
	default:
		result.Class = TelegramProbeUnknown
	}
	return result
}

// telegramChatMissing recognises the 400 answers for a chat the bot cannot
// address ("Bad Request: chat not found", "user not found", PEER_ID_INVALID).
func telegramChatMissing(description string) bool {
	description = strings.ToLower(description)
	return strings.Contains(description, "chat not found") || strings.Contains(description, "user not found") ||
		strings.Contains(description, "peer_id_invalid")
}

// telegramNetworkReason reduces a transport error to a reason from the fixed
// list. The error itself, which quotes the URL and so the token, goes no
// further.
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
		return TelegramReasonCanceled
	case errors.Is(err, context.DeadlineExceeded):
		return TelegramReasonTimeout
	case errors.As(err, &dnsError):
		return TelegramReasonDNS
	case errors.As(err, &unknownCA), errors.As(err, &hostname), errors.As(err, &invalid), errors.As(err, &verification), errors.As(err, &recordHeader):
		return TelegramReasonTLS
	case errors.As(err, &netError) && netError.Timeout():
		return TelegramReasonTimeout
	case errors.As(err, &opError):
		return TelegramReasonConnect
	default:
		return TelegramReasonOther
	}
}
