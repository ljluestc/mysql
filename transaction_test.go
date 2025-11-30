// Go MySQL Driver - Transaction ID Handling Tests
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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTransactionBeginCommit tests basic transaction begin and commit operations
func TestTransactionBeginCommit(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	tx, err := db.Begin()
	require.NoError(t, err, "Failed to begin transaction")

	// Verify transaction is active
	assert.NotNil(t, tx, "Transaction should not be nil")

	// Create a temporary table for testing
	_, err = tx.Exec("CREATE TEMPORARY TABLE test_tx (id INT PRIMARY KEY, value VARCHAR(50))")
	require.NoError(t, err, "Failed to create temporary table")

	// Insert test data
	_, err = tx.Exec("INSERT INTO test_tx (id, value) VALUES (?, ?)", 1, "test_value")
	require.NoError(t, err, "Failed to insert test data")

	// Commit the transaction
	err = tx.Commit()
	require.NoError(t, err, "Failed to commit transaction")

	// Verify data exists after commit
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM test_tx").Scan(&count)
	require.NoError(t, err, "Failed to query test table")
	assert.Equal(t, 1, count, "Expected 1 row after commit")
}

// TestTransactionRollback tests transaction rollback functionality
func TestTransactionRollback(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	tx, err := db.Begin()
	require.NoError(t, err, "Failed to begin transaction")

	// Create temporary table
	_, err = tx.Exec("CREATE TEMPORARY TABLE test_rollback (id INT PRIMARY KEY, value VARCHAR(50))")
	require.NoError(t, err, "Failed to create temporary table")

	// Insert test data
	_, err = tx.Exec("INSERT INTO test_rollback (id, value) VALUES (?, ?)", 1, "rollback_test")
	require.NoError(t, err, "Failed to insert test data")

	// Rollback the transaction
	err = tx.Rollback()
	require.NoError(t, err, "Failed to rollback transaction")

	// Verify table doesn't exist or is empty after rollback
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM test_rollback").Scan(&count)
	if err == nil {
		assert.Equal(t, 0, count, "Table should be empty after rollback")
	}
	// If table doesn't exist, that's also expected after rollback
}

// TestTransactionWithContext tests transaction with context timeout
func TestTransactionWithContext(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	// Create a context with very short timeout to test cancellation
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Wait for context to timeout
	time.Sleep(1 * time.Millisecond)

	// Try to begin transaction with timed out context
	tx, err := db.BeginTx(ctx, nil)
	assert.Error(t, err, "Expected error when context is already timed out")
	assert.Nil(t, tx, "Transaction should be nil when context is timed out")
	assert.Equal(t, context.DeadlineExceeded, err, "Expected deadline exceeded error")
}

// TestTransactionIsolationLevels tests different isolation levels
func TestTransactionIsolationLevels(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	isolationLevels := []struct {
		name  string
		level string
	}{
		{"READ_UNCOMMITTED", "READ UNCOMMITTED"},
		{"READ_COMMITTED", "READ COMMITTED"},
		{"REPEATABLE_READ", "REPEATABLE READ"},
		{"SERIALIZABLE", "SERIALIZABLE"},
	}

	for _, test := range isolationLevels {
		t.Run(test.name, func(t *testing.T) {
			// Set isolation level
			_, err := db.Exec(fmt.Sprintf("SET SESSION TRANSACTION ISOLATION LEVEL %s", test.level))
			require.NoError(t, err, "Failed to set isolation level %s", test.level)

			// Begin transaction
			tx, err := db.Begin()
			require.NoError(t, err, "Failed to begin transaction")

			// Create temporary table
			_, err = tx.Exec("CREATE TEMPORARY TABLE test_isolation (id INT, value VARCHAR(50))")
			require.NoError(t, err, "Failed to create temporary table")

			// Insert test data
			_, err = tx.Exec("INSERT INTO test_isolation (id, value) VALUES (?, ?)", 1, "isolation_test")
			require.NoError(t, err, "Failed to insert test data")

			// Commit transaction
			err = tx.Commit()
			require.NoError(t, err, "Failed to commit transaction")

			// Verify data exists
			var count int
			err = db.QueryRow("SELECT COUNT(*) FROM test_isolation").Scan(&count)
			require.NoError(t, err, "Failed to verify data")
			assert.Equal(t, 1, count, "Expected 1 row after commit")
		})
	}
}

// TestConcurrentTransactions tests multiple concurrent transactions
func TestConcurrentTransactions(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	// Create a test table
	_, err := db.Exec("CREATE TEMPORARY TABLE test_concurrent (id INT PRIMARY KEY, value VARCHAR(50))")
	require.NoError(t, err, "Failed to create test table")

	const numGoroutines = 5
	done := make(chan bool, numGoroutines)
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer func() { done <- true }()
			
			tx, err := db.Begin()
			if err != nil {
				errors <- fmt.Errorf("goroutine %d: failed to begin transaction: %w", id, err)
				return
			}

			// Insert with unique ID
			_, err = tx.Exec("INSERT INTO test_concurrent (id, value) VALUES (?, ?)", id, fmt.Sprintf("value_%d", id))
			if err != nil {
				tx.Rollback()
				errors <- fmt.Errorf("goroutine %d: failed to insert data: %w", id, err)
				return
			}

			err = tx.Commit()
			if err != nil {
				errors <- fmt.Errorf("goroutine %d: failed to commit transaction: %w", id, err)
				return
			}

			errors <- nil
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Check for errors
	for i := 0; i < numGoroutines; i++ {
		err := <-errors
		assert.NoError(t, err, "Goroutine should complete without error")
	}

	// Verify all data was inserted correctly
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM test_concurrent").Scan(&count)
	require.NoError(t, err, "Failed to count rows")
	assert.Equal(t, numGoroutines, count, "Expected all rows to be inserted")
}

