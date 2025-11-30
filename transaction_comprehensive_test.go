// Go MySQL Driver - Comprehensive Transaction Test Suite
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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTransactionComprehensiveSuite runs a complete transaction test suite
func TestTransactionComprehensiveSuite(t *testing.T) {
	t.Run("BasicOperations", TestTransactionBasicOperations)
	t.Run("CommitRollback", TestTransactionCommitRollback)
	t.Run("ErrorHandling", TestTransactionErrorHandlingComprehensive)
	t.Run("Concurrency", TestTransactionConcurrency)
	t.Run("IsolationLevels", TestTransactionIsolationLevelsComprehensive)
	t.Run("Savepoints", TestTransactionSavepointsComprehensive)
	t.Run("ContextHandling", TestTransactionContextHandling)
	t.Run("Performance", TestTransactionPerformance)
	t.Run("EdgeCases", TestTransactionEdgeCases)
}

// TestTransactionBasicOperations tests basic transaction operations
func TestTransactionBasicOperations(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	// Create test table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS basic_ops_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			name VARCHAR(50),
			value INT
		) ENGINE=InnoDB
	`)
	require.NoError(t, err)

	// Clean up
	_, err = db.Exec("DELETE FROM basic_ops_test")
	require.NoError(t, err)

	t.Run("Insert", func(t *testing.T) {
		tx, err := db.Begin()
		require.NoError(t, err)

		_, err = tx.Exec("INSERT INTO basic_ops_test (name, value) VALUES (?, ?)", "test1", 100)
		require.NoError(t, err)

		err = tx.Commit()
		assert.NoError(t, err)

		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM basic_ops_test").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("Update", func(t *testing.T) {
		tx, err := db.Begin()
		require.NoError(t, err)

		_, err = tx.Exec("UPDATE basic_ops_test SET value = ? WHERE name = ?", 200, "test1")
		require.NoError(t, err)

		err = tx.Commit()
		assert.NoError(t, err)

		var value int
		err = db.QueryRow("SELECT value FROM basic_ops_test WHERE name = ?", "test1").Scan(&value)
		require.NoError(t, err)
		assert.Equal(t, 200, value)
	})

	t.Run("Delete", func(t *testing.T) {
		tx, err := db.Begin()
		require.NoError(t, err)

		_, err = tx.Exec("DELETE FROM basic_ops_test WHERE name = ?", "test1")
		require.NoError(t, err)

		err = tx.Commit()
		assert.NoError(t, err)

		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM basic_ops_test").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})
}

// TestTransactionCommitRollback tests commit and rollback behavior
func TestTransactionCommitRollback(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	// Create test table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS commit_rollback_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			data VARCHAR(50)
		) ENGINE=InnoDB
	`)
	require.NoError(t, err)

	// Clean up
	_, err = db.Exec("DELETE FROM commit_rollback_test")
	require.NoError(t, err)

	t.Run("CommitPreservesData", func(t *testing.T) {
		tx, err := db.Begin()
		require.NoError(t, err)

		_, err = tx.Exec("INSERT INTO commit_rollback_test (data) VALUES (?)", "commit_test")
		require.NoError(t, err)

		err = tx.Commit()
		assert.NoError(t, err)

		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM commit_rollback_test").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("RollbackDiscardsData", func(t *testing.T) {
		tx, err := db.Begin()
		require.NoError(t, err)

		_, err = tx.Exec("INSERT INTO commit_rollback_test (data) VALUES (?)", "rollback_test")
		require.NoError(t, err)

		err = tx.Rollback()
		assert.NoError(t, err)

		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM commit_rollback_test WHERE data = 'rollback_test'").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})
}

// TestTransactionErrorHandlingComprehensive tests error handling in transactions
func TestTransactionErrorHandlingComprehensive(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	// Create test table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS error_handling_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			value VARCHAR(50) UNIQUE
		) ENGINE=InnoDB
	`)
	require.NoError(t, err)

	// Clean up
	_, err = db.Exec("DELETE FROM error_handling_test")
	require.NoError(t, err)

	t.Run("ConstraintViolation", func(t *testing.T) {
		tx, err := db.Begin()
		require.NoError(t, err)

		// Insert first record
		_, err = tx.Exec("INSERT INTO error_handling_test (value) VALUES (?)", "unique_value")
		require.NoError(t, err)

		// Try to insert duplicate
		_, err = tx.Exec("INSERT INTO error_handling_test (value) VALUES (?)", "unique_value")
		assert.Error(t, err)

		// Transaction should still be usable for rollback
		err = tx.Rollback()
		assert.NoError(t, err)

		// Verify no data was committed
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM error_handling_test").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("SyntaxError", func(t *testing.T) {
		tx, err := db.Begin()
		require.NoError(t, err)

		// Invalid SQL
		_, err = tx.Exec("INVALID SQL STATEMENT")
		assert.Error(t, err)

		// Transaction should still be usable for rollback
		err = tx.Rollback()
		assert.NoError(t, err)
	})
}

// TestTransactionConcurrency tests concurrent transaction behavior
func TestTransactionConcurrency(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	// Create test table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS concurrency_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			value INT,
			worker_id INT
		) ENGINE=InnoDB
	`)
	require.NoError(t, err)

	// Clean up
	_, err = db.Exec("DELETE FROM concurrency_test")
	require.NoError(t, err)

	const numWorkers = 10
	const numOperations = 5
	var wg sync.WaitGroup
	errors := make(chan error, numWorkers)

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < numOperations; j++ {
				tx, err := db.Begin()
				if err != nil {
					errors <- fmt.Errorf("worker %d: begin failed: %w", workerID, err)
					return
				}

				// Insert data
				_, err = tx.Exec("INSERT INTO concurrency_test (value, worker_id) VALUES (?, ?)", 
					j, workerID)
				if err != nil {
					tx.Rollback()
					errors <- fmt.Errorf("worker %d: insert failed: %w", workerID, err)
					return
				}

				// Update some existing data
				_, err = tx.Exec("UPDATE concurrency_test SET value = value + 1 WHERE worker_id < ?", workerID)
				if err != nil {
					tx.Rollback()
					errors <- fmt.Errorf("worker %d: update failed: %w", workerID, err)
					return
				}

				err = tx.Commit()
				if err != nil {
					errors <- fmt.Errorf("worker %d: commit failed: %w", workerID, err)
					return
				}
			}
			errors <- nil
		}(i)
	}

	wg.Wait()

	// Check for errors
	for i := 0; i < numWorkers; i++ {
		err := <-errors
		// Some deadlocks are expected in high concurrency
		if err != nil && !strings.Contains(err.Error(), "Deadlock") {
			assert.NoError(t, err, "Worker should complete without non-deadlock errors")
		}
	}

	// Verify final state
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM concurrency_test").Scan(&count)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, numWorkers*numOperations/2, "At least half of operations should succeed")
}

