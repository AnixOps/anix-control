### Fixed

- Pages of the admin UI no longer show "didn't load" when a quick page change
  spends the API rate limit. The admin APIs share one bucket per client
  address (10 requests a second, burst 20); a read refused with 429 is now sent
  again after the server's `Retry-After` (at most 5 s, at most twice). Writes
  are never repeated.
