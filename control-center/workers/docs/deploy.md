# Deploy

Production deploys go through Cloudflare Workers Builds, connected to
`AnixOps/anix-control` with root directory `control-center/workers`, production
branch `go_dev`, and watch path `control-center/workers/**`. A merge to `go_dev`
that touches this directory deploys the `wrangler.toml` worker
(`api.anixops.com`).

## Staging

1. Configure Cloudflare bindings and secrets (`JWT_SECRET`, `API_KEY_SALT`).
2. Run `npm run typecheck`, `npm run build` and `npm run test:coverage:ci`.
3. Deploy to the staging environment.

## Production

1. Verify staging is healthy.
2. Merge the change to `go_dev`; Workers Builds deploys it.
3. Apply new D1 migrations with `npm run db:migrate`. Only add migrations; never
   edit or delete one that is already applied.
