# Testing Guide for Transaction ID Handling

## How to Run the Tests

### Prerequisites

1. **Go Testing Framework**: The tests use `testify/assert` and `testify/require`
2. **Test Database**: MySQL database for integration tests
3. **Build Tags**: Integration tests require `integration` build tag

### Install Dependencies

```bash
go get github.com/stretchr/testify/assert
go get github.com/stretchr/testify/require
```

### Run Unit Tests

```bash
# Run all unit tests
go test -v ./...

# Run specific test file
go test -v -run TestTransaction

# Run with coverage
go test -v -cover ./...
```

### Run Integration Tests

```bash
# Set up test database
mysql -u root -p -e "CREATE DATABASE mysql_test;"

# Run integration tests
go test -v -tags=integration ./...

# Run specific integration test
go test -v -tags=integration -run TestTransactionIntegrationDeadlock
```

### Test Database Configuration

Create a test database and user:

```sql
CREATE DATABASE mysql_test;
CREATE USER 'testuser'@'localhost' IDENTIFIED BY 'testpass';
GRANT ALL PRIVILEGES ON mysql_test.* TO 'testuser'@'localhost';
FLUSH PRIVILEGES;
```

## Test Files Overview

### 1. `transaction_test.go` - Basic Transaction Tests

**Coverage:**
- Transaction begin, commit, rollback
- Context handling and timeouts
- Isolation levels
- Concurrent transactions
- Savepoint functionality
- Error handling patterns
- Large data transactions
- Prepared statements

**Key Tests:**
```bash
go test -v -run TestTransactionBeginCommit
go test -v -run TestTransactionRollback
go test -v -run TestTransactionWithContext
go test -v -run TestConcurrentTransactions
```

### 2. `transaction_integration_test.go` - Integration Tests

**Coverage:**
- Deadlock detection and handling
- Lock wait timeouts
- Long-running transactions
- Connection pooling
- Isolation level behavior
- Context cancellation
- Error recovery

**Key Tests:**
```bash
go test -v -tags=integration -run TestTransactionIntegrationDeadlock
go test -v -tags=integration -run TestTransactionIntegrationLockWaitTimeout
go test -v -tags=integration -run TestTransactionIntegrationConnectionPool
```

### 3. `transaction_examples_test.go` - Best Practice Examples

**Coverage:**
- TransactionManager pattern
- Retry logic with exponential backoff
- Complex nested operations
- Comprehensive error handling
- Savepoint usage
- Health monitoring

**Key Tests:**
```bash
go test -v -run TestTransactionManagerExample
go test -v -run TestTransactionRetryExample
go test -v -run TestTransactionNestedOperationsExample
```

## Running Tests in Different Environments

### Local Development

```bash
# Quick test run (skip integration tests)
go test -v ./...

# Full test suite
go test -v -tags=integration ./...
```

### CI/CD Pipeline

```bash
# Run with race detection
go test -race -v ./...

# Run with coverage
go test -coverprofile=coverage.out -v ./...
go tool cover -html=coverage.out

# Run benchmarks
go test -bench=. -v ./...
```

### Docker Environment

```dockerfile
# Dockerfile for testing
FROM golang:1.21
RUN apt-get update && apt-get install -y mysql-client
COPY . /app
WORKDIR /app
RUN go mod download
CMD ["go", "test", "-v", "-tags=integration", "./..."]
```

## Test Configuration

### Environment Variables

```bash
# Test database DSN
export TEST_DSN="testuser:testpass@tcp(localhost:3306)/mysql_test?parseTime=true"

# Integration test DSN
export INTEGRATION_TEST_DSN="root:password@tcp(localhost:3306)/mysql_test?parseTime=true"
```

### Test Timeout Configuration

```bash
# Increase timeout for slow tests
go test -v -timeout=30s ./...

# Set specific test timeout
go test -v -timeout=60s -tags=integration ./...
```

## Performance Testing

### Benchmark Tests

```bash
# Run transaction benchmarks
go test -bench=BenchmarkTransaction -v ./...

# Run with memory profiling
go test -bench=BenchmarkTransaction -memprofile=mem.prof -v ./...
go tool pprof mem.prof
```

### Load Testing

```bash
# Run concurrent transaction test
go test -v -run TestConcurrentTransactions -count=10

# Stress test with high concurrency
go test -v -run TestTransactionIntegrationConnectionPool -count=50
```

## Troubleshooting

### Common Issues

1. **Connection Refused**
   ```bash
   # Check MySQL is running
   sudo systemctl status mysql
   
   # Start MySQL if needed
   sudo systemctl start mysql
   ```

2. **Permission Denied**
   ```sql
   -- Grant permissions to test user
   GRANT ALL PRIVILEGES ON mysql_test.* TO 'testuser'@'localhost';
   ```

3. **Database Doesn't Exist**
   ```sql
   -- Create test database
   CREATE DATABASE mysql_test;
   ```

4. **Integration Tests Skipped**
   ```bash
   # Ensure build tag is used
   go test -v -tags=integration ./...
   ```

### Debug Mode

```bash
# Run tests with verbose output
go test -v -tags=integration ./...

# Run specific test with debugging
go test -v -tags=integration -run TestTransactionIntegrationDeadlock -test.v
```

## Pull Request Testing

### Before Submitting PR

```bash
# Run full test suite
go test -v -race -cover ./...

# Run integration tests
go test -v -tags=integration -race ./...

# Run benchmarks
go test -bench=. -v ./...

# Check code formatting
go fmt ./...
go vet ./...
```

### Test Coverage Requirements

- Unit tests: >80% coverage
- Integration tests: All critical paths
- Examples: All documented patterns

## Continuous Integration

### GitHub Actions Example

```yaml
name: Transaction Tests
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    services:
      mysql:
        image: mysql:8.0
        env:
          MYSQL_ROOT_PASSWORD: password
          MYSQL_DATABASE: mysql_test
        ports:
          - 3306:3306
    steps:
    - uses: actions/checkout@v2
    - uses: actions/setup-go@v2
      with:
        go-version: 1.21
    - run: go mod download
    - run: go test -v -race -cover ./...
    - run: go test -v -tags=integration -race ./...
```

## Best Practices

1. **Test Naming**: Use descriptive test names that explain what is being tested
2. **Table-Driven Tests**: Use table-driven tests for multiple scenarios
3. **Cleanup**: Always clean up test data in defer statements
4. **Isolation**: Tests should not depend on each other
5. **Mocking**: Use mocks for external dependencies when appropriate
6. **Timeouts**: Set appropriate timeouts for integration tests
7. **Retry Logic**: Test retry logic with controlled failures
8. **Error Cases**: Test both success and failure scenarios

## Example Test Commands

```bash
# Quick smoke test
go test -v -run TestTransactionBeginCommit

# Full regression test
go test -v -race -cover -tags=integration ./...

# Performance test
go test -bench=BenchmarkTransaction -benchmem -v ./...

# Specific scenario test
go test -v -run TestTransactionRetryExample -count=5

# Debug failing test
go test -v -run TestTransactionIntegrationDeadlock -test.v
```
