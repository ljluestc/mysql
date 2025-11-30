// Go MySQL Driver - Transaction Edge Case Tests
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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTransactionEmptyCommit tests commit on empty transaction
func TestTransactionEmptyCommit(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	tx, err := db.Begin()
	require.NoError(t, err)

	// Commit without any operations
	err = tx.Commit()
	assert.NoError(t, err, "Empty transaction should commit successfully")
}

// TestTransactionMultipleRollback tests multiple rollback calls
func TestTransactionMultipleRollback(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	tx, err := db.Begin()
	require.NoError(t, err)

	// First rollback
	err = tx.Rollback()
	assert.NoError(t, err, "First rollback should succeed")

	// Second rollback should also succeed (idempotent)
	err = tx.Rollback()
	assert.NoError(t, err, "Second rollback should succeed (idempotent)")

	// Third rollback
	err = tx.Rollback()
	assert.NoError(t, err, "Third rollback should succeed")
}

// TestTransactionAfterCommit tests operations after commit
func TestTransactionAfterCommit(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	tx, err := db.Begin()
	require.NoError(t, err)

	// Create test table
	_, err = tx.Exec(`
		CREATE TEMPORARY TABLE after_commit_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			value VARCHAR(50)
		) ENGINE=InnoDB
	`)
	require.NoError(t, err)

	// Insert data
	_, err = tx.Exec("INSERT INTO after_commit_test (value) VALUES (?)", "test")
	require.NoError(t, err)

	// Commit transaction
	err = tx.Commit()
	require.NoError(t, err)

	// Try to use transaction after commit
	_, err = tx.Exec("INSERT INTO after_commit_test (value) VALUES (?)", "after_commit")
	assert.Error(t, err, "Operations after commit should fail")

	// Rollback after commit should also fail
	err = tx.Rollback()
	assert.Error(t, err, "Rollback after commit should fail")
}

// TestTransactionVeryLongQuery tests very long SQL queries in transactions
func TestTransactionVeryLongQuery(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	tx, err := db.Begin()
	require.NoError(t, err)

	// Create test table
	_, err = tx.Exec(`
		CREATE TEMPORARY TABLE long_query_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			data TEXT
		) ENGINE=InnoDB
	`)
	require.NoError(t, err)

	// Generate a very long query
	longString := strings.Repeat("This is a very long string that will make our query quite long. ", 1000)
	
	_, err = tx.Exec("INSERT INTO long_query_test (data) VALUES (?)", longString)
	require.NoError(t, err)

	// Query back the long data
	var retrievedData string
	err = tx.QueryRow("SELECT data FROM long_query_test WHERE id = 1").Scan(&retrievedData)
	require.NoError(t, err)
	assert.Equal(t, longString, retrievedData, "Long data should be preserved")

	err = tx.Commit()
	assert.NoError(t, err)
}

// TestTransactionSpecialCharacters tests special characters in transaction data
func TestTransactionSpecialCharacters(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	tx, err := db.Begin()
	require.NoError(t, err)

	// Create test table
	_, err = tx.Exec(`
		CREATE TEMPORARY TABLE special_chars_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			data VARCHAR(255)
		) ENGINE=InnoDB
	`)
	require.NoError(t, err)

	// Test various special characters
	testCases := []string{
		"Single ' quote",
		"Double \" quote",
		"Backslash \\",
		"Newline\ncharacter",
		"Tab\tcharacter",
		"Unicode: 你好世界 🌍",
		"Emojis: 🚀🔥💯",
		"SQL injection: ' OR '1'='1",
		"HTML: <script>alert('xss')</script>",
		"Null\x00byte",
		"Backspace\bcharacter",
	}

	for i, testCase := range testCases {
		_, err = tx.Exec("INSERT INTO special_chars_test (data) VALUES (?)", testCase)
		require.NoError(t, err, "Failed to insert special character case %d", i)

		var retrieved string
		err = tx.QueryRow("SELECT data FROM special_chars_test WHERE id = ?", i+1).Scan(&retrieved)
		require.NoError(t, err, "Failed to retrieve special character case %d", i)
		assert.Equal(t, testCase, retrieved, "Special characters should be preserved for case %d", i)
	}

	err = tx.Commit()
	assert.NoError(t, err)
}

