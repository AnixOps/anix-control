### Security

- A login token (JWT) in the URL (`?token=`) is accepted only on a WebSocket
  handshake (the live monitor), not on ordinary API requests, and the text
  access log no longer writes query strings, so node `api_key`/`token`,
  subscription tokens and WebSocket JWTs stay out of the logs (the JSON
  access log already logged the path only).
