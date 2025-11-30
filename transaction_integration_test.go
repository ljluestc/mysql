// Go MySQL Driver - Transaction Integration Tests
//
// Copyright 2012 The Go-MySQL-Driver Authors. All rights reserved.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

//go:build integration
// +build integration

package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTransactionIntegrationDeadlock tests deadlock detection and handling
func TestTransactionIntegrationDeadlock(t *testing.T) {
	db := createIntegrationDB(t)
	defer db.Close()

	// Create test table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS deadlock_test (
			id INT PRIMARY KEY,
			value VARCHAR(50),
			version INT DEFAULT 0
		) ENGINE=InnoDB
	`)
	require.NoError(t, err)

	// Clean up any existing data
	db.Exec("DELETE FROM deadlock_test")

	// Insert initial data
	_, err = db.Exec("INSERT INTO deadlock_test (id, value) VALUES (1, 'row1'), (2, 'row2')")
	require.NoError(t, err)

	var wg sync.WaitGroup
	errors := make(chan error, 2)

	// Transaction 1: Update row 1 then row 2
	wg.Add(1)
	go func() {
		defer wg.Done()
		tx, err := db.Begin()
		if err != nil {
			errors <- fmt.Errorf("tx1 begin failed: %w", err)
			return
		}

		// Update row 1
		_, err = tx.Exec("UPDATE deadlock_test SET value = 'tx1_row1' WHERE id = 1")
		if err != nil {
			tx.Rollback()
			errors <- fmt.Errorf("tx1 update row1 failed: %w", err)
			return
		}

		// Small delay to increase chance of deadlock
		time.Sleep(10 * time.Millisecond)

		// Try to update row 2 (might cause deadlock)
		_, err = tx.Exec("UPDATE deadlock_test SET value = 'tx1_row2' WHERE id = 2")
		if err != nil {
			tx.Rollback()
			errors <- fmt.Errorf("tx1 update row2 failed: %w", err)
			return
		}

		err = tx.Commit()
		if err != nil {
			errors <- fmt.Errorf("tx1 commit failed: %w", err)
			return
		}

		errors <- nil
	}()

	// Transaction 2: Update row 2 then row 1
	wg.Add(1)
	go func() {
		defer wg.Done()
		tx, err := db.Begin()
		if err != nil {
			errors <- fmt.Errorf("tx2 begin failed: %w", err)
			return
		}

		// Update row 2
		_, err = tx.Exec("UPDATE deadlock_test SET value = 'tx2_row2' WHERE id = 2")
		if err != nil {
			tx.Rollback()
			errors <- fmt.Errorf("tx2 update row2 failed: %w", err)
			return
		}

		// Small delay to increase chance of deadlock
		time.Sleep(10 * time.Millisecond)

		// Try to update row 1 (might cause deadlock)
		_, err = tx.Exec("UPDATE deadlock_test SET value = 'tx2_row1' WHERE id = 1")
		if err != nil {
			tx.Rollback()
			errors <- fmt.Errorf("tx2 update row1 failed: %w", err)
			return
		}

		err = tx.Commit()
		if err != nil {
			errors <- fmt.Errorf("tx2 commit failed: %w", err)
			return
		}

		errors <- nil
	}()

	wg.Wait()

	// Check results - at least one transaction should succeed
	successCount := 0
	for i := 0; i < 2; i++ {
		err := <-errors
		if err == nil {
			successCount++
		} else {
			t.Logf("Transaction failed (expected in deadlock test): %v", err)
		}
	}

	assert.GreaterOrEqual(t, successCount, 1, "At least one transaction should succeed")
}

// TestTransactionIntegrationLockWaitTimeout tests lock wait timeout handling
func TestTransactionIntegrationLockWaitTimeout(t *testing.T) {
	db := createIntegrationDB(t)
	defer db.Close()

	// Create test table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS lock_test (
			id INT PRIMARY KEY,
			value VARCHAR(50)
		) ENGINE=InnoDB
	`)
	require.NoError(t, err)

	// Clean up and insert test data
	db.Exec("DELETE FROM lock_test")
	_, err = db.Exec("INSERT INTO lock_test (id, value) VALUES (1, 'original')")
	require.NoError(t, err)

	// Start a transaction and lock a row
	tx1, err := db.Begin()
	require.NoError(t, err)

	_, err = tx1.Exec("SELECT * FROM lock_test WHERE id = 1 FOR UPDATE")
	require.NoError(t, err)

	// Start another transaction and try to lock the same row with short timeout
	tx2, err := db.Begin()
	require.NoError(t, err)

	// Set short lock wait timeout
	_, err = tx2.Exec("SET SESSION innodb_lock_wait_timeout = 1")
	require.NoError(t, err)

	// Try to lock the same row - should timeout
	_, err = tx2.Exec("SELECT * FROM lock_test WHERE id = 1 FOR UPDATE")
	assert.Error(t, err, "Expected lock wait timeout error")
	assert.Contains(t, err.Error(), "lock wait timeout", "Error should mention lock wait timeout")

	// Clean up
	tx2.Rollback()
	tx1.Rollback()
}

