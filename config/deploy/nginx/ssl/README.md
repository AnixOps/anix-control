Place the Cloudflare origin certificate pair used by the panel Nginx here.

Files:

- `panel-origin.crt`
- `panel-origin.key`
- `server-names.txt`

Expected contents:

- `panel-origin.crt`: PEM certificate block, beginning with `-----BEGIN CERTIFICATE-----`
- `panel-origin.key`: PEM private key block, beginning with `-----BEGIN PRIVATE KEY-----` or `-----BEGIN RSA PRIVATE KEY-----`
- `server-names.txt`: one server name per line, starting with the current compatibility domain

Current compatibility domain:

- `x.kalijerry.uk`

Recommended certificate strategy:

- If future domains stay under the same parent domain, prefer a wildcard or SAN certificate.
- If future domains are unrelated names, issue a Cloudflare origin certificate that covers every hostname you will put into `server_name`.

Suggested system destination names under `/etc/nginx/ssl/`:

- `/etc/nginx/ssl/panel-origin.crt`
- `/etc/nginx/ssl/panel-origin.key`

This workspace copy is only a staging location so you can paste the certificate material safely before moving it into the system path.
