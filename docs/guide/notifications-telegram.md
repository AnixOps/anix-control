# Testing The Telegram Bot

After you save the Telegram bot token (**Telegram Bot** settings), check it
before you rely on it for notifications: the test sends one short message
through the bot and tells you what Telegram answered
([API reference](../reference/telegram-test-endpoint.md)).

## Before You Start

- The bot exists (@BotFather) and its token is saved in the bot settings.
- Your own administrator account is bound to Telegram (open the bot, press
  Start, then bind the account), or the target chat's id is in the bot's
  `admin_ids`. The test sends only to your bound chat or to one of the bot's
  `admin_ids`.

## Run The Test

```sh
curl -sS -X POST https://panel.example.com/api/v4/kernel/notifications/telegram/test \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' -d '{}'
```

To test a chat from the bot's `admin_ids`, add `{"chat_id": 123456789}`.

Read `data.class` in the answer:

| Class | What to do |
|---|---|
| `ok` | Nothing. The message arrived in the chat. |
| `invalid_token` | The token is wrong or revoked. Copy it again from @BotFather. |
| `chat_not_found` | The chat id is wrong, or the user never started the bot. |
| `bot_blocked` | The chat blocked the bot or never pressed Start. |
| `rate_limited` | Telegram is slowing the bot down. Wait a minute. |
| `not_configured` | Save the bot settings with a token first. |
| `network_error` | Control cannot reach `api.telegram.org`. `data.reason` is `dns`, `connect`, `timeout` or `tls`: check the Control host's DNS, firewall, outbound proxy and CA store (a TLS-intercepting proxy shows as `tls`). |
| `unknown` | Telegram answered something unexpected; `data.error_code` has its code. |

At most five tests a minute per administrator are accepted (HTTP 429 with
`Retry-After`). Every test is in the operation log (module `notification`,
action `telegram_test`) with its class, never the token.

The older "test notification" action for Telegram (`type: "telegram"` on
`/api/v2/admin/notification/test`) does not send anything and always reports
success; use this test instead.
