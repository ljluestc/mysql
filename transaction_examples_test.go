// Go MySQL Driver - Transaction Examples and Best Practices Tests
//
// Copyright 2012 The Go-MySQL-Driver Authors. All rights reserved.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TransactionManager provides a reusable pattern for transaction management
type TransactionManager struct {
	db *sql.DB
}

// NewTransactionManager creates a new transaction manager
func NewTransactionManager(db *sql.DB) *TransactionManager {
	return &TransactionManager{db: db}
}

// ExecuteInTransaction executes a function within a transaction context
func (tm *TransactionManager) ExecuteInTransaction(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := tm.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if err != nil {
			// Attempt to rollback, don't overwrite the original error
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				log.Printf("failed to rollback transaction: %v", rollbackErr)
			}
		}
	}()

	// Execute the user function within the transaction
	err = fn(tx)
	if err != nil {
		return err
	}

	// Commit the transaction
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// TestTransactionManagerExample demonstrates the transaction manager pattern
func TestTransactionManagerExample(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	tm := NewTransactionManager(db)

	// Create test table
	_, err := db.Exec("CREATE TEMPORARY TABLE test_manager (id INT PRIMARY KEY, value VARCHAR(50))")
	require.NoError(t, err)

	err = tm.ExecuteInTransaction(context.Background(), func(tx *sql.Tx) error {
		// Insert user
		_, err := tx.Exec("INSERT INTO test_manager (id, value) VALUES (?, ?)", 1, "user1")
		if err != nil {
			return fmt.Errorf("failed to insert user: %w", err)
		}

		// Insert user profile
		_, err = tx.Exec("INSERT INTO test_manager (id, value) VALUES (?, ?)", 2, "profile1")
		if err != nil {
			return fmt.Errorf("failed to insert profile: %w", err)
		}

		return nil
	})

	assert.NoError(t, err, "Transaction manager should execute successfully")

	// Verify data was committed
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM test_manager").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 2, count, "All data should be committed")
}

// TestTransactionManagerRollback demonstrates rollback on error
func TestTransactionManagerRollback(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	tm := NewTransactionManager(db)

	// Create test table
	_, err := db.Exec("CREATE TEMPORARY TABLE test_manager_rollback (id INT PRIMARY KEY, value VARCHAR(50))")
	require.NoError(t, err)

	err = tm.ExecuteInTransaction(context.Background(), func(tx *sql.Tx) error {
		// Insert first record
		_, err := tx.Exec("INSERT INTO test_manager_rollback (id, value) VALUES (?, ?)", 1, "record1")
		if err != nil {
			return err
		}

		// Try to insert duplicate ID - should fail
		_, err = tx.Exec("INSERT INTO test_manager_rollback (id, value) VALUES (?, ?)", 1, "record2")
		if err != nil {
			return fmt.Errorf("duplicate insert failed: %w", err)
		}

		return nil
	})

	assert.Error(t, err, "Transaction should fail on duplicate key")

	// Verify no data was committed (rollback worked)
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM test_manager_rollback").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "No data should be committed after rollback")
}

// RetryConfig defines configuration for transaction retry logic
type RetryConfig struct {
	MaxRetries int
	Backoff    time.Duration
	MaxBackoff time.Duration
}

// DefaultRetryConfig returns a default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries: 3,
		Backoff:    100 * time.Millisecond,
		MaxBackoff: 5 * time.Second,
	}
}

