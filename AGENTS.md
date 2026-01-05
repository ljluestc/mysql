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

## AI Agent Workflow

### 1. Requirements Discovery
- **Primary Source**: `PRD.md` (Always prioritize this if present).
- **Secondary**: `requirements.txt`, `README.md`, or specific task files.
- **Goal**: Understand the full scope before writing code.

### 2. Implementation Protocol
- **Branching**: Work on a dedicated feature branch (e.g., `feat/implementation-details`).
- **Development**:
  - Analyze code structure.
  - Implement changes in `src/` or relevant directories.
  - Adhere to existing code style.
- **Verification**:
  - Run build commands (see above).
  - Run test suite (see above).
  - Ensure no regressions.

### 3. Delivery
- **Commit**: Use conventional commits (e.g., `feat: ...`, `fix: ...`).
- **PR Creation**:
  - Push branch: `git push -u origin <branch-name>`
  - Create a Pull Request against the main branch.
  - Summary: Link to `PRD.md` requirements solved.

## Task Implementation
1. **Analyze Requirements**: Refer to `README.md` for detailed feature specifications and system design.
2. **Implementation**: Modify source code in the respective directories (e.g., `src/`, `internal/`).
3. **Verification**: Run provided build and test commands (see above) to ensure correctness.
4. **Push Changes**:
   - Commit changes: `git commit -m "feat: implement <feature>"`
   - Push to remote: `git push origin <branch-name>`
