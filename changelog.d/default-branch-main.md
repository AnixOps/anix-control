### Changed

- The default branch of `AnixOps/anix-control` is `main` (was `go_dev`). CI,
  the nightly security run, the `:edge` image build and Cloudflare Workers
  Builds follow `main`, and the documentation links in the admin UI, the
  installers and the systemd units point at `main`. Update a local clone with
  `git branch -m go_dev main && git fetch origin && git branch -u origin/main main && git remote set-head origin -a`.