// ExecuteWithRetry executes a transaction with retry logic for transient errors
func (tm *TransactionManager) ExecuteWithRetry(ctx context.Context, config RetryConfig, fn func(*sql.Tx) error) error {
	var lastErr error

	for attempt := 0; attempt < config.MaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff with jitter
			backoff := time.Duration(1<<uint(attempt)) * config.Backoff
			if backoff > config.MaxBackoff {
				backoff = config.MaxBackoff
			}
			
			// Add small random jitter to avoid thundering herd
			jitter := time.Duration(float64(backoff) * 0.1 * (float64(time.Now().UnixNano()%1000) / 1000.0))
			backoff += jitter
			
			log.Printf("Retrying transaction attempt %d after %v backoff", attempt+1, backoff)
			time.Sleep(backoff)
		}

		err := tm.ExecuteInTransaction(ctx, fn)
		if err == nil {
			return nil // Success
		}

		lastErr = err

		// Check if error is retryable
		if !isRetryableError(err) {
			break // Don't retry non-retryable errors
		}

		log.Printf("Transaction attempt %d failed: %v", attempt+1, err)
	}

	return fmt.Errorf("transaction failed after %d attempts, last error: %w", config.MaxRetries, lastErr)
}

// TestTransactionRetryExample demonstrates retry logic
func TestTransactionRetryExample(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	tm := NewTransactionManager(db)
	config := RetryConfig{
		MaxRetries: 2,
		Backoff:    10 * time.Millisecond,
		MaxBackoff: 100 * time.Millisecond,
	}

	// Create test table
	_, err := db.Exec("CREATE TEMPORARY TABLE test_retry (id INT PRIMARY KEY, value VARCHAR(50))")
	require.NoError(t, err)

	// Simulate a transaction that might fail and need retry
	attemptCount := 0
	err = tm.ExecuteWithRetry(context.Background(), config, func(tx *sql.Tx) error {
		attemptCount++
		
		// Insert data
		_, err := tx.Exec("INSERT INTO test_retry (id, value) VALUES (?, ?)", attemptCount, fmt.Sprintf("attempt_%d", attemptCount))
		if err != nil {
			return err
		}

		// Simulate occasional failure (only fail on first attempt)
		if attemptCount == 1 {
			return fmt.Errorf("simulated transient error")
		}

		return nil
	})

	assert.NoError(t, err, "Transaction should succeed after retry")
	assert.Equal(t, 2, attemptCount, "Should have attempted twice")

	// Verify final data was committed
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM test_retry").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "One record should be committed")
}

// TestTransactionTimeoutExample demonstrates timeout handling
func TestTransactionTimeoutExample(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	tm := NewTransactionManager(db)

	// Create test table
	_, err := db.Exec("CREATE TEMPORARY TABLE test_timeout (id INT PRIMARY KEY, value VARCHAR(50))")
	require.NoError(t, err)

	// Test with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// Wait for context to timeout
	time.Sleep(10 * time.Millisecond)

	err = tm.ExecuteInTransaction(ctx, func(tx *sql.Tx) error {
		// This should fail immediately due to context timeout
		_, err := tx.Exec("INSERT INTO test_timeout (id, value) VALUES (?, ?)", 1, "test")
		return err
	})

	assert.Error(t, err, "Transaction should fail due to context timeout")
	assert.Contains(t, err.Error(), "deadline exceeded", "Error should mention deadline exceeded")
}

