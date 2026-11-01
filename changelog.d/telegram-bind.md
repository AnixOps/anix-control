### Fixed

- Telegram bot: `/bind` ignored its argument, so no member could bind. It now
  binds with the member's subscription link or token, in a private chat only,
  and respects the bot's "allow bind" switch (an e-mail alone is not accepted:
  it does not prove ownership). `/sub` answers the real subscription link
  (first subscription domain, else the panel domain) instead of
  `your-domain.com`.
