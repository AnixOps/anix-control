### Changed

- CI: the mocked browser E2E gate runs in three parallel jobs, Frontend E2E
  (1/3 to 3/3), against the production bundle (`vite preview` of `public/`)
  instead of the dev server, and records traces and videos only for a
  retried test instead of every test. The rest of the old Frontend Build job
  is now Frontend Checks; Frontend Build, still the required check, passes
  when Frontend Checks and every shard passed. Locally `npm run test:e2e`
  still uses the dev server.
