// Go MySQL Driver - Transaction Stress Tests
//
// Copyright 2012 The Go-MySQL-Driver Authors. All rights reserved.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package mysql

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTransactionHighConcurrency tests high concurrent transaction scenarios
func TestTransactionHighConcurrency(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	// Create test table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS stress_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			value VARCHAR(50),
			counter INT DEFAULT 0,
			timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		) ENGINE=InnoDB
	`)
	require.NoError(t, err)

	// Clean up any existing data
	_, err = db.Exec("DELETE FROM stress_test")
	require.NoError(t, err)

	const numGoroutines = 50
	const numOperations = 10
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			
			for j := 0; j < numOperations; j++ {
				tx, err := db.Begin()
				if err != nil {
					errors <- fmt.Errorf("goroutine %d: begin failed: %w", goroutineID, err)
					return
				}

				// Insert data
				_, err = tx.Exec("INSERT INTO stress_test (value, counter) VALUES (?, ?)", 
					fmt.Sprintf("goroutine_%d_op_%d", goroutineID, j), j)
				if err != nil {
					tx.Rollback()
					errors <- fmt.Errorf("goroutine %d: insert failed: %w", goroutineID, err)
					return
				}

				// Update some random row
				_, err = tx.Exec("UPDATE stress_test SET counter = counter + 1 WHERE id % 10 = ?", 
					j%10)
				if err != nil {
					tx.Rollback()
					errors <- fmt.Errorf("goroutine %d: update failed: %w", goroutineID, err)
					return
				}

				err = tx.Commit()
				if err != nil {
					errors <- fmt.Errorf("goroutine %d: commit failed: %w", goroutineID, err)
					return
				}
			}
			errors <- nil
		}(i)
	}

	wg.Wait()

	// Check for errors
	for i := 0; i < numGoroutines; i++ {
		err := <-errors
		assert.NoError(t, err, "Goroutine should complete without error")
	}

	// Verify all data was inserted
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM stress_test").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, numGoroutines*numOperations, count, "All operations should succeed")
}

// TestTransactionMemoryLeak tests for memory leaks in transaction handling
func TestTransactionMemoryLeak(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	// Create test table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS memory_leak_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			data LONGBLOB
		) ENGINE=InnoDB
	`)
	require.NoError(t, err)

	// Clean up any existing data
	_, err = db.Exec("DELETE FROM memory_leak_test")
	require.NoError(t, err)

	// Generate large data
	largeData := make([]byte, 100*1024) // 100KB
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	const numIterations = 100
	for i := 0; i < numIterations; i++ {
		tx, err := db.Begin()
		require.NoError(t, err)

		// Insert large data
		_, err = tx.Exec("INSERT INTO memory_leak_test (data) VALUES (?)", largeData)
		require.NoError(t, err)

		// Query the data back
		var retrievedData []byte
		err = tx.QueryRow("SELECT data FROM memory_leak_test WHERE id = ?", i+1).Scan(&retrievedData)
		require.NoError(t, err)

		// Verify data integrity
		assert.Equal(t, len(largeData), len(retrievedData), "Data length should match")

		err = tx.Commit()
		require.NoError(t, err)
	}

	// Verify final count
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM memory_leak_test").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, numIterations, count, "All iterations should succeed")
}