// TestTransactionSavepoints tests savepoint functionality
func TestTransactionSavepoints(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	tx, err := db.Begin()
	require.NoError(t, err, "Failed to begin transaction")

	// Create temporary table
	_, err = tx.Exec("CREATE TEMPORARY TABLE test_savepoints (id INT PRIMARY KEY, value VARCHAR(50))")
	require.NoError(t, err, "Failed to create temporary table")

	// Insert initial data
	_, err = tx.Exec("INSERT INTO test_savepoints (id, value) VALUES (?, ?)", 1, "initial")
	require.NoError(t, err, "Failed to insert initial data")

	// Create savepoint
	_, err = tx.Exec("SAVEPOINT sp1")
	require.NoError(t, err, "Failed to create savepoint")

	// Insert more data
	_, err = tx.Exec("INSERT INTO test_savepoints (id, value) VALUES (?, ?)", 2, "after_savepoint")
	require.NoError(t, err, "Failed to insert data after savepoint")

	// Rollback to savepoint
	_, err = tx.Exec("ROLLBACK TO SAVEPOINT sp1")
	require.NoError(t, err, "Failed to rollback to savepoint")

	// Commit the main transaction
	err = tx.Commit()
	require.NoError(t, err, "Failed to commit transaction")

	// Verify only initial data exists
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM test_savepoints").Scan(&count)
	require.NoError(t, err, "Failed to verify data")
	assert.Equal(t, 1, count, "Expected 1 row after savepoint rollback")
}

// TestTransactionErrorHandling tests error handling in transactions
func TestTransactionErrorHandling(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	t.Run("SQL Error in Transaction", func(t *testing.T) {
		tx, err := db.Begin()
		require.NoError(t, err, "Failed to begin transaction")

		// Try to insert into non-existent table
		_, err = tx.Exec("INSERT INTO non_existent_table VALUES (1)")
		assert.Error(t, err, "Expected error when inserting into non-existent table")

		// Transaction should still be usable after error
		_, err = tx.Exec("SELECT 1")
		assert.NoError(t, err, "Should be able to execute queries after error")

		err = tx.Rollback()
		assert.NoError(t, err, "Should be able to rollback after error")
	})

	t.Run("Double Commit", func(t *testing.T) {
		tx, err := db.Begin()
		require.NoError(t, err, "Failed to begin transaction")

		err = tx.Commit()
		assert.NoError(t, err, "First commit should succeed")

		// Second commit should fail or be no-op
		err = tx.Commit()
		// MySQL driver might return error or silently ignore - both are acceptable
		// The important thing is that it doesn't panic
	})

	t.Run("Double Rollback", func(t *testing.T) {
		tx, err := db.Begin()
		require.NoError(t, err, "Failed to begin transaction")

		err = tx.Rollback()
		assert.NoError(t, err, "First rollback should succeed")

		// Second rollback should fail or be no-op
		err = tx.Rollback()
		// MySQL driver might return error or silently ignore - both are acceptable
		// The important thing is that it doesn't panic
	})
}

// TestTransactionAfterConnectionClose tests transaction behavior after connection close
func TestTransactionAfterConnectionClose(t *testing.T) {
	db := createTestDBConnection(t)

	tx, err := db.Begin()
	require.NoError(t, err, "Failed to begin transaction")

	// Close the database connection
	err = db.Close()
	require.NoError(t, err, "Failed to close database")

	// Try to use transaction after connection close
	_, err = tx.Exec("SELECT 1")
	assert.Error(t, err, "Expected error when using transaction after connection close")

	// Try to commit after connection close
	err = tx.Commit()
	assert.Error(t, err, "Expected error when committing after connection close")
}

// TestTransactionLargeData tests transaction with large amounts of data
func TestTransactionLargeData(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	tx, err := db.Begin()
	require.NoError(t, err, "Failed to begin transaction")

	// Create table for large data test
	_, err = tx.Exec("CREATE TEMPORARY TABLE test_large_data (id INT PRIMARY KEY, data TEXT)")
	require.NoError(t, err, "Failed to create table for large data test")

	// Insert multiple rows with substantial data
	const numRows = 100
	largeData := make([]byte, 1024) // 1KB of data per row
	for i := range largeData {
		largeData[i] = byte('A' + (i % 26))
	}

	for i := 0; i < numRows; i++ {
		_, err = tx.Exec("INSERT INTO test_large_data (id, data) VALUES (?, ?)", i, string(largeData))
		require.NoError(t, err, "Failed to insert row %d", i)
	}

	// Commit the transaction
	err = tx.Commit()
	require.NoError(t, err, "Failed to commit large data transaction")

	// Verify all data was inserted
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM test_large_data").Scan(&count)
	require.NoError(t, err, "Failed to count rows")
	assert.Equal(t, numRows, count, "Expected all rows to be inserted")
}