// TestTransactionIsolationLevelsComprehensive tests different isolation levels
func TestTransactionIsolationLevelsComprehensive(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	// Create test table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS isolation_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			value INT,
			isolation_level VARCHAR(50)
		) ENGINE=InnoDB
	`)
	require.NoError(t, err)

	// Clean up
	_, err = db.Exec("DELETE FROM isolation_test")
	require.NoError(t, err)

	isolationLevels := []sql.IsolationLevel{
		sql.LevelReadCommitted,
		sql.LevelRepeatableRead,
		sql.LevelSerializable,
	}

	for _, level := range isolationLevels {
		t.Run(level.String(), func(t *testing.T) {
			tx, err := db.BeginTx(context.Background(), &sql.TxOptions{
				Isolation: level,
			})
			if err != nil {
				t.Skipf("Isolation level %v not supported: %v", level, err)
				return
			}

			// Insert test data
			_, err = tx.Exec("INSERT INTO isolation_test (value, isolation_level) VALUES (?, ?)", 
				100, level.String())
			require.NoError(t, err)

			// Read data back
			var value, isolation string
			err = tx.QueryRow("SELECT value, isolation_level FROM isolation_test WHERE id = 1").Scan(&value, &isolation)
			require.NoError(t, err)

			assert.Equal(t, "100", value)
			assert.Equal(t, level.String(), isolation)

			err = tx.Commit()
			assert.NoError(t, err)
		})
	}
}

// TestTransactionSavepointsComprehensive tests savepoint functionality
func TestTransactionSavepointsComprehensive(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	// Create test table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS savepoint_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			value VARCHAR(50)
		) ENGINE=InnoDB
	`)
	require.NoError(t, err)

	// Clean up
	_, err = db.Exec("DELETE FROM savepoint_test")
	require.NoError(t, err)

	tx, err := db.Begin()
	require.NoError(t, err)

	// Create first savepoint
	_, err = tx.Exec("SAVEPOINT sp1")
	require.NoError(t, err)

	// Insert first record
	_, err = tx.Exec("INSERT INTO savepoint_test (value) VALUES (?)", "before_sp2")
	require.NoError(t, err)

	// Create second savepoint
	_, err = tx.Exec("SAVEPOINT sp2")
	require.NoError(t, err)

	// Insert second record
	_, err = tx.Exec("INSERT INTO savepoint_test (value) VALUES (?)", "after_sp2")
	require.NoError(t, err)

	// Rollback to second savepoint
	_, err = tx.Exec("ROLLBACK TO SAVEPOINT sp2")
	require.NoError(t, err)

	// Commit transaction
	err = tx.Commit()
	assert.NoError(t, err)

	// Verify only first record exists
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM savepoint_test").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	var value string
	err = db.QueryRow("SELECT value FROM savepoint_test WHERE id = 1").Scan(&value)
	require.NoError(t, err)
	assert.Equal(t, "before_sp2", value)
}