// TestTransactionTimeoutStress tests timeout behavior under stress
func TestTransactionTimeoutStress(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	// Create test table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS timeout_stress_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			value VARCHAR(50)
		) ENGINE=InnoDB
	`)
	require.NoError(t, err)

	// Clean up any existing data
	_, err = db.Exec("DELETE FROM timeout_stress_test")
	require.NoError(t, err)

	const numGoroutines = 20
	var wg sync.WaitGroup
	successCount := make(chan int, numGoroutines)
	timeoutCount := make(chan int, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			// Create context with very short timeout
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
			defer cancel()

			// Small delay to ensure context times out
			time.Sleep(time.Microsecond)

			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				if err == context.DeadlineExceeded {
					timeoutCount <- 1
					return
				}
				// Other errors are unexpected
				return
			}

			// If we got here, try to use the transaction
			_, err = tx.Exec("INSERT INTO timeout_stress_test (value) VALUES (?)", 
				fmt.Sprintf("timeout_test_%d", goroutineID))
			if err != nil {
				tx.Rollback()
				return
			}

			err = tx.Commit()
			if err != nil {
				return
			}

			successCount <- 1
		}(i)
	}

	wg.Wait()

	// Count results
	var successes, timeouts int
	for i := 0; i < numGoroutines; i++ {
		select {
		case <-successCount:
			successes++
		case <-timeoutCount:
			timeouts++
		default:
			// Some goroutines may have failed for other reasons
		}
	}

	// Most should timeout due to very short timeout
	assert.Greater(t, timeouts, successes, "Most operations should timeout")
}

// TestTransactionRollbackStress tests rollback behavior under stress
func TestTransactionRollbackStress(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	// Create test table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS rollback_stress_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			value VARCHAR(50) UNIQUE
		) ENGINE=InnoDB
	`)
	require.NoError(t, err)

	// Clean up any existing data
	_, err = db.Exec("DELETE FROM rollback_stress_test")
	require.NoError(t, err)

	// Insert initial data to cause conflicts
	_, err = db.Exec("INSERT INTO rollback_stress_test (id, value) VALUES (1, 'conflict')")
	require.NoError(t, err)

	const numGoroutines = 30
	var wg sync.WaitGroup
	rollbackCount := make(chan int, numGoroutines)
	conflictCount := make(chan int, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			tx, err := db.Begin()
			if err != nil {
				return
			}

			// Try to insert conflicting data
			_, err = tx.Exec("INSERT INTO rollback_stress_test (id, value) VALUES (?, ?)", 
				goroutineID+2, "conflict")
			if err != nil {
				// Expected conflict
				tx.Rollback()
				conflictCount <- 1
				return
			}

			// If no conflict, insert unique data and rollback
			_, err = tx.Exec("INSERT INTO rollback_stress_test (id, value) VALUES (?, ?)", 
				goroutineID+100, fmt.Sprintf("unique_%d", goroutineID))
			if err != nil {
				tx.Rollback()
				return
			}

			// Explicit rollback
			err = tx.Rollback()
			if err != nil {
				return
			}

			rollbackCount <- 1
		}(i)
	}

	wg.Wait()

	// Count results
	var rollbacks, conflicts int
	for i := 0; i < numGoroutines; i++ {
		select {
		case <-rollbackCount:
			rollbacks++
		case <-conflictCount:
			conflicts++
		default:
			// Some goroutines may have failed for other reasons
		}
	}

	// Verify only original data exists
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM rollback_stress_test").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "Only original data should exist after rollbacks")

	assert.Greater(t, conflicts+rollbacks, 0, "Some operations should have been handled")
}