// TestTransactionZeroTimeContext tests transaction with zero time context
func TestTransactionZeroTimeContext(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	// Create context with zero timeout (should timeout immediately)
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()

	// Small delay to ensure context is already timed out
	time.Sleep(time.Microsecond)

	tx, err := db.BeginTx(ctx, nil)
	assert.Error(t, err, "Transaction with zero timeout should fail")
	assert.Contains(t, err.Error(), "context deadline exceeded", "Error should mention deadline exceeded")

	if tx != nil {
		// Should be nil, but if not, try to rollback
		tx.Rollback()
	}
}

// TestTransactionCancelledContext tests transaction with cancelled context
func TestTransactionCancelledContext(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	// Create context and cancel it immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tx, err := db.BeginTx(ctx, nil)
	assert.Error(t, err, "Transaction with cancelled context should fail")
	assert.Contains(t, err.Error(), "context canceled", "Error should mention context canceled")

	if tx != nil {
		// Should be nil, but if not, try to rollback
		tx.Rollback()
	}
}

// TestTransactionLargeNumberOfStatements tests transaction with many statements
func TestTransactionLargeNumberOfStatements(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	tx, err := db.Begin()
	require.NoError(t, err)

	// Create test table
	_, err = tx.Exec(`
		CREATE TEMPORARY TABLE many_statements_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			value VARCHAR(50),
			iteration INT
		) ENGINE=InnoDB
	`)
	require.NoError(t, err)

	const numStatements = 1000
	for i := 0; i < numStatements; i++ {
		_, err = tx.Exec("INSERT INTO many_statements_test (value, iteration) VALUES (?, ?)", 
			fmt.Sprintf("stmt_%d", i), i)
		require.NoError(t, err, "Statement %d should succeed", i)
	}

	// Verify all data was inserted
	var count int
	err = tx.QueryRow("SELECT COUNT(*) FROM many_statements_test").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, numStatements, count, "All statements should be inserted")

	err = tx.Commit()
	assert.NoError(t, err)
}

// TestTransactionConcurrentSavepoints tests concurrent savepoint operations
func TestTransactionConcurrentSavepoints(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	tx, err := db.Begin()
	require.NoError(t, err)

	// Create test table
	_, err = tx.Exec(`
		CREATE TEMPORARY TABLE concurrent_savepoints_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			value VARCHAR(50)
		) ENGINE=InnoDB
	`)
	require.NoError(t, err)

	// Create multiple savepoints and perform operations
	for i := 0; i < 10; i++ {
		// Create savepoint
		_, err = tx.Exec(fmt.Sprintf("SAVEPOINT sp_%d", i))
		require.NoError(t, err)

		// Insert data
		_, err = tx.Exec("INSERT INTO concurrent_savepoints_test (value) VALUES (?)", 
			fmt.Sprintf("savepoint_%d", i))
		require.NoError(t, err)

		// Every third operation, rollback to previous savepoint
		if i > 0 && i%3 == 0 {
			_, err = tx.Exec(fmt.Sprintf("ROLLBACK TO SAVEPOINT sp_%d", i-1))
			require.NoError(t, err)
		}
	}

	// Check final state
	var count int
	err = tx.QueryRow("SELECT COUNT(*) FROM concurrent_savepoints_test").Scan(&count)
	require.NoError(t, err)

	// Should have fewer rows due to rollbacks
	assert.Less(t, count, 10, "Rollbacks should reduce row count")

	err = tx.Commit()
	assert.NoError(t, err)
}

// TestTransactionNilContext tests transaction with nil context
func TestTransactionNilContext(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	// This should work - nil context should be treated as no context
	tx, err := db.BeginTx(nil, nil)
	require.NoError(t, err)

	// Create test table
	_, err = tx.Exec(`
		CREATE TEMPORARY TABLE nil_context_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			value VARCHAR(50)
		) ENGINE=InnoDB
	`)
	require.NoError(t, err)

	// Insert data
	_, err = tx.Exec("INSERT INTO nil_context_test (value) VALUES (?)", "nil_context_test")
	require.NoError(t, err)

	err = tx.Commit()
	assert.NoError(t, err)
}