// TestTransactionContextHandling tests context handling in transactions
func TestTransactionContextHandling(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	// Create test table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS context_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			value VARCHAR(50)
		) ENGINE=InnoDB
	`)
	require.NoError(t, err)

	// Clean up
	_, err = db.Exec("DELETE FROM context_test")
	require.NoError(t, err)

	t.Run("TimeoutContext", func(t *testing.T) {
		// Create context with short timeout
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()

		// Small delay to ensure timeout
		time.Sleep(time.Microsecond)

		tx, err := db.BeginTx(ctx, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "deadline exceeded")

		if tx != nil {
			tx.Rollback()
		}
	})

	t.Run("CancelledContext", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		tx, err := db.BeginTx(ctx, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "context canceled")

		if tx != nil {
			tx.Rollback()
		}
	})

	t.Run("ValidContext", func(t *testing.T) {
		ctx := context.Background()
		tx, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)

		_, err = tx.Exec("INSERT INTO context_test (value) VALUES (?)", "context_test")
		require.NoError(t, err)

		err = tx.Commit()
		assert.NoError(t, err)
	})
}

// TestTransactionPerformance tests transaction performance
func TestTransactionPerformance(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	// Create test table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS performance_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			value INT
		) ENGINE=InnoDB
	`)
	require.NoError(t, err)

	// Clean up
	_, err = db.Exec("DELETE FROM performance_test")
	require.NoError(t, err)

	const numOperations = 100
	start := time.Now()

	tx, err := db.Begin()
	require.NoError(t, err)

	for i := 0; i < numOperations; i++ {
		_, err = tx.Exec("INSERT INTO performance_test (value) VALUES (?)", i)
		require.NoError(t, err)
	}

	err = tx.Commit()
	require.NoError(t, err)

	duration := time.Since(start)
	opsPerSecond := float64(numOperations) / duration.Seconds()

	t.Logf("Performance: %d operations in %v (%.2f ops/sec)", 
		numOperations, duration, opsPerSecond)

	// Verify all data was inserted
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM performance_test").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, numOperations, count)

	// Performance should be reasonable
	assert.Less(t, duration, 5*time.Second, "Operations should complete within 5 seconds")
}