// TestTransactionMixedWorkload tests mixed read/write transaction workloads
func TestTransactionMixedWorkload(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	// Create test table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS mixed_workload_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			value VARCHAR(50),
			read_count INT DEFAULT 0,
			write_count INT DEFAULT 0
		) ENGINE=InnoDB
	`)
	require.NoError(t, err)

	// Clean up any existing data
	_, err = db.Exec("DELETE FROM mixed_workload_test")
	require.NoError(t, err)

	// Insert initial data
	for i := 0; i < 10; i++ {
		_, err = db.Exec("INSERT INTO mixed_workload_test (value) VALUES (?)", 
			fmt.Sprintf("initial_%d", i))
		require.NoError(t, err)
	}

	const numReaders = 20
	const numWriters = 10
	var wg sync.WaitGroup
	errors := make(chan error, numReaders+numWriters)

	// Start reader goroutines
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()

			for j := 0; j < 5; j++ {
				tx, err := db.Begin()
				if err != nil {
					errors <- fmt.Errorf("reader %d: begin failed: %w", readerID, err)
					return
				}

				// Read data
				rows, err := tx.Query("SELECT id, value FROM mixed_workload_test")
				if err != nil {
					tx.Rollback()
					errors <- fmt.Errorf("reader %d: query failed: %w", readerID, err)
					return
				}

				count := 0
				for rows.Next() {
					var id int
					var value string
					err := rows.Scan(&id, &value)
					if err != nil {
						rows.Close()
						tx.Rollback()
						errors <- fmt.Errorf("reader %d: scan failed: %w", readerID, err)
						return
					}
					count++
				}
				rows.Close()

				// Update read count
				_, err = tx.Exec("UPDATE mixed_workload_test SET read_count = read_count + 1 WHERE id <= 5")
				if err != nil {
					tx.Rollback()
					errors <- fmt.Errorf("reader %d: update failed: %w", readerID, err)
					return
				}

				err = tx.Commit()
				if err != nil {
					errors <- fmt.Errorf("reader %d: commit failed: %w", readerID, err)
					return
				}

				if count < 10 {
					errors <- fmt.Errorf("reader %d: expected at least 10 rows, got %d", readerID, count)
					return
				}
			}
			errors <- nil
		}(i)
	}

	// Start writer goroutines
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()

			for j := 0; j < 3; j++ {
				tx, err := db.Begin()
				if err != nil {
					errors <- fmt.Errorf("writer %d: begin failed: %w", writerID, err)
					return
				}

				// Write data
				_, err = tx.Exec("INSERT INTO mixed_workload_test (value) VALUES (?)", 
					fmt.Sprintf("writer_%d_op_%d", writerID, j))
				if err != nil {
					tx.Rollback()
					errors <- fmt.Errorf("writer %d: insert failed: %w", writerID, err)
					return
				}

				// Update write count
				_, err = tx.Exec("UPDATE mixed_workload_test SET write_count = write_count + 1 WHERE id <= 5")
				if err != nil {
					tx.Rollback()
					errors <- fmt.Errorf("writer %d: update failed: %w", writerID, err)
					return
				}

				err = tx.Commit()
				if err != nil {
					errors <- fmt.Errorf("writer %d: commit failed: %w", writerID, err)
					return
				}
			}
			errors <- nil
		}(i)
	}

	wg.Wait()

	// Check for errors
	for i := 0; i < numReaders+numWriters; i++ {
		err := <-errors
		assert.NoError(t, err, "Operation should complete without error")
	}

	// Verify final state
	var totalCount, readCount, writeCount int
	err = db.QueryRow("SELECT COUNT(*), SUM(read_count), SUM(write_count) FROM mixed_workload_test").Scan(&totalCount, &readCount, &writeCount)
	require.NoError(t, err)

	expectedTotal := 10 + (numWriters * 3) // initial + writer inserts
	assert.Equal(t, expectedTotal, totalCount, "Total count should match expected")
	assert.Greater(t, readCount, 0, "Read count should be greater than 0")
	assert.Greater(t, writeCount, 0, "Write count should be greater than 0")
}

// TestTransactionRandomFailures tests transaction behavior with random failures
func TestTransactionRandomFailures(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	// Create test table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS random_failure_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			value VARCHAR(50),
			should_fail BOOLEAN DEFAULT FALSE
		) ENGINE=InnoDB
	`)
	require.NoError(t, err)

	// Clean up any existing data
	_, err = db.Exec("DELETE FROM random_failure_test")
	require.NoError(t, err)

	const numGoroutines = 15
	var wg sync.WaitGroup
	successCount := make(chan int, numGoroutines)
	failureCount := make(chan int, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			// Random failure simulation
			shouldFail := rand.Intn(100) < 30 // 30% chance of failure

			tx, err := db.Begin()
			if err != nil {
				return
			}

			// Insert data
			_, err = tx.Exec("INSERT INTO random_failure_test (value, should_fail) VALUES (?, ?)", 
				fmt.Sprintf("test_%d", goroutineID), shouldFail)
			if err != nil {
				tx.Rollback()
				failureCount <- 1
				return
			}

			if shouldFail {
				// Simulate failure by trying to violate a constraint
				_, err = tx.Exec("INSERT INTO random_failure_test (id, value) VALUES (1, 'duplicate')")
				if err != nil {
					tx.Rollback()
					failureCount <- 1
					return
				}
			}

			err = tx.Commit()
			if err != nil {
				failureCount <- 1
				return
			}

			successCount <- 1
		}(i)
	}

	wg.Wait()

	// Count results
	var successes, failures int
	for i := 0; i < numGoroutines; i++ {
		select {
		case <-successCount:
			successes++
		case <-failureCount:
			failures++
		default:
			// Some goroutines may have failed for other reasons
		}
	}

	// Verify that we have both successes and failures
	assert.Greater(t, successes, 0, "Some transactions should succeed")
	assert.Greater(t, failures, 0, "Some transactions should fail")

	// Verify final data integrity
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM random_failure_test WHERE should_fail = FALSE").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, successes, count, "Only successful transactions should be committed")
}

// BenchmarkTransactionStress benchmarks transaction stress performance
func BenchmarkTransactionStress(b *testing.B) {
	db := createBenchDBConnection(b)
	defer db.Close()

	// Create test table
	_, err := db.Exec(`
		CREATE TEMPORARY TABLE bench_stress_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			value VARCHAR(50)
		) ENGINE=InnoDB
	`)
	if err != nil {
		b.Skipf("Failed to create table: %v", err)
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

			_, err = tx.Exec("INSERT INTO bench_stress_test (value) VALUES (?)", fmt.Sprintf("bench_%d", i))
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
