### Security

- The web build uses `follow-redirects` 1.16.2, which fixes advisory
  GHSA-mrwp-f3q8-3g9p (credentials following redirects). The Frontend Build
  audit gate failed on it for every pull request.
