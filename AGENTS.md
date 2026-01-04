# Go-MySQL-Driver (`github.com/go-sql-driver/mysql`) — Agent Guide
 
 ## Project overview
 
 This repository implements a MySQL driver for Go’s `database/sql` package.
 
 It is a pure Go implementation of the `database/sql/driver` interfaces and is intended to be imported by applications as `github.com/go-sql-driver/mysql` (see `README.md`).
 
 ## Tech stack
 
 - **Language**: Go (see `go.mod`; module is `github.com/go-sql-driver/mysql`).
 - **Testing**: `go test` (CI runs tests against multiple MySQL and MariaDB versions).
 - **Static analysis**: Staticcheck (CI `lint` job).
 
 ## Repo layout
 
 The repository is largely a single Go package with implementation split across files:
 
 - `driver.go`, `connector.go`: `database/sql/driver` integration.
 - `dsn.go`: DSN parsing and formatting.
 - `connection.go`, `packets.go`: connection and MySQL protocol handling.
 - `auth.go`: authentication methods.
 - `infile.go`: `LOAD DATA LOCAL INFILE` support.
 - `*_test.go`: extensive test suite.
 - `examples/`: example usage.
 
 ## Build and test commands
 
 - **Build**:
 
 ```bash
 go build ./...
 ```
 
 - **Run tests**:
 
 ```bash
 go test ./...
 ```
 
 Notes:
 
 - The upstream CI runs with `-race` and coverage flags and spins up MySQL/MariaDB via GitHub Actions (`.github/workflows/test.yml`).
 - Some tests require a running MySQL instance and environment variables (also visible in CI):
   - `MYSQL_TEST_USER`, `MYSQL_TEST_PASS`, `MYSQL_TEST_ADDR`.
 
 ## Development conventions
 
 - When changing behavior, update/extend tests in the corresponding `*_test.go` files.
 - Check `.github/CONTRIBUTING.md` for contribution/testing expectations.
 
 ## Security considerations
 
 - Be careful when enabling `LOAD DATA LOCAL INFILE`; the README describes allowlisting and flags like `allowAllFiles=true`.
 - Prefer TLS settings (`tls` DSN parameter) when connecting over untrusted networks.
