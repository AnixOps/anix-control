### Added

- Telegram binding without the bot's commands: users link Telegram on the
  Account page (a QR code / button for a one-time t.me link to the bot, 10
  minutes) and unlink there; administrators link a user to a Telegram ID by
  hand and unlink from Notifications → Telegram → 已绑定用户.
  `GET/DELETE /api/v4/user/telegram`, `POST /api/v4/user/telegram/bind-link`,
  `POST /api/v4/admin/telegram/bindings`,
  `DELETE /api/v4/admin/telegram/bindings/{user_id}`.

### Fixed

- Telegram bot: `/bind` ignored its argument, so no member could bind. It now
  binds with the member's subscription link or token, in a private chat only,
  and respects the bot's "allow bind" switch (an e-mail alone is not accepted:
  it does not prove ownership). `/sub` answers the real subscription link
  (first subscription domain, else the panel domain) instead of
  `your-domain.com`.
