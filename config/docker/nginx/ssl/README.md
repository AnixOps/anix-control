# TLS files

Place the production certificate chain at `cert.pem` and its private key at
`key.pem` before enabling the Compose `proxy` profile. Keep both files out of
version control and restrict the private key to the deployment operator.
