# Telegram Test Endpoint

`POST /api/v4/kernel/notifications/telegram/test` sends one fixed test
message through the configured Telegram bot and answers **what Telegram
said**, as a result class. It is how an administrator finds out whether the
bot token and a chat work, without sending a real notification.

The route is in the v4 administrator group (`/api/v4`, the same admission as
`/api/v3`: JWT, the administrator role, the admin rate limiter). The audit
middleware does not cover `/api/v4/kernel`, so the handler writes its own
operation log rows (below).

> `POST /api/v2/admin/notification/test` with `type: "telegram"` is a stub: it
> logs that Telegram delivery is not implemented and still answers success.
> It is a v2board-compatible route and is left as it is. The console's
> Notifications, Telegram, **Send test message** button calls this endpoint
> (`{}`, the administrator's own chat) and shows the class, `message` and
> `reason`; 409 says to link Telegram first and 429 counts down `Retry-After`.

## Request

```http
POST /api/v4/kernel/notifications/telegram/test
Authorization: Bearer <administrator token>
Content-Type: application/json

{"chat_id": 123456789}
```

The body and `chat_id` are optional.

| Field | Meaning |
|---|---|
| `chat_id` | The chat to send to. Omitted: the calling administrator's own bound Telegram chat (`v2_telegram_user`, a banned binding does not count). Given: it must be that chat or one of the bot's `admin_ids`; any other value is refused with 403 and nothing is sent. The bot cannot be used to message anybody else. |

The text is fixed (`<product> test message: ...`). No caller text, secret or
user data goes into it.

## Answer

`200` with the same envelope as the other kernel routes (`{"data": ...}`),
`Cache-Control: no-store`:

```json
{"data": {"class": "ok", "ok": true, "message": "Telegram accepted the test message.", "target": "self"}}
```

```json
{"data": {"class": "network_error", "ok": false, "reason": "dns", "message": "Telegram could not be reached from this server.", "target": "self"}}
```

| Field | Meaning |
|---|---|
| `class` | The result class, below. |
| `ok` | `true` only for class `ok`. |
| `message` | A fixed sentence per class. It is never Telegram's own text or a Go error. |
| `target` | `self` (the administrator's bound chat) or `bot_admin` (a chat from `admin_ids`). Absent for `not_configured`. |
| `error_code` | Telegram's `error_code` (else the HTTP status) of a refusal; absent when there was none. |
| `reason` | With `network_error` only: `timeout`, `dns`, `tls`, `connect`, `canceled` or `other`. |

| Class | Meaning | Usual fix |
|---|---|---|
| `ok` | Telegram accepted the message. | |
| `invalid_token` | The token is not shaped like a bot token (`<digits>:<secret>`; it is then not sent anywhere), or Telegram answered 401 or 404 for it. | Set the token from @BotFather in the bot settings. |
| `chat_not_found` | Telegram answered 400 "chat not found" (or "user not found"). | Check the chat id; for a user, the user must have started the bot. |
| `bot_blocked` | Telegram answered 403: the chat blocked the bot, never started it, or removed it. | Open the bot in Telegram and press Start. |
| `rate_limited` | Telegram answered 429. | Wait and retry. |
| `not_configured` | There is no bot row or its token is empty. | Save the bot settings. |
| `network_error` | Telegram could not be reached from Control (`reason` says how). | DNS, firewall or proxy of the Control host; `tls` is a TLS-intercepting proxy or a wrong system CA store. |
| `unknown` | Any other answer. | Look at `error_code`. |

Other answers:

| Status | `error.code` | When |
|---|---|---|
| 400 | `invalid_request` | The body is not a JSON object, or `chat_id` is not an integer, or it is 0. |
| 401, 403 | | No token, or not an administrator (the admission middleware). |
| 403 | `chat_not_allowed` | `chat_id` is neither the administrator's bound chat nor in the bot's `admin_ids`. |
| 409 | `telegram_not_bound` | No `chat_id`, and the administrator has no usable bound Telegram account. Pass a `chat_id` from `admin_ids`, or bind the account first. |
| 429 | `rate_limited` | More than 5 tests in a minute for this administrator, or 20 in ten minutes for all administrators. `Retry-After` says how many seconds to wait. |

## Rate limit and audit

- An attempt that gets past the checks above counts against the limits even
  when Telegram refuses it. A request refused by the limiter is logged at
  warning level and not written to the operation log, so a client hammering
  the route cannot fill the table.
- Every attempt that is not rate limited writes one `v2_operation_log` row:
  module `notification`, action `telegram_test`, target type `telegram_bot`,
  the administrator, the client address, status 1 (ok) or 2, and content
  `{"class": "...", "target": "self|bot_admin|none"}`. Refusals have the class
  `chat_not_allowed` or `no_target`. The row holds no token, no chat id and no
  Telegram text.

## What it never shows

The bot token is part of the Bot API URL
(`https://api.telegram.org/bot<TOKEN>/sendMessage`), and the error of Go's
HTTP client quotes the whole URL. The test therefore never returns, logs or
wraps such an error: a transport failure is reduced to `network_error` and a
`reason` from the fixed list, and Telegram's description is read only to tell
the 400 answers apart. The host is fixed, redirects are not followed (the
token cannot be sent elsewhere), the call has a 10 second limit and ends with
the request, and a token with a character outside `[A-Za-z0-9_-]` after the
colon is never put in a URL at all.

The bot's `enabled` flag is display-only today: no delivery path reads it and
no route sets it. The test follows what delivery does and does not stop on it.

## Known gap

The existing `SendMessage` and the notification package's copy
(`packages/notification/native/botapi.go`) return the HTTP client's error
as it is. Their error text, which contains the token, reaches
`v2_notification_log.error` and some administrator answers. That is outside
this endpoint and is tracked as a follow-up: it needs the kernel's and the
package's copy changed together, because the route-mode shadow comparison
checks that both answer alike.