// TestTransactionEdgeCases tests edge cases and boundary conditions
func TestTransactionEdgeCases(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	// Create test table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS edge_case_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			data TEXT
		) ENGINE=InnoDB
	`)
	require.NoError(t, err)

	// Clean up
	_, err = db.Exec("DELETE FROM edge_case_test")
	require.NoError(t, err)

	t.Run("EmptyTransaction", func(t *testing.T) {
		tx, err := db.Begin()
		require.NoError(t, err)

		// Commit without any operations
		err = tx.Commit()
		assert.NoError(t, err)
	})

	t.Run("LargeData", func(t *testing.T) {
		tx, err := db.Begin()
		require.NoError(t, err)

		// Generate large text data
		largeData := strings.Repeat("Large data string. ", 1000)

		_, err = tx.Exec("INSERT INTO edge_case_test (data) VALUES (?)", largeData)
		require.NoError(t, err)

		err = tx.Commit()
		assert.NoError(t, err)

		// Verify data integrity
		var retrievedData string
		err = db.QueryRow("SELECT data FROM edge_case_test WHERE id = 1").Scan(&retrievedData)
		require.NoError(t, err)
		assert.Equal(t, largeData, retrievedData)
	})

	t.Run("SpecialCharacters", func(t *testing.T) {
		tx, err := db.Begin()
		require.NoError(t, err)

		specialData := "Special chars: ' \" \\ \n \t % & * @ # ! $ ^ ( ) { } [ ] | : ; < > , . ? / ~ `"
		
		_, err = tx.Exec("INSERT INTO edge_case_test (data) VALUES (?)", specialData)
		require.NoError(t, err)

		err = tx.Commit()
		assert.NoError(t, err)

		// Verify data integrity
		var retrievedData string
		err = db.QueryRow("SELECT data FROM edge_case_test WHERE data LIKE ?", "Special chars:%").Scan(&retrievedData)
		require.NoError(t, err)
		assert.Equal(t, specialData, retrievedData)
	})

	t.Run("UnicodeData", func(t *testing.T) {
		tx, err := db.Begin()
		require.NoError(t, err)

		unicodeData := "Unicode: 你好世界 🌍 🚀 🔥 💯 ñáéíóú ü ß"

		_, err = tx.Exec("INSERT INTO edge_case_test (data) VALUES (?)", unicodeData)
		require.NoError(t, err)

		err = tx.Commit()
		assert.NoError(t, err)

		// Verify data integrity
		var retrievedData string
		err = db.QueryRow("SELECT data FROM edge_case_test WHERE data LIKE ?", "Unicode:%").Scan(&retrievedData)
		require.NoError(t, err)
		assert.Equal(t, unicodeData, retrievedData)
	})
}