// TestTransactionIntegrationLongRunning tests long-running transaction behavior
func TestTransactionIntegrationLongRunning(t *testing.T) {
	db := createIntegrationDB(t)
	defer db.Close()

	// Create test table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS long_running_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			data VARCHAR(100)
		) ENGINE=InnoDB
	`)
	require.NoError(t, err)

	tx, err := db.Begin()
	require.NoError(t, err)

	// Insert some data
	_, err = tx.Exec("INSERT INTO long_running_test (data) VALUES (?)", "start")
	require.NoError(t, err)

	// Simulate long-running transaction
	time.Sleep(2 * time.Second)

	// Insert more data
	_, err = tx.Exec("INSERT INTO long_running_test (data) VALUES (?)", "middle")
	require.NoError(t, err)

	// More work
	time.Sleep(1 * time.Second)

	_, err = tx.Exec("INSERT INTO long_running_test (data) VALUES (?)", "end")
	require.NoError(t, err)

	err = tx.Commit()
	require.NoError(t, err)

	// Verify all data was inserted
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM long_running_test").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 3, count, "All rows should be inserted")
}

// TestTransactionIntegrationRollbackOnError tests automatic rollback on errors
func TestTransactionIntegrationRollbackOnError(t *testing.T) {
	db := createIntegrationDB(t)
	defer db.Close()

	// Create test table with unique constraint
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS rollback_test (
			id INT PRIMARY KEY,
			value VARCHAR(50) UNIQUE
		) ENGINE=InnoDB
	`)
	require.NoError(t, err)

	// Clean up
	db.Exec("DELETE FROM rollback_test")

	tx, err := db.Begin()
	require.NoError(t, err)

	// Insert first row
	_, err = tx.Exec("INSERT INTO rollback_test (id, value) VALUES (1, 'unique_value')")
	require.NoError(t, err)

	// Try to insert duplicate value - should fail
	_, err = tx.Exec("INSERT INTO rollback_test (id, value) VALUES (2, 'unique_value')")
	assert.Error(t, err, "Expected duplicate key error")

	// Rollback explicitly
	err = tx.Rollback()
	require.NoError(t, err)

	// Verify no data was committed
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM rollback_test").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "No rows should be committed after rollback")
}

