Subscription-only domain scaffolding for `sub.5555133.xyz`.

Files to fill:

- `ssl/sub.5555133.xyz.crt`
- `ssl/sub.5555133.xyz.key`

Nginx behavior:

- `http://sub.5555133.xyz` -> redirects to HTTPS
- `https://sub.5555133.xyz/s/...` -> proxies to `127.0.0.1:18080`
- every other path on HTTPS returns `403`

Apply after pasting the certificate:

```bash
sudo /home/dev/anixops/v2board_AnixOps/config/deploy/nginx/apply_subscription_domain.sh
```

After Nginx is live, add `sub.5555133.xyz` into `app.subscribe_domains` from the admin system page.
