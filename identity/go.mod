module github.com/AnixOps/anix-control/identity

go 1.25.0

toolchain go1.26.8

require (
	github.com/AnixOps/anix-control/sdk v0.0.0-00010101000000-000000000000
	github.com/golang-jwt/jwt/v5 v5.3.0
	github.com/stretchr/testify v1.11.1
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/AnixOps/anix-control/sdk => ../sdk