// TestTransactionIntegrationIsolationLevels tests isolation level behavior
func TestTransactionIntegrationIsolationLevels(t *testing.T) {
	db := createIntegrationDB(t)
	defer db.Close()

	// Create test table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS isolation_test (
			id INT PRIMARY KEY,
			value VARCHAR(50),
			counter INT DEFAULT 0
		) ENGINE=InnoDB
	`)
	require.NoError(t, err)

	// Clean up and insert initial data
	db.Exec("DELETE FROM isolation_test")
	_, err = db.Exec("INSERT INTO isolation_test (id, value, counter) VALUES (1, 'test', 0)")
	require.NoError(t, err)

	t.Run("READ_COMMITTED", func(t *testing.T) {
		// Set isolation level
		_, err := db.Exec("SET SESSION TRANSACTION ISOLATION LEVEL READ COMMITTED")
		require.NoError(t, err)

		// Start transaction 1
		tx1, err := db.Begin()
		require.NoError(t, err)

		// Read initial value
		var value string
		err = tx1.QueryRow("SELECT value FROM isolation_test WHERE id = 1").Scan(&value)
		require.NoError(t, err)
		assert.Equal(t, "test", value)

		// Start transaction 2 and update
		tx2, err := db.Begin()
		require.NoError(t, err)

		_, err = tx2.Exec("UPDATE isolation_test SET value = 'updated' WHERE id = 1")
		require.NoError(t, err)

		err = tx2.Commit()
		require.NoError(t, err)

		// Transaction 1 should still see old value in READ COMMITTED after commit
		err = tx1.QueryRow("SELECT value FROM isolation_test WHERE id = 1").Scan(&value)
		require.NoError(t, err)
		assert.Equal(t, "updated", value) // READ COMMITTED sees committed changes

		tx1.Rollback()
	})

	t.Run("REPEATABLE_READ", func(t *testing.T) {
		// Reset data
		_, err := db.Exec("UPDATE isolation_test SET value = 'test' WHERE id = 1")
		require.NoError(t, err)

		// Set isolation level
		_, err = db.Exec("SET SESSION TRANSACTION ISOLATION LEVEL REPEATABLE READ")
		require.NoError(t, err)

		// Start transaction 1
		tx1, err := db.Begin()
		require.NoError(t, err)

		// Read initial value
		var value string
		err = tx1.QueryRow("SELECT value FROM isolation_test WHERE id = 1").Scan(&value)
		require.NoError(t, err)
		assert.Equal(t, "test", value)

		// Start transaction 2 and update
		tx2, err := db.Begin()
		require.NoError(t, err)

		_, err = tx2.Exec("UPDATE isolation_test SET value = 'updated' WHERE id = 1")
		require.NoError(t, err)

		err = tx2.Commit()
		require.NoError(t, err)

		// Transaction 1 should still see old value in REPEATABLE READ
		err = tx1.QueryRow("SELECT value FROM isolation_test WHERE id = 1").Scan(&value)
		require.NoError(t, err)
		assert.Equal(t, "test", value) // REPEATABLE READ doesn't see committed changes

		tx1.Rollback()
	})
}

// TestTransactionIntegrationConnectionPool tests transaction behavior with connection pooling
func TestTransactionIntegrationConnectionPool(t *testing.T) {
	db := createIntegrationDB(t)
	defer db.Close()

	// Configure aggressive connection pooling
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(10 * time.Second)

	// Create test table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS pool_test (
			id INT PRIMARY KEY,
			connection_id VARCHAR(50),
			timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		) ENGINE=InnoDB
	`)
	require.NoError(t, err)

	const numTransactions = 20
	var wg sync.WaitGroup
	errors := make(chan error, numTransactions)

	for i := 0; i < numTransactions; i++ {
		wg.Add(1)
		go func(txID int) {
			defer wg.Done()

			tx, err := db.Begin()
			if err != nil {
				errors <- fmt.Errorf("transaction %d: begin failed: %w", txID, err)
				return
			}

			// Get connection ID
			var connID string
			err = tx.QueryRow("SELECT CONNECTION_ID()").Scan(&connID)
			if err != nil {
				tx.Rollback()
				errors <- fmt.Errorf("transaction %d: get connection ID failed: %w", txID, err)
				return
			}

			// Insert data
			_, err = tx.Exec("INSERT INTO pool_test (id, connection_id) VALUES (?, ?)", txID, connID)
			if err != nil {
				tx.Rollback()
				errors <- fmt.Errorf("transaction %d: insert failed: %w", txID, err)
				return
			}

			err = tx.Commit()
			if err != nil {
				errors <- fmt.Errorf("transaction %d: commit failed: %w", txID, err)
				return
			}

			errors <- nil
		}(i)
	}

	wg.Wait()

	// Check for errors
	for i := 0; i < numTransactions; i++ {
		err := <-errors
		assert.NoError(t, err, "All transactions should succeed")
	}

	// Verify all data was inserted
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM pool_test").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, numTransactions, count, "All transactions should commit successfully")

	// Check that multiple connection IDs were used (pooling worked)
	var uniqueConnections int
	err = db.QueryRow("SELECT COUNT(DISTINCT connection_id) FROM pool_test").Scan(&uniqueConnections)
	require.NoError(t, err)
	assert.Greater(t, uniqueConnections, 1, "Should use multiple connections from pool")
}

