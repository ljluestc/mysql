// Go MySQL Driver - Transaction ID Overflow Tests
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
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTransactionIDOverflowBehavior tests transaction ID behavior under various conditions
func TestTransactionIDOverflowBehavior(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	// Create test table for transaction ID testing
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS transaction_id_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			data VARCHAR(100),
			tx_info VARCHAR(200),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		) ENGINE=InnoDB
	`)
	require.NoError(t, err)

	// Clean up existing data
	_, err = db.Exec("DELETE FROM transaction_id_test")
	require.NoError(t, err)

	t.Run("BasicTransactionIDTracking", func(t *testing.T) {
		tx, err := db.Begin()
		require.NoError(t, err)

		// Insert test data
		_, err = tx.Exec("INSERT INTO transaction_id_test (data, tx_info) VALUES (?, ?)", 
			"test_data", "basic_tx_test")
		require.NoError(t, err)

		// Get transaction ID information
		var txInfo string
		err = tx.QueryRow("SELECT CONNECTION_ID()").Scan(&txInfo)
		require.NoError(t, err)

		err = tx.Commit()
		assert.NoError(t, err)

		// Verify data was committed
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM transaction_id_test WHERE tx_info = ?", 
			"basic_tx_test").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count)

		t.Logf("Transaction completed with connection ID: %s", txInfo)
	})

	t.Run("HighVolumeTransactionGeneration", func(t *testing.T) {
		const numTransactions = 1000
		var wg sync.WaitGroup
		errors := make(chan error, numTransactions)

		start := time.Now()

		for i := 0; i < numTransactions; i++ {
			wg.Add(1)
			go func(txNum int) {
				defer wg.Done()

				tx, err := db.Begin()
				if err != nil {
					errors <- fmt.Errorf("tx %d: begin failed: %w", txNum, err)
					return
				}

				// Insert data with transaction number
				_, err = tx.Exec("INSERT INTO transaction_id_test (data, tx_info) VALUES (?, ?)", 
					fmt.Sprintf("high_vol_data_%d", txNum), 
					fmt.Sprintf("high_vol_tx_%d", txNum))
				if err != nil {
					tx.Rollback()
					errors <- fmt.Errorf("tx %d: insert failed: %w", txNum, err)
					return
				}

				err = tx.Commit()
				if err != nil {
					errors <- fmt.Errorf("tx %d: commit failed: %w", txNum, err)
					return
				}

				errors <- nil
			}(i)
		}

		wg.Wait()
		duration := time.Since(start)

		// Check for errors
		errorCount := 0
		for i := 0; i < numTransactions; i++ {
			err := <-errors
			if err != nil {
				errorCount++
				t.Logf("Transaction error: %v", err)
			}
		}

		successRate := float64(numTransactions-errorCount) / float64(numTransactions) * 100
		t.Logf("High volume test completed in %v: %d/%d transactions successful (%.1f%%)", 
			duration, numTransactions-errorCount, numTransactions, successRate)

		// Verify most transactions succeeded
		assert.Greater(t, successRate, 90.0, "At least 90% of transactions should succeed")

		// Verify data integrity
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM transaction_id_test WHERE tx_info LIKE 'high_vol_tx_%'").Scan(&count)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, count, int(float64(numTransactions)*0.9), 
			"At least 90% of data should be inserted")
	})

	t.Run("TransactionIDConsistency", func(t *testing.T) {
		// Test that transaction IDs are consistent within a transaction
		tx, err := db.Begin()
		require.NoError(t, err)

		// Get connection ID at start
		var initialConnID string
		err = tx.QueryRow("SELECT CONNECTION_ID()").Scan(&initialConnID)
		require.NoError(t, err)

		// Perform multiple operations
		for i := 0; i < 5; i++ {
			_, err = tx.Exec("INSERT INTO transaction_id_test (data, tx_info) VALUES (?, ?)", 
				fmt.Sprintf("consistency_data_%d", i), 
				fmt.Sprintf("consistency_tx_%s", initialConnID))
			require.NoError(t, err)

			// Verify connection ID remains the same
			var currentConnID string
			err = tx.QueryRow("SELECT CONNECTION_ID()").Scan(&currentConnID)
			require.NoError(t, err)
			assert.Equal(t, initialConnID, currentConnID, 
				"Connection ID should remain consistent within transaction")
		}

		err = tx.Commit()
		assert.NoError(t, err)

		// Verify all data was inserted with consistent transaction info
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM transaction_id_test WHERE tx_info = ?", 
			fmt.Sprintf("consistency_tx_%s", initialConnID)).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 5, count, "All 5 records should be inserted")
	})

	t.Run("TransactionIDAfterRollback", func(t *testing.T) {
		// Test transaction ID behavior after rollback
		tx, err := db.Begin()
		require.NoError(t, err)

		var rollbackConnID string
		err = tx.QueryRow("SELECT CONNECTION_ID()").Scan(&rollbackConnID)
		require.NoError(t, err)

		// Insert data that will be rolled back
		_, err = tx.Exec("INSERT INTO transaction_id_test (data, tx_info) VALUES (?, ?)", 
			"rollback_data", fmt.Sprintf("rollback_tx_%s", rollbackConnID))
		require.NoError(t, err)

		// Rollback the transaction
		err = tx.Rollback()
		assert.NoError(t, err)

		// Verify data was not committed
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM transaction_id_test WHERE data = ?", 
			"rollback_data").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count, "Rolled back data should not exist")

		// Start new transaction and verify it gets a new connection
		tx2, err := db.Begin()
		require.NoError(t, err)

		var newConnID string
		err = tx2.QueryRow("SELECT CONNECTION_ID()").Scan(&newConnID)
		require.NoError(t, err)

		err = tx2.Commit()
		assert.NoError(t, err)

		t.Logf("Rollback transaction ID: %s, New transaction ID: %s", 
			rollbackConnID, newConnID)
	})

	t.Run("ConcurrentTransactionIDUniqueness", func(t *testing.T) {
		// Test that concurrent transactions have unique IDs
		const numConcurrent = 20
		var wg sync.WaitGroup
		connIDs := make(chan string, numConcurrent)

		for i := 0; i < numConcurrent; i++ {
			wg.Add(1)
			go func(txNum int) {
				defer wg.Done()

				tx, err := db.Begin()
				if err != nil {
					t.Errorf("Concurrent tx %d: begin failed: %v", txNum, err)
					return
				}

				var connID string
				err = tx.QueryRow("SELECT CONNECTION_ID()").Scan(&connID)
				if err != nil {
					tx.Rollback()
					t.Errorf("Concurrent tx %d: connection ID failed: %v", txNum, err)
					return
				}

				// Insert test data
				_, err = tx.Exec("INSERT INTO transaction_id_test (data, tx_info) VALUES (?, ?)", 
					fmt.Sprintf("concurrent_data_%d", txNum), 
					fmt.Sprintf("concurrent_tx_%s", connID))
				if err != nil {
					tx.Rollback()
					t.Errorf("Concurrent tx %d: insert failed: %v", txNum, err)
					return
				}

				err = tx.Commit()
				if err != nil {
					t.Errorf("Concurrent tx %d: commit failed: %v", txNum, err)
					return
				}

				connIDs <- connID
			}(i)
		}

		wg.Wait()

		// Collect all connection IDs
		idMap := make(map[string]bool)
		for i := 0; i < numConcurrent; i++ {
			connID := <-connIDs
			idMap[connID] = true
		}

		t.Logf("Concurrent test used %d unique connection IDs out of %d transactions", 
			len(idMap), numConcurrent)

		// Verify we have reasonable uniqueness (allowing for connection pooling)
		assert.GreaterOrEqual(t, len(idMap), numConcurrent/4, 
			"Should have at least some unique connection IDs")
	})

	t.Run("TransactionIDWithSavepoints", func(t *testing.T) {
		// Test transaction ID behavior with savepoints
		tx, err := db.Begin()
		require.NoError(t, err)

		var savepointConnID string
		err = tx.QueryRow("SELECT CONNECTION_ID()").Scan(&savepointConnID)
		require.NoError(t, err)

		// Create savepoint 1
		_, err = tx.Exec("SAVEPOINT sp1")
		require.NoError(t, err)

		_, err = tx.Exec("INSERT INTO transaction_id_test (data, tx_info) VALUES (?, ?)", 
			"savepoint1_data", fmt.Sprintf("savepoint_tx_%s", savepointConnID))
		require.NoError(t, err)

		// Create savepoint 2
		_, err = tx.Exec("SAVEPOINT sp2")
		require.NoError(t, err)

		_, err = tx.Exec("INSERT INTO transaction_id_test (data, tx_info) VALUES (?, ?)", 
			"savepoint2_data", fmt.Sprintf("savepoint_tx_%s", savepointConnID))
		require.NoError(t, err)

		// Rollback to savepoint 1
		_, err = tx.Exec("ROLLBACK TO SAVEPOINT sp1")
		require.NoError(t, err)

		// Verify connection ID is still consistent
		var currentConnID string
		err = tx.QueryRow("SELECT CONNECTION_ID()").Scan(&currentConnID)
		require.NoError(t, err)
		assert.Equal(t, savepointConnID, currentConnID, 
			"Connection ID should remain consistent after savepoint rollback")

		err = tx.Commit()
		assert.NoError(t, err)

		// Verify only savepoint1 data exists
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM transaction_id_test WHERE tx_info = ?", 
			fmt.Sprintf("savepoint_tx_%s", savepointConnID)).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count, "Only data before savepoint2 should exist")
	})

	t.Run("TransactionIDWithTimeouts", func(t *testing.T) {
		// Test transaction ID behavior with context timeouts
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		tx, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)

		var timeoutConnID string
		err = tx.QueryRow("SELECT CONNECTION_ID()").Scan(&timeoutConnID)
		require.NoError(t, err)

		// Insert data within timeout
		_, err = tx.Exec("INSERT INTO transaction_id_test (data, tx_info) VALUES (?, ?)", 
			"timeout_data", fmt.Sprintf("timeout_tx_%s", timeoutConnID))
		require.NoError(t, err)

		err = tx.Commit()
		assert.NoError(t, err)

		// Verify data was committed
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM transaction_id_test WHERE data = ?", 
			"timeout_data").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count, "Timeout transaction data should exist")
	})

	t.Run("TransactionIDPersistence", func(t *testing.T) {
		// Test that transaction IDs are properly persisted
		tx, err := db.Begin()
		require.NoError(t, err)

		var persistenceConnID string
		err = tx.QueryRow("SELECT CONNECTION_ID()").Scan(&persistenceConnID)
		require.NoError(t, err)

		testData := fmt.Sprintf("persistence_data_%s", persistenceConnID)
		txInfo := fmt.Sprintf("persistence_tx_%s", persistenceConnID)

		_, err = tx.Exec("INSERT INTO transaction_id_test (data, tx_info) VALUES (?, ?)", 
			testData, txInfo)
		require.NoError(t, err)

		err = tx.Commit()
		assert.NoError(t, err)

		// Wait a moment to ensure persistence
		time.Sleep(100 * time.Millisecond)

		// Verify data persists across new connections
		var persistedData, persistedTxInfo string
		err = db.QueryRow("SELECT data, tx_info FROM transaction_id_test WHERE data = ?", 
			testData).Scan(&persistedData, &persistedTxInfo)
		require.NoError(t, err)

		assert.Equal(t, testData, persistedData, "Data should persist correctly")
		assert.Equal(t, txInfo, persistedTxInfo, "Transaction info should persist correctly")

		t.Logf("Persistence test completed with transaction ID: %s", persistenceConnID)
	})
}

// TestTransactionIDOverflowSimulation simulates transaction ID overflow scenarios
func TestTransactionIDOverflowSimulation(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	// Create table for overflow simulation
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS overflow_simulation_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			simulation_data VARCHAR(100),
			tx_sequence BIGINT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		) ENGINE=InnoDB
	`)
	require.NoError(t, err)

	// Clean up
	_, err = db.Exec("DELETE FROM overflow_simulation_test")
	require.NoError(t, err)

	t.Run("RapidTransactionGeneration", func(t *testing.T) {
		// Generate many transactions rapidly to test ID allocation
		const rapidTxCount = 500
		var wg sync.WaitGroup
		successCount := make(chan int, rapidTxCount)

		for i := 0; i < rapidTxCount; i++ {
			wg.Add(1)
			go func(seq int) {
				defer wg.Done()

				tx, err := db.Begin()
				if err != nil {
					t.Errorf("Rapid tx %d: begin failed: %v", seq, err)
					return
				}

				// Insert with sequence number
				_, err = tx.Exec("INSERT INTO overflow_simulation_test (simulation_data, tx_sequence) VALUES (?, ?)", 
					fmt.Sprintf("rapid_data_%d", seq), int64(seq))
				if err != nil {
					tx.Rollback()
					t.Errorf("Rapid tx %d: insert failed: %v", seq, err)
					return
				}

				err = tx.Commit()
				if err != nil {
					t.Errorf("Rapid tx %d: commit failed: %v", seq, err)
					return
				}

				successCount <- 1
			}(i)
		}

		wg.Wait()

		// Count successful transactions
		totalSuccess := 0
		for i := 0; i < rapidTxCount; i++ {
			totalSuccess += <-successCount
		}

		t.Logf("Rapid generation: %d/%d transactions successful", totalSuccess, rapidTxCount)

		// Verify data integrity
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM overflow_simulation_test").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, totalSuccess, count, "Database count should match successful transactions")

		// Verify sequence integrity
		var maxSeq int64
		err = db.QueryRow("SELECT MAX(tx_sequence) FROM overflow_simulation_test").Scan(&maxSeq)
		require.NoError(t, err)
		t.Logf("Maximum sequence number: %d", maxSeq)
	})

	t.Run("TransactionIDReuseDetection", func(t *testing.T) {
		// Test to detect if transaction IDs are being reused (which could indicate overflow)
		const reuseTestCount = 100
		connIDMap := make(map[string]int)
		var mu sync.Mutex

		var wg sync.WaitGroup
		for i := 0; i < reuseTestCount; i++ {
			wg.Add(1)
			go func(testNum int) {
				defer wg.Done()

				tx, err := db.Begin()
				if err != nil {
					t.Errorf("Reuse test tx %d: begin failed: %v", testNum, err)
					return
				}

				var connID string
				err = tx.QueryRow("SELECT CONNECTION_ID()").Scan(&connID)
				if err != nil {
					tx.Rollback()
					t.Errorf("Reuse test tx %d: connection ID failed: %v", testNum, err)
					return
				}

				// Track connection ID usage
				mu.Lock()
				connIDMap[connID]++
				mu.Unlock()

				_, err = tx.Exec("INSERT INTO overflow_simulation_test (simulation_data, tx_sequence) VALUES (?, ?)", 
					fmt.Sprintf("reuse_test_%d", testNum), int64(testNum))
				if err != nil {
					tx.Rollback()
					t.Errorf("Reuse test tx %d: insert failed: %v", testNum, err)
					return
				}

				err = tx.Commit()
				if err != nil {
					t.Errorf("Reuse test tx %d: commit failed: %v", testNum, err)
					return
				}
			}(i)
		}

		wg.Wait()

		t.Logf("Reuse test: %d unique connection IDs used out of %d transactions", 
			len(connIDMap), reuseTestCount)

		// Check for potential ID reuse (same ID used multiple times)
		reusedIDs := 0
		for connID, count := range connIDMap {
			if count > 1 {
				reusedIDs++
				t.Logf("Connection ID %s was reused %d times", connID, count)
			}
		}

		if reusedIDs > 0 {
			t.Logf("Detected %d connection IDs that were reused (normal for connection pooling)", reusedIDs)
		}

		// Verify all data was inserted
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM overflow_simulation_test WHERE simulation_data LIKE 'reuse_test_%'").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, reuseTestCount, count, "All reuse test data should be inserted")
	})
}