// TestTransactionNestedOperationsExample demonstrates complex transaction operations
func TestTransactionNestedOperationsExample(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	tm := NewTransactionManager(db)

	// Create test tables
	_, err := db.Exec(`
		CREATE TEMPORARY TABLE users (
			id INT PRIMARY KEY AUTO_INCREMENT,
			name VARCHAR(50),
			email VARCHAR(50) UNIQUE
		);
		CREATE TEMPORARY TABLE user_profiles (
			user_id INT PRIMARY KEY,
			bio TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	require.NoError(t, err)

	err = tm.ExecuteInTransaction(context.Background(), func(tx *sql.Tx) error {
		// Insert user
		result, err := tx.Exec("INSERT INTO users (name, email) VALUES (?, ?)", "John Doe", "john@example.com")
		if err != nil {
			return fmt.Errorf("failed to insert user: %w", err)
		}

		userID, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("failed to get user ID: %w", err)
		}

		// Insert user profile
		_, err = tx.Exec("INSERT INTO user_profiles (user_id, bio) VALUES (?, ?)", userID, "Software Developer")
		if err != nil {
			return fmt.Errorf("failed to insert user profile: %w", err)
		}

		// Update user with profile reference
		_, err = tx.Exec("UPDATE users SET email = ? WHERE id = ?", "john.doe@example.com", userID)
		if err != nil {
			return fmt.Errorf("failed to update user: %w", err)
		}

		// Verify data within transaction
		var count int
		err = tx.QueryRow("SELECT COUNT(*) FROM users WHERE id = ?", userID).Scan(&count)
		if err != nil {
			return fmt.Errorf("failed to verify user: %w", err)
		}
		if count != 1 {
			return fmt.Errorf("user verification failed: expected 1, got %d", count)
		}

		err = tx.QueryRow("SELECT COUNT(*) FROM user_profiles WHERE user_id = ?", userID).Scan(&count)
		if err != nil {
			return fmt.Errorf("failed to verify profile: %w", err)
		}
		if count != 1 {
			return fmt.Errorf("profile verification failed: expected 1, got %d", count)
		}

		return nil
	})

	assert.NoError(t, err, "Complex transaction should succeed")

	// Verify final state
	var userCount, profileCount int
	err = db.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount)
	require.NoError(t, err)
	err = db.QueryRow("SELECT COUNT(*) FROM user_profiles").Scan(&profileCount)
	require.NoError(t, err)

	assert.Equal(t, 1, userCount, "One user should be created")
	assert.Equal(t, 1, profileCount, "One profile should be created")
}

// TestTransactionErrorHandlingExample demonstrates comprehensive error handling
func TestTransactionErrorHandlingExample(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	tm := NewTransactionManager(db)

	// Create test table with constraints
	_, err := db.Exec(`
		CREATE TEMPORARY TABLE error_handling_test (
			id INT PRIMARY KEY,
			value VARCHAR(50) NOT NULL,
			unique_field VARCHAR(50) UNIQUE
		)
	`)
	require.NoError(t, err)

	testCases := []struct {
		name        string
		transaction func(*sql.Tx) error
		expectError bool
		errorType   string
	}{
		{
			name: "Valid Transaction",
			transaction: func(tx *sql.Tx) error {
				_, err := tx.Exec("INSERT INTO error_handling_test (id, value, unique_field) VALUES (?, ?, ?)", 1, "valid", "unique1")
				return err
			},
			expectError: false,
		},
		{
			name: "Null Value Violation",
			transaction: func(tx *sql.Tx) error {
				_, err := tx.Exec("INSERT INTO error_handling_test (id, value, unique_field) VALUES (?, ?, ?)", 2, nil, "unique2")
				return err
			},
			expectError: true,
			errorType:   "Column 'value' cannot be null",
		},
		{
			name: "Duplicate Key",
			transaction: func(tx *sql.Tx) error {
				_, err := tx.Exec("INSERT INTO error_handling_test (id, value, unique_field) VALUES (?, ?, ?)", 3, "duplicate", "unique1")
				return err
			},
			expectError: true,
			errorType:   "Duplicate entry",
		},
		{
			name: "Primary Key Violation",
			transaction: func(tx *sql.Tx) error {
				_, err := tx.Exec("INSERT INTO error_handling_test (id, value, unique_field) VALUES (?, ?, ?)", 1, "pk_violation", "unique3")
				return err
			},
			expectError: true,
			errorType:   "Duplicate entry",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tm.ExecuteInTransaction(context.Background(), tc.transaction)
			
			if tc.expectError {
				assert.Error(t, err, "Expected transaction to fail")
				if tc.errorType != "" {
					assert.Contains(t, err.Error(), tc.errorType, "Error should contain expected type")
				}
			} else {
				assert.NoError(t, err, "Expected transaction to succeed")
			}
		})
	}

	// Verify only valid transaction was committed
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM error_handling_test").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "Only valid transaction should be committed")
}

// TestTransactionSavepointExample demonstrates savepoint usage
func TestTransactionSavepointExample(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	err := func() error {
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin transaction: %w", err)
		}
		defer func() {
			if err != nil {
				tx.Rollback()
			}
		}()

		// Create test table
		_, err = tx.Exec("CREATE TEMPORARY TABLE savepoint_test (id INT PRIMARY KEY, value VARCHAR(50))")
		if err != nil {
			return fmt.Errorf("failed to create table: %w", err)
		}

		// Insert initial data
		_, err = tx.Exec("INSERT INTO savepoint_test (id, value) VALUES (?, ?)", 1, "initial")
		if err != nil {
			return fmt.Errorf("failed to insert initial data: %w", err)
		}

		// Create savepoint
		_, err = tx.Exec("SAVEPOINT sp_initial")
		if err != nil {
			return fmt.Errorf("failed to create savepoint: %w", err)
		}

		// Insert more data
		_, err = tx.Exec("INSERT INTO savepoint_test (id, value) VALUES (?, ?)", 2, "after_savepoint")
		if err != nil {
			return fmt.Errorf("failed to insert data after savepoint: %w", err)
		}

		// Create another savepoint
		_, err = tx.Exec("SAVEPOINT sp_second")
		if err != nil {
			return fmt.Errorf("failed to create second savepoint: %w", err)
		}

		// Insert data that we'll rollback
		_, err = tx.Exec("INSERT INTO savepoint_test (id, value) VALUES (?, ?)", 3, "will_be_rolled_back")
		if err != nil {
			return fmt.Errorf("failed to insert data for rollback: %w", err)
		}

		// Rollback to second savepoint
		_, err = tx.Exec("ROLLBACK TO SAVEPOINT sp_second")
		if err != nil {
			return fmt.Errorf("failed to rollback to second savepoint: %w", err)
		}

		// Commit the transaction
		err = tx.Commit()
		if err != nil {
			return fmt.Errorf("failed to commit transaction: %w", err)
		}

		return nil
	}()

	assert.NoError(t, err, "Savepoint transaction should succeed")

	// Verify final state - should have 2 rows (initial and after_savepoint)
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM savepoint_test").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 2, count, "Should have 2 rows after savepoint rollback")

	// Verify which rows exist
	var values []string
	rows, err := db.Query("SELECT value FROM savepoint_test ORDER BY id")
	require.NoError(t, err)
	defer rows.Close()

	for rows.Next() {
		var value string
		err := rows.Scan(&value)
		require.NoError(t, err)
		values = append(values, value)
	}

	expected := []string{"initial", "after_savepoint"}
	assert.Equal(t, expected, values, "Should have correct values after savepoint operations")
}

// TestTransactionHealthCheckExample demonstrates transaction health monitoring
func TestTransactionHealthCheckExample(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	// Health check function
	performHealthCheck := func(db *sql.DB) error {
		// Check if we can begin a transaction
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("health check: failed to begin transaction: %w", err)
		}

		// Execute a simple query
		var result int
		err = tx.QueryRow("SELECT 1").Scan(&result)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("health check: failed to execute query: %w", err)
		}

		// Commit the health check transaction
		err = tx.Commit()
		if err != nil {
			return fmt.Errorf("health check: failed to commit transaction: %w", err)
		}

		if result != 1 {
			return fmt.Errorf("health check: unexpected result %d, expected 1", result)
		}

		return nil
	}

	// Perform health check
	err := performHealthCheck(db)
	assert.NoError(t, err, "Health check should pass")

	// Test health check with closed connection
	db.Close()
	err = performHealthCheck(db)
	assert.Error(t, err, "Health check should fail with closed connection")
	assert.Contains(t, err.Error(), "failed to begin transaction", "Error should mention transaction failure")
}