// TestTransactionIntegrationCancelByContext tests transaction cancellation via context
func TestTransactionIntegrationCancelByContext(t *testing.T) {
	db := createIntegrationDB(t)
	defer db.Close()

	// Create test table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS cancel_test (
			id INT PRIMARY KEY,
			value VARCHAR(50)
		) ENGINE=InnoDB
	`)
	require.NoError(t, err)

	// Create a context that can be cancelled
	ctx, cancel := context.WithCancel(context.Background())

	// Start transaction with context
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)

	// Cancel the context
	cancel()

	// Try to use the transaction - should fail
	_, err = tx.Exec("INSERT INTO cancel_test (id, value) VALUES (1, 'test')")
	assert.Error(t, err, "Expected error after context cancellation")

	// Try to commit - should fail
	err = tx.Commit()
	assert.Error(t, err, "Expected commit error after context cancellation")
}

// TestTransactionIntegrationRecoveryAfterError tests recovery after transaction errors
func TestTransactionIntegrationRecoveryAfterError(t *testing.T) {
	db := createIntegrationDB(t)
	defer db.Close()

	// Create test table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS recovery_test (
			id INT PRIMARY KEY,
			value VARCHAR(50) UNIQUE
		) ENGINE=InnoDB
	`)
	require.NoError(t, err)

	// Clean up
	db.Exec("DELETE FROM recovery_test")

	// First transaction - will fail due to duplicate key
	tx1, err := db.Begin()
	require.NoError(t, err)

	_, err = tx1.Exec("INSERT INTO recovery_test (id, value) VALUES (1, 'unique')")
	require.NoError(t, err)

	_, err = tx1.Exec("INSERT INTO recovery_test (id, value) VALUES (2, 'unique')")
	assert.Error(t, err, "Expected duplicate key error")

	err = tx1.Rollback()
	require.NoError(t, err)

	// Second transaction - should succeed after the error
	tx2, err := db.Begin()
	require.NoError(t, err)

	_, err = tx2.Exec("INSERT INTO recovery_test (id, value) VALUES (1, 'unique1')")
	require.NoError(t, err)

	_, err = tx2.Exec("INSERT INTO recovery_test (id, value) VALUES (2, 'unique2')")
	require.NoError(t, err)

	err = tx2.Commit()
	require.NoError(t, err)

	// Verify data was inserted
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM recovery_test").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 2, count, "Second transaction should succeed after first transaction error")
}

// Helper function to create integration test database connection
func createIntegrationDB(t *testing.T) *sql.DB {
	// Use environment variable for integration test DSN
	dsn := "root:password@tcp(localhost:3306)/mysql_test?parseTime=true&timeout=30s&readTimeout=30s&writeTimeout=30s"
	
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Skipf("Failed to connect to integration test database: %v", err)
	}

	// Test the connection with retry logic
	for i := 0; i < 5; i++ {
		err = db.Ping()
		if err == nil {
			break
		}
		if i == 4 {
			db.Close()
			t.Skipf("Failed to ping integration test database after 5 attempts: %v", err)
		}
		time.Sleep(time.Second)
	}

	// Configure connection pool for integration tests
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(3)
	db.SetConnMaxLifetime(5 * time.Minute)

	return db
}