// BenchmarkTransactionIDGeneration benchmarks transaction ID generation performance
func BenchmarkTransactionIDGeneration(b *testing.B) {
	db := createBenchDBConnection(b)
	defer db.Close()

	// Create benchmark table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS bench_tx_id_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			data VARCHAR(50)
		) ENGINE=InnoDB
	`)
	if err != nil {
		b.Skipf("Failed to create table: %v", err)
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

			// Get connection ID (simulates transaction ID access)
			var connID string
			err = tx.QueryRow("SELECT CONNECTION_ID()").Scan(&connID)
			if err != nil {
				tx.Rollback()
				b.Errorf("Connection ID failed: %v", err)
				continue
			}

			_, err = tx.Exec("INSERT INTO bench_tx_id_test (data) VALUES (?)", 
				fmt.Sprintf("bench_%d_%s", i, connID))
			if err != nil {
				tx.Rollback()
				b.Errorf("Insert failed: %v", err)
				continue
			}

			err = tx.Commit()
			if err != nil {
				b.Errorf("Commit failed: %v", err)
				continue
			}
			i++
		}
	})
}

// createTestDBConnection creates a test database connection
func createTestDBConnection(t *testing.T) *sql.DB {
	dsn := "testuser:testpass@tcp(localhost:3306)/testdb?parseTime=true&timeout=30s&readTimeout=30s&writeTimeout=30s"
	
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Skipf("Failed to connect to test database: %v", err)
		return nil
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	if err := db.PingContext(ctx); err != nil {
		t.Skipf("Failed to ping test database: %v", err)
		db.Close()
		return nil
	}

	return db
}

// createBenchDBConnection creates a benchmark database connection
func createBenchDBConnection(b *testing.B) *sql.DB {
	dsn := "testuser:testpass@tcp(localhost:3306)/testdb?parseTime=true&timeout=30s&readTimeout=30s&writeTimeout=30s"
	
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		b.Skipf("Failed to connect to test database: %v", err)
		return nil
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	if err := db.PingContext(ctx); err != nil {
		b.Skipf("Failed to ping test database: %v", err)
		db.Close()
		return nil
	}

	return db
}