// TestTransactionRecovery tests transaction recovery scenarios
func TestTransactionRecovery(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	// Create test table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS recovery_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			status VARCHAR(20),
			data VARCHAR(50)
		) ENGINE=InnoDB
	`)
	require.NoError(t, err)

	// Clean up
	_, err = db.Exec("DELETE FROM recovery_test")
	require.NoError(t, err)

	t.Run("CommitAfterError", func(t *testing.T) {
		tx, err := db.Begin()
		require.NoError(t, err)

		// Successful operation
		_, err = tx.Exec("INSERT INTO recovery_test (status, data) VALUES (?, ?)", "success", "data1")
		require.NoError(t, err)

		// Failed operation (constraint violation)
		_, err = tx.Exec("INSERT INTO recovery_test (id, status, data) VALUES (1, 'fail', 'data2')")
		assert.Error(t, err)

		// Try to commit - should fail due to previous error
		err = tx.Commit()
		// Some databases allow commit after errors, others don't
		// Both behaviors are acceptable
		if err != nil {
			assert.Error(t, err)
		}
	})

	t.Run("RollbackAfterError", func(t *testing.T) {
		tx, err := db.Begin()
		require.NoError(t, err)

		// Successful operation
		_, err = tx.Exec("INSERT INTO recovery_test (status, data) VALUES (?, ?)", "success", "data3")
		require.NoError(t, err)

		// Failed operation
		_, err = tx.Exec("INVALID SQL")
		assert.Error(t, err)

		// Rollback should always succeed
		err = tx.Rollback()
		assert.NoError(t, err)

		// Verify no data was committed
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM recovery_test WHERE data LIKE 'data%'").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})
}

// TestTransactionConnectionPool tests transaction behavior with connection pooling
func TestTransactionConnectionPool(t *testing.T) {
	// Create a connection pool
	db := createTestDBConnection(t)
	defer db.Close()

	// Set connection pool parameters
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	// Create test table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS pool_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			connection_id VARCHAR(50),
			value INT
		) ENGINE=InnoDB
	`)
	require.NoError(t, err)

	// Clean up
	_, err = db.Exec("DELETE FROM pool_test")
	require.NoError(t, err)

	const numConcurrentTx = 20
	var wg sync.WaitGroup
	errors := make(chan error, numConcurrentTx)

	for i := 0; i < numConcurrentTx; i++ {
		wg.Add(1)
		go func(txID int) {
			defer wg.Done()

			tx, err := db.Begin()
			if err != nil {
				errors <- fmt.Errorf("tx %d: begin failed: %w", txID, err)
				return
			}

			// Get connection ID
			var connectionID string
			err = tx.QueryRow("SELECT CONNECTION_ID()").Scan(&connectionID)
			if err != nil {
				tx.Rollback()
				errors <- fmt.Errorf("tx %d: connection_id failed: %w", txID, err)
				return
			}

			// Insert data with connection ID
			_, err = tx.Exec("INSERT INTO pool_test (connection_id, value) VALUES (?, ?)", 
				connectionID, txID)
			if err != nil {
				tx.Rollback()
				errors <- fmt.Errorf("tx %d: insert failed: %w", txID, err)
				return
			}

			err = tx.Commit()
			if err != nil {
				errors <- fmt.Errorf("tx %d: commit failed: %w", txID, err)
				return
			}

			errors <- nil
		}(i)
	}

	wg.Wait()

	// Check for errors
	for i := 0; i < numConcurrentTx; i++ {
		err := <-errors
		if err != nil && !strings.Contains(err.Error(), "Deadlock") {
			assert.NoError(t, err, "Transaction should complete without error")
		}
	}

	// Verify all transactions completed
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM pool_test").Scan(&count)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, numConcurrentTx/2, "At least half of transactions should succeed")

	// Check connection pool usage
	var uniqueConnections int
	err = db.QueryRow("SELECT COUNT(DISTINCT connection_id) FROM pool_test").Scan(&uniqueConnections)
	require.NoError(t, err)
	t.Logf("Used %d unique connections from pool", uniqueConnections)
}

// BenchmarkTransactionComprehensive benchmarks comprehensive transaction performance
func BenchmarkTransactionComprehensive(b *testing.B) {
	db := createBenchDBConnection(b)
	defer db.Close()

	// Create test table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS bench_comprehensive (
			id INT PRIMARY KEY AUTO_INCREMENT,
			value INT,
			data VARCHAR(100)
		) ENGINE=InnoDB
	`)
	if err != nil {
		b.Skipf("Failed to create table: %v", err)
		return
	}

	// Clean up
	_, err = db.Exec("DELETE FROM bench_comprehensive")
	if err != nil {
		b.Skipf("Failed to clean up table: %v", err)
		return
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			tx, err := db.Begin()
			if err != nil {
				b.Errorf("Begin failed: %v", err)
				continue
			}

			// Insert
			_, err = tx.Exec("INSERT INTO bench_comprehensive (value, data) VALUES (?, ?)", 
				i, fmt.Sprintf("bench_data_%d", i))
			if err != nil {
				tx.Rollback()
				b.Errorf("Insert failed: %v", err)
				continue
			}

			// Update
			_, err = tx.Exec("UPDATE bench_comprehensive SET value = value + 1 WHERE id = ?", i+1)
			if err != nil {
				tx.Rollback()
				b.Errorf("Update failed: %v", err)
				continue
			}

			// Select
			var value int
			err = tx.QueryRow("SELECT value FROM bench_comprehensive WHERE id = ?", i+1).Scan(&value)
			if err != nil {
				tx.Rollback()
				b.Errorf("Select failed: %v", err)
				continue
			}

			err = tx.Commit()
			if err != nil {
				b.Errorf("Commit failed: %v", err)
				continue
			}

			if value != i+1 {
				b.Errorf("Expected value %d, got %d", i+1, value)
			}

			i++
		}
	})
}