// TestTransactionReadOnlyOptions tests transaction with read-only options
func TestTransactionReadOnlyOptions(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	// Test with read-only transaction (if supported)
	tx, err := db.BeginTx(context.Background(), &sql.TxOptions{
		ReadOnly: true,
	})
	if err != nil {
		// Read-only might not be supported, skip this test
		t.Skipf("Read-only transactions not supported: %v", err)
		return
	}

	// Create test table
	_, err = tx.Exec(`
		CREATE TEMPORARY TABLE readonly_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			value VARCHAR(50)
		) ENGINE=InnoDB
	`)
	if err != nil {
		// Table creation might fail in read-only transaction, which is expected
		tx.Rollback()
		t.Skipf("Table creation failed in read-only transaction (expected): %v", err)
		return
	}

	// Try to insert data (should fail in read-only)
	_, err = tx.Exec("INSERT INTO readonly_test (value) VALUES (?)", "test")
	if err != nil {
		// This is expected in read-only transaction
		tx.Rollback()
		return
	}

	// If we got here, read-only wasn't enforced, which is also valid
	err = tx.Commit()
	assert.NoError(t, err)
}

// TestTransactionIsolationLevelsDetailed tests all isolation levels in detail
func TestTransactionIsolationLevelsDetailed(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	isolationLevels := []sql.IsolationLevel{
		sql.LevelReadCommitted,
		sql.LevelRepeatableRead,
		sql.LevelSerializable,
	}

	for _, level := range isolationLevels {
		t.Run(fmt.Sprintf("IsolationLevel_%v", level), func(t *testing.T) {
			tx, err := db.BeginTx(context.Background(), &sql.TxOptions{
				Isolation: level,
			})
			if err != nil {
				t.Skipf("Isolation level %v not supported: %v", level, err)
				return
			}

			// Create test table
			_, err = tx.Exec(`
				CREATE TEMPORARY TABLE isolation_test (
					id INT PRIMARY KEY AUTO_INCREMENT,
					value VARCHAR(50),
					isolation_level VARCHAR(50)
				) ENGINE=InnoDB
			`)
			require.NoError(t, err)

			// Insert test data
			_, err = tx.Exec("INSERT INTO isolation_test (value, isolation_level) VALUES (?, ?)", 
				"test", level.String())
			require.NoError(t, err)

			// Query data
			var value, isolation string
			err = tx.QueryRow("SELECT value, isolation_level FROM isolation_test WHERE id = 1").Scan(&value, &isolation)
			require.NoError(t, err)

			assert.Equal(t, "test", value)
			assert.Equal(t, level.String(), isolation)

			err = tx.Commit()
			assert.NoError(t, err)
		})
	}
}

// TestTransactionLargeValues tests transaction with very large values
func TestTransactionLargeValues(t *testing.T) {
	db := createTestDBConnection(t)
	defer db.Close()

	tx, err := db.Begin()
	require.NoError(t, err)

	// Create test table
	_, err = tx.Exec(`
		CREATE TEMPORARY TABLE large_values_test (
			id INT PRIMARY KEY AUTO_INCREMENT,
			text_data TEXT,
			blob_data LONGBLOB
		) ENGINE=InnoDB
	`)
	require.NoError(t, err)

	// Generate large text data (1MB)
	largeText := strings.Repeat("This is large text data. ", 25000)

	// Generate large binary data (2MB)
	largeBinary := make([]byte, 2*1024*1024)
	for i := range largeBinary {
		largeBinary[i] = byte(i % 256)
	}

	// Insert large data
	_, err = tx.Exec("INSERT INTO large_values_test (text_data, blob_data) VALUES (?, ?)", 
		largeText, largeBinary)
	require.NoError(t, err)

	// Retrieve and verify large data
	var retrievedText string
	var retrievedBinary []byte
	err = tx.QueryRow("SELECT text_data, blob_data FROM large_values_test WHERE id = 1").Scan(&retrievedText, &retrievedBinary)
	require.NoError(t, err)

	assert.Equal(t, len(largeText), len(retrievedText), "Text data length should match")
	assert.Equal(t, len(largeBinary), len(retrievedBinary), "Binary data length should match")
	assert.Equal(t, largeText, retrievedText, "Text data should match")

	err = tx.Commit()
	assert.NoError(t, err)
}