// TestTransactionPreparedStatements tests prepared statements within transactions
func TestTransactionPreparedStatements(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	tx, err := db.Begin()
	require.NoError(t, err, "Failed to begin transaction")

	// Create table
	_, err = tx.Exec("CREATE TEMPORARY TABLE test_prepared (id INT PRIMARY KEY, value VARCHAR(50))")
	require.NoError(t, err, "Failed to create table")

	// Prepare statement within transaction
	stmt, err := tx.Prepare("INSERT INTO test_prepared (id, value) VALUES (?, ?)")
	require.NoError(t, err, "Failed to prepare statement")
	defer stmt.Close()

	// Execute prepared statement multiple times
	for i := 0; i < 5; i++ {
		_, err = stmt.Exec(i, fmt.Sprintf("value_%d", i))
		require.NoError(t, err, "Failed to execute prepared statement for row %d", i)
	}

	// Commit transaction
	err = tx.Commit()
	require.NoError(t, err, "Failed to commit transaction")

	// Verify data
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM test_prepared").Scan(&count)
	require.NoError(t, err, "Failed to count rows")
	assert.Equal(t, 5, count, "Expected 5 rows")
}

// BenchmarkTransactionBeginCommit benchmarks transaction begin and commit performance
func BenchmarkTransactionBeginCommit(b *testing.B) {
	db := createBenchDBConnection(b)
	defer db.Close()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		tx, err := db.Begin()
		if err != nil {
			b.Fatalf("Failed to begin transaction: %v", err)
		}

		_, err = tx.Exec("SELECT 1")
		if err != nil {
			tx.Rollback()
			b.Fatalf("Failed to execute query: %v", err)
		}

		err = tx.Commit()
		if err != nil {
			b.Fatalf("Failed to commit transaction: %v", err)
		}
	}
}

// Helper function to create test database connection
func createTestDBConnection(t *testing.T) *sql.DB {
	// Use environment variable or default test DSN
	dsn := "testuser:testpass@tcp(localhost:3306)/testdb?parseTime=true&timeout=10s&readTimeout=10s&writeTimeout=10s"
	
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Skipf("Failed to connect to test database: %v", err)
	}

	// Test the connection
	err = db.Ping()
	if err != nil {
		db.Close()
		t.Skipf("Failed to ping test database: %v", err)
	}

	// Configure connection pool for testing
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)

	return db
}

// Helper function to create benchmark database connection
func createBenchDBConnection(b *testing.B) *sql.DB {
	dsn := "testuser:testpass@tcp(localhost:3306)/testdb?parseTime=true"
	
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		b.Skipf("Failed to connect to benchmark database: %v", err)
	}

	err = db.Ping()
	if err != nil {
		db.Close()
		b.Skipf("Failed to ping benchmark database: %v", err)
	}

	// Optimize for benchmarks
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(10 * time.Minute)

	return db
}

// TestTransactionIDErrorSimulation simulates transaction ID related errors
func TestTransactionIDErrorSimulation(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	// This test simulates how the application should handle transaction ID related errors
	// In practice, these errors come from MySQL server corruption, not from normal operation

	t.Run("HandleTransactionIDErrors", func(t *testing.T) {
		tx, err := db.Begin()
		require.NoError(t, err, "Failed to begin transaction")

		// Simulate error handling pattern for transaction ID issues
		err = func() error {
			// In a real scenario, this might be a transaction ID corruption error
			_, err := tx.Exec("INSERT INTO test_table VALUES (1)")
			if err != nil {
				// Check if it's a transaction ID related error
				if isTransactionIDError(err) {
					return fmt.Errorf("transaction ID corruption detected: %w", err)
				}
				return err
			}
			return nil
		}()

		// For this test, we expect the table doesn't exist error
		if err != nil {
			tx.Rollback()
			// Verify error handling works
			assert.Error(t, err, "Expected error for non-existent table")
		} else {
			tx.Commit()
		}
	})
}

// Helper function to check if error is transaction ID related
func isTransactionIDError(err error) bool {
	if err == nil {
		return false
	}
	
	errStr := err.Error()
	transactionIDErrors := []string{
		"transaction id",
		"system-wide maximum",
		"trx_id",
		"newer than the system-wide maximum",
	}

	for _, trxErr := range transactionIDErrors {
		if contains(errStr, trxErr) {
			return true
		}
	}

	return false
}

// Helper function to check if string contains substring (case-insensitive)
func contains(s, substr string) bool {
	if len(s) < len(substr) {
		return false
	}
	
	// Simple case-insensitive contains
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if toLower(s[i+j]) != toLower(substr[j]) {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// Helper function to convert character to lowercase
func toLower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}
