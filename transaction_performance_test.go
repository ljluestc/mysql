// Go MySQL Driver - Transaction Performance Tests
//
// Copyright 2012 The Go-MySQL-Driver Authors. All rights reserved.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package mysql

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// BenchmarkTransactionBasicInsert benchmarks basic transaction insert performance
func BenchmarkTransactionBasicInsert(b *testing.B) {
	db := createBenchDBConnection(b)
	defer db.Close()

	// Create test table
	_, err := db.Exec(`
		CREATE TEMPORARY TABLE bench_basic_insert (
			id INT PRIMARY KEY AUTO_INCREMENT,
			value VARCHAR(50)
		) ENGINE=InnoDB
	`)
	if err != nil {
		b.Skipf("Failed to create table: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tx, err := db.Begin()
		if err != nil {
			b.Errorf("Begin failed: %v", err)
			continue
		}

		_, err = tx.Exec("INSERT INTO bench_basic_insert (value) VALUES (?)", fmt.Sprintf("bench_%d", i))
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
	}
}

// BenchmarkTransactionBatchInsert benchmarks batch insert performance
func BenchmarkTransactionBatchInsert(b *testing.B) {
	db := createBenchDBConnection(b)
	defer db.Close()

	// Create test table
	_, err := db.Exec(`
		CREATE TEMPORARY TABLE bench_batch_insert (
			id INT PRIMARY KEY AUTO_INCREMENT,
			value VARCHAR(50)
		) ENGINE=InnoDB
	`)
	if err != nil {
		b.Skipf("Failed to create table: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tx, err := db.Begin()
		if err != nil {
			b.Errorf("Begin failed: %v", err)
			continue
		}

		// Insert 100 rows per transaction
		for j := 0; j < 100; j++ {
			_, err = tx.Exec("INSERT INTO bench_batch_insert (value) VALUES (?)", 
				fmt.Sprintf("bench_%d_%d", i, j))
			if err != nil {
				tx.Rollback()
				b.Errorf("Insert failed: %v", err)
				continue
			}
		}

		err = tx.Commit()
		if err != nil {
			b.Errorf("Commit failed: %v", err)
			continue
		}
	}
}

// BenchmarkTransactionReadPerformance benchmarks read performance in transactions
func BenchmarkTransactionReadPerformance(b *testing.B) {
	db := createBenchDBConnection(b)
	defer db.Close()

	// Create and populate test table
	_, err := db.Exec(`
		CREATE TEMPORARY TABLE bench_read_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			value VARCHAR(50),
			data TEXT
		) ENGINE=InnoDB
	`)
	if err != nil {
		b.Skipf("Failed to create table: %v", err)
	}

	// Populate with test data
	tx, err := db.Begin()
	if err != nil {
		b.Skipf("Failed to begin transaction: %v", err)
		return
	}

	for i := 0; i < 1000; i++ {
		_, err = tx.Exec("INSERT INTO bench_read_test (value, data) VALUES (?, ?)", 
			fmt.Sprintf("value_%d", i), fmt.Sprintf("data_%d", i))
		if err != nil {
			tx.Rollback()
			b.Skipf("Failed to populate data: %v", err)
			return
		}
	}

	err = tx.Commit()
	if err != nil {
		b.Skipf("Failed to commit data: %v", err)
		return
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tx, err := db.Begin()
		if err != nil {
			b.Errorf("Begin failed: %v", err)
			continue
		}

		// Read 100 rows
		rows, err := tx.Query("SELECT id, value, data FROM bench_read_test LIMIT 100")
		if err != nil {
			tx.Rollback()
			b.Errorf("Query failed: %v", err)
			continue
		}

		count := 0
		for rows.Next() {
			var id int
			var value, data string
			err := rows.Scan(&id, &value, &data)
			if err != nil {
				rows.Close()
				tx.Rollback()
				b.Errorf("Scan failed: %v", err)
				continue
			}
			count++
		}
		rows.Close()

		err = tx.Commit()
		if err != nil {
			b.Errorf("Commit failed: %v", err)
			continue
		}

		if count != 100 {
			b.Errorf("Expected 100 rows, got %d", count)
		}
	}
}

// BenchmarkTransactionUpdatePerformance benchmarks update performance
func BenchmarkTransactionUpdatePerformance(b *testing.B) {
	db := createBenchDBConnection(b)
	defer db.Close()

	// Create and populate test table
	_, err := db.Exec(`
		CREATE TEMPORARY TABLE bench_update_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			value VARCHAR(50),
			counter INT DEFAULT 0
		) ENGINE=InnoDB
	`)
	if err != nil {
		b.Skipf("Failed to create table: %v", err)
	}

	// Populate with test data
	tx, err := db.Begin()
	if err != nil {
		b.Skipf("Failed to begin transaction: %v", err)
		return
	}

	for i := 0; i < 100; i++ {
		_, err = tx.Exec("INSERT INTO bench_update_test (value) VALUES (?)", 
			fmt.Sprintf("value_%d", i))
		if err != nil {
			tx.Rollback()
			b.Skipf("Failed to populate data: %v", err)
			return
		}
	}

	err = tx.Commit()
	if err != nil {
		b.Skipf("Failed to commit data: %v", err)
		return
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tx, err := db.Begin()
		if err != nil {
			b.Errorf("Begin failed: %v", err)
			continue
		}

		// Update 10 rows
		_, err = tx.Exec("UPDATE bench_update_test SET counter = counter + 1 WHERE id <= 10")
		if err != nil {
			tx.Rollback()
			b.Errorf("Update failed: %v", err)
			continue
		}

		err = tx.Commit()
		if err != nil {
			b.Errorf("Commit failed: %v", err)
			continue
		}
	}
}

// BenchmarkTransactionConcurrentPerformance benchmarks concurrent transaction performance
func BenchmarkTransactionConcurrentPerformance(b *testing.B) {
	db := createBenchDBConnection(b)
	defer db.Close()

	// Create test table
	_, err := db.Exec(`
		CREATE TEMPORARY TABLE bench_concurrent_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			value VARCHAR(50),
			goroutine_id INT
		) ENGINE=InnoDB
	`)
	if err != nil {
		b.Skipf("Failed to create table: %v", err)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		goroutineID := 0
		for pb.Next() {
			tx, err := db.Begin()
			if err != nil {
				b.Errorf("Begin failed: %v", err)
				continue
			}

			_, err = tx.Exec("INSERT INTO bench_concurrent_test (value, goroutine_id) VALUES (?, ?)", 
				fmt.Sprintf("concurrent_%d", goroutineID), goroutineID)
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
			goroutineID++
		}
	})
}

// BenchmarkTransactionRollbackPerformance benchmarks rollback performance
func BenchmarkTransactionRollbackPerformance(b *testing.B) {
	db := createBenchDBConnection(b)
	defer db.Close()

	// Create test table
	_, err := db.Exec(`
		CREATE TEMPORARY TABLE bench_rollback_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			value VARCHAR(50)
		) ENGINE=InnoDB
	`)
	if err != nil {
		b.Skipf("Failed to create table: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tx, err := db.Begin()
		if err != nil {
			b.Errorf("Begin failed: %v", err)
			continue
		}

		// Insert some data
		_, err = tx.Exec("INSERT INTO bench_rollback_test (value) VALUES (?)", 
			fmt.Sprintf("rollback_%d", i))
		if err != nil {
			tx.Rollback()
			b.Errorf("Insert failed: %v", err)
			continue
		}

		// Rollback instead of commit
		err = tx.Rollback()
		if err != nil {
			b.Errorf("Rollback failed: %v", err)
			continue
		}
	}
}

// BenchmarkTransactionSavepointPerformance benchmarks savepoint performance
func BenchmarkTransactionSavepointPerformance(b *testing.B) {
	db := createBenchDBConnection(b)
	defer db.Close()

	// Create test table
	_, err := db.Exec(`
		CREATE TEMPORARY TABLE bench_savepoint_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			value VARCHAR(50)
		) ENGINE=InnoDB
	`)
	if err != nil {
		b.Skipf("Failed to create table: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tx, err := db.Begin()
		if err != nil {
			b.Errorf("Begin failed: %v", err)
			continue
		}

		// Create savepoint
		_, err = tx.Exec("SAVEPOINT sp")
		if err != nil {
			tx.Rollback()
			b.Errorf("Savepoint failed: %v", err)
			continue
		}

		// Insert data
		_, err = tx.Exec("INSERT INTO bench_savepoint_test (value) VALUES (?)", 
			fmt.Sprintf("savepoint_%d", i))
		if err != nil {
			tx.Rollback()
			b.Errorf("Insert failed: %v", err)
			continue
		}

		// Rollback to savepoint
		_, err = tx.Exec("ROLLBACK TO SAVEPOINT sp")
		if err != nil {
			tx.Rollback()
			b.Errorf("Rollback to savepoint failed: %v", err)
			continue
		}

		err = tx.Commit()
		if err != nil {
			b.Errorf("Commit failed: %v", err)
			continue
		}
	}
}

// BenchmarkTransactionMixedWorkload benchmarks mixed read/write workload
func BenchmarkTransactionMixedWorkload(b *testing.B) {
	db := createBenchDBConnection(b)
	defer db.Close()

	// Create and populate test table
	_, err := db.Exec(`
		CREATE TEMPORARY TABLE bench_mixed_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			value VARCHAR(50),
			counter INT DEFAULT 0
		) ENGINE=InnoDB
	`)
	if err != nil {
		b.Skipf("Failed to create table: %v", err)
	}

	// Populate with initial data
	tx, err := db.Begin()
	if err != nil {
		b.Skipf("Failed to begin transaction: %v", err)
		return
	}

	for i := 0; i < 50; i++ {
		_, err = tx.Exec("INSERT INTO bench_mixed_test (value) VALUES (?)", 
			fmt.Sprintf("initial_%d", i))
		if err != nil {
			tx.Rollback()
			b.Skipf("Failed to populate data: %v", err)
			return
		}
	}

	err = tx.Commit()
	if err != nil {
		b.Skipf("Failed to commit data: %v", err)
		return
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tx, err := db.Begin()
		if err != nil {
			b.Errorf("Begin failed: %v", err)
			continue
		}

		// Read operation
		rows, err := tx.Query("SELECT id, value FROM bench_mixed_test LIMIT 10")
		if err != nil {
			tx.Rollback()
			b.Errorf("Query failed: %v", err)
			continue
		}

		count := 0
		for rows.Next() {
			var id int
			var value string
			err := rows.Scan(&id, &value)
			if err != nil {
				rows.Close()
				tx.Rollback()
				b.Errorf("Scan failed: %v", err)
				continue
			}
			count++
		}
		rows.Close()

		// Write operation
		_, err = tx.Exec("INSERT INTO bench_mixed_test (value) VALUES (?)", 
			fmt.Sprintf("mixed_%d", i))
		if err != nil {
			tx.Rollback()
			b.Errorf("Insert failed: %v", err)
			continue
		}

		// Update operation
		_, err = tx.Exec("UPDATE bench_mixed_test SET counter = counter + 1 WHERE id <= 5")
		if err != nil {
			tx.Rollback()
			b.Errorf("Update failed: %v", err)
			continue
		}

		err = tx.Commit()
		if err != nil {
			b.Errorf("Commit failed: %v", err)
			continue
		}

		if count != 10 {
			b.Errorf("Expected 10 rows, got %d", count)
		}
	}
}

// TestTransactionPerformanceMetrics tests transaction performance metrics
func TestTransactionPerformanceMetrics(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	// Create test table
	_, err := db.Exec(`
		CREATE TEMPORARY TABLE perf_metrics_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			value VARCHAR(50)
		) ENGINE=InnoDB
	`)
	require.NoError(t, err)

	const numTransactions = 100
	var totalDuration time.Duration
	var successCount int

	for i := 0; i < numTransactions; i++ {
		start := time.Now()

		tx, err := db.Begin()
		require.NoError(t, err)

		// Insert data
		_, err = tx.Exec("INSERT INTO perf_metrics_test (value) VALUES (?)", 
			fmt.Sprintf("perf_%d", i))
		require.NoError(t, err)

		err = tx.Commit()
		require.NoError(t, err)

		duration := time.Since(start)
		totalDuration += duration
		successCount++
	}

	avgDuration := totalDuration / time.Duration(successCount)
	t.Logf("Performance Metrics:")
	t.Logf("  Total transactions: %d", successCount)
	t.Logf("  Average transaction duration: %v", avgDuration)
	t.Logf("  Total duration: %v", totalDuration)
	t.Logf("  Transactions per second: %.2f", 
		float64(successCount)/totalDuration.Seconds())

	// Performance should be reasonable (less than 100ms per transaction on average)
	assert.Less(t, avgDuration, 100*time.Millisecond, 
		"Average transaction duration should be less than 100ms")
}

// TestTransactionMemoryUsage tests memory usage during transactions
func TestTransactionMemoryUsage(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	// Create test table
	_, err := db.Exec(`
		CREATE TEMPORARY TABLE memory_usage_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			data BLOB
		) ENGINE=InnoDB
	`)
	require.NoError(t, err)

	// Generate test data
	dataSizes := []int{
		1024,      // 1KB
		10240,     // 10KB
		102400,    // 100KB
		1024000,   // 1MB
	}

	for _, dataSize := range dataSizes {
		t.Run(fmt.Sprintf("DataSize_%d", dataSize), func(t *testing.T) {
			data := make([]byte, dataSize)
			for i := range data {
				data[i] = byte(i % 256)
			}

			tx, err := db.Begin()
			require.NoError(t, err)

			// Insert large data
			_, err = tx.Exec("INSERT INTO memory_usage_test (data) VALUES (?)", data)
			require.NoError(t, err)

			// Retrieve data
			var retrievedData []byte
			err = tx.QueryRow("SELECT data FROM memory_usage_test WHERE id = ?", 1).Scan(&retrievedData)
			require.NoError(t, err)

			// Verify data integrity
			assert.Equal(t, len(data), len(retrievedData), "Data length should match")
			assert.Equal(t, data, retrievedData, "Data should match")

			err = tx.Commit()
			assert.NoError(t, err)

			t.Logf("Successfully handled %d bytes of data", dataSize)
		})
	}
}

// TestTransactionConcurrencyScaling tests how transaction performance scales with concurrency
func TestTransactionConcurrencyScaling(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	// Create test table
	_, err := db.Exec(`
		CREATE TEMPORARY TABLE scaling_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			value VARCHAR(50),
			goroutine_id INT
		) ENGINE=InnoDB
	`)
	require.NoError(t, err)

	concurrencyLevels := []int{1, 2, 4, 8, 16}

	for _, concurrency := range concurrencyLevels {
		t.Run(fmt.Sprintf("Concurrency_%d", concurrency), func(t *testing.T) {
			const operationsPerGoroutine = 10
			var wg sync.WaitGroup
			start := time.Now()

			for i := 0; i < concurrency; i++ {
				wg.Add(1)
				go func(goroutineID int) {
					defer wg.Done()

					for j := 0; j < operationsPerGoroutine; j++ {
						tx, err := db.Begin()
						if err != nil {
							t.Errorf("Begin failed: %v", err)
							return
						}

						_, err = tx.Exec("INSERT INTO scaling_test (value, goroutine_id) VALUES (?, ?)", 
							fmt.Sprintf("scale_%d_%d", goroutineID, j), goroutineID)
						if err != nil {
							tx.Rollback()
							t.Errorf("Insert failed: %v", err)
							return
						}

						err = tx.Commit()
						if err != nil {
							t.Errorf("Commit failed: %v", err)
							return
						}
					}
				}(i)
			}

			wg.Wait()
			duration := time.Since(start)

			totalOperations := concurrency * operationsPerGoroutine
			opsPerSecond := float64(totalOperations) / duration.Seconds()

			t.Logf("Concurrency %d: %v total, %.2f ops/sec", 
				concurrency, duration, opsPerSecond)

			// Verify all data was inserted
			var count int
			err = db.QueryRow("SELECT COUNT(*) FROM scaling_test").Scan(&count)
			require.NoError(t, err)
			assert.Equal(t, totalOperations, count, "All operations should succeed")
		})
	}
}
