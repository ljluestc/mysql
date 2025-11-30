package main

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	// Database connection configuration
	// Using MariaDB/MySQL with default installation
	dsn := "root:rootpassword@tcp(127.0.0.1:3306)/mysql?parseTime=true"
	
	// Connect to database
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Test connection
	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}
	
	fmt.Println("✅ Successfully connected to MariaDB/MySQL using go-sql-driver/mysql!")

	// Create test database if it doesn't exist
	if err := createTestDatabase(db); err != nil {
		log.Fatalf("Failed to create test database: %v", err)
	}

	// Switch to test database
	db.Close()
	dsn = "root:rootpassword@tcp(127.0.0.1:3306)/testdb?parseTime=true"
	db, err = sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("Failed to reconnect to test database: %v", err)
	}
	defer db.Close()

	// Run comprehensive tests
	fmt.Println("\n🧪 Running comprehensive database tests...")
	
	testBasicCRUD(db)
	testTransactions(db)
	testPreparedStatements(db)
	testConnectionPooling(db)
	testErrorHandling(db)
	testMySQLSpecificFeatures(db)
	
	fmt.Println("\n✅ All tests completed successfully!")
}

func createTestDatabase(db *sql.DB) error {
	fmt.Println("📝 Creating test database...")
	
	// Drop database if exists
	_, err := db.Exec("DROP DATABASE IF EXISTS testdb")
	if err != nil {
		return fmt.Errorf("failed to drop existing database: %w", err)
	}
	
	// Create new database
	_, err = db.Exec("CREATE DATABASE testdb")
	if err != nil {
		return fmt.Errorf("failed to create database: %w", err)
	}
	
	fmt.Println("✅ Test database created successfully")
	return nil
}

func testBasicCRUD(db *sql.DB) {
	fmt.Println("\n🔍 Testing Basic CRUD Operations...")
	
	// Create table with MySQL-specific features
	createTableSQL := `
		CREATE TABLE users (
			id INT AUTO_INCREMENT PRIMARY KEY,
			name VARCHAR(100) NOT NULL,
			email VARCHAR(100) UNIQUE NOT NULL,
			age INT,
			salary DECIMAL(10,2),
			profile JSON,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`
	
	if _, err := db.Exec(createTableSQL); err != nil {
		log.Fatalf("Failed to create table: %v", err)
	}
	fmt.Println("✅ Table created with MySQL-specific features")
	
	// Insert data with JSON
	insertSQL := "INSERT INTO users (name, email, age, salary, profile) VALUES (?, ?, ?, ?, ?)"
	profile := `{"skills": ["Go", "MySQL", "Docker"], "experience": 5}`
	
	result, err := db.Exec(insertSQL, "John Doe", "john@example.com", 30, 75000.50, profile)
	if err != nil {
		log.Fatalf("Failed to insert data: %v", err)
	}
	
	id, err := result.LastInsertId()
	if err != nil {
		log.Fatalf("Failed to get last insert ID: %v", err)
	}
	fmt.Printf("✅ Inserted user with ID: %d\n", id)
	
	// Insert more users
	users := []struct{
		name, email, profile string
		age int
		salary float64
	}{
		{"Jane Smith", "jane@example.com", `{"skills": ["Python", "PostgreSQL"], "experience": 3}`, 25, 65000.00},
		{"Bob Johnson", "bob@example.com", `{"skills": ["Java", "Oracle"], "experience": 8}`, 35, 85000.75},
		{"Alice Brown", "alice@example.com", `{"skills": ["JavaScript", "MongoDB"], "experience": 4}`, 28, 70000.25},
	}
	
	for _, user := range users {
		_, err := db.Exec(insertSQL, user.name, user.email, user.age, user.salary, user.profile)
		if err != nil {
			log.Fatalf("Failed to insert user %s: %v", user.name, err)
		}
	}
	fmt.Println("✅ Multiple users inserted successfully")
	
	// Query with MySQL-specific functions
	rows, err := db.Query(`
		SELECT id, name, email, age, salary, 
		       JSON_EXTRACT(profile, '$.skills') as skills,
		       created_at
		FROM users 
		WHERE age > ? AND salary > ?
		ORDER BY salary DESC`, 25, 66000)
	if err != nil {
		log.Fatalf("Failed to query data: %v", err)
	}
	defer rows.Close()
	
	fmt.Println("📋 Users with high salary:")
	for rows.Next() {
		var id int
		var name, email, skills string
		var age int
		var salary float64
		var createdAt time.Time
		
		if err := rows.Scan(&id, &name, &email, &age, &salary, &skills, &createdAt); err != nil {
			log.Fatalf("Failed to scan row: %v", err)
		}
		fmt.Printf("  ID: %d, Name: %s, Email: %s, Age: %d, Salary: $%.2f, Skills: %s, Created: %s\n", 
			id, name, email, age, salary, skills, createdAt.Format("2006-01-02 15:04:05"))
	}
	
	// Update with MySQL function
	updateSQL := "UPDATE users SET salary = salary * 1.1 WHERE name = ?"
	result, err = db.Exec(updateSQL, "John Doe")
	if err != nil {
		log.Fatalf("Failed to update data: %v", err)
	}
	
	affected, err := result.RowsAffected()
	if err != nil {
		log.Fatalf("Failed to get affected rows: %v", err)
	}
	fmt.Printf("✅ Updated %d rows with 10%% salary increase\n", affected)
}

func testTransactions(db *sql.DB) {
	fmt.Println("\n💳 Testing Transactions...")
	
	// Begin transaction with isolation level
	tx, err := db.BeginTx(nil, &sql.TxOptions{
		Isolation: sql.LevelRepeatableRead,
		ReadOnly:  false,
	})
	if err != nil {
		log.Fatalf("Failed to begin transaction: %v", err)
	}
	
	// Insert records within transaction
	insertSQL := "INSERT INTO users (name, email, age, salary, profile) VALUES (?, ?, ?, ?, ?)"
	
	_, err = tx.Exec(insertSQL, "Transaction User 1", "tx1@example.com", 40, 90000.00, `{"type": "transaction"}`)
	if err != nil {
		tx.Rollback()
		log.Fatalf("Failed to insert in transaction: %v", err)
	}
	
	_, err = tx.Exec(insertSQL, "Transaction User 2", "tx2@example.com", 45, 95000.00, `{"type": "transaction"}`)
	if err != nil {
		tx.Rollback()
		log.Fatalf("Failed to insert in transaction: %v", err)
	}
	
	// Commit transaction
	if err := tx.Commit(); err != nil {
		log.Fatalf("Failed to commit transaction: %v", err)
	}
	
	fmt.Println("✅ Transaction with isolation level committed successfully")
}

func testPreparedStatements(db *sql.DB) {
	fmt.Println("\n🎯 Testing Prepared Statements...")
	
	// Prepare statement for complex query
	stmt, err := db.Prepare(`
		SELECT name, age, salary, 
		       JSON_EXTRACT(profile, '$.experience') as experience
		FROM users 
		WHERE age > ? AND salary > ?
		ORDER BY experience DESC`)
	if err != nil {
		log.Fatalf("Failed to prepare statement: %v", err)
	}
	defer stmt.Close()
	
	// Execute prepared statement multiple times
	ageThresholds := []int{25, 30, 35}
	salaryThreshold := 70000.0
	
	for _, age := range ageThresholds {
		rows, err := stmt.Query(age, salaryThreshold)
		if err != nil {
			log.Fatalf("Failed to execute prepared statement: %v", err)
		}
		
		var users []string
		for rows.Next() {
			var name string
			var userAge, experience int
			var salary float64
			
			if err := rows.Scan(&name, &userAge, &salary, &experience); err != nil {
				log.Fatalf("Failed to scan prepared statement result: %v", err)
			}
			users = append(users, fmt.Sprintf("%s (age:%d, exp:%dy)", name, userAge, experience))
		}
		rows.Close()
		
		fmt.Printf("✅ Experienced users older than %d: %v\n", age, users)
	}
}

func testConnectionPooling(db *sql.DB) {
	fmt.Println("\n🏊 Testing Connection Pooling...")
	
	// Configure connection pool for MySQL
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(time.Minute * 5)
	
	fmt.Printf("✅ Connection pool configured - Max Open: %d, Max Idle: %d\n", 
		25, 10)
	
	// Simulate concurrent connections
	done := make(chan bool, 10)
	
	for i := 0; i < 10; i++ {
		go func(workerID int) {
			// Each goroutine gets its own connection from the pool
			rows, err := db.Query("SELECT SLEEP(?), CONNECTION_ID(), @@hostname", 1)
			if err != nil {
				log.Printf("Worker %d failed: %v", workerID, err)
				done <- false
				return
			}
			
			if rows.Next() {
				var sleep int
				var connID int
				var hostname string
				rows.Scan(&sleep, &connID, &hostname)
				fmt.Printf("✅ Worker %d used connection ID: %d on host: %s\n", workerID, connID, hostname)
			}
			rows.Close()
			done <- true
		}(i)
	}
	
	// Wait for all workers to complete
	completed := 0
	for i := 0; i < 10; i++ {
		if <-done {
			completed++
		}
	}
	
	stats := db.Stats()
	fmt.Printf("✅ %d/%d workers completed. Pool stats - Open: %d, InUse: %d, Idle: %d\n",
		completed, 10, stats.OpenConnections, stats.InUse, stats.Idle)
}

func testErrorHandling(db *sql.DB) {
	fmt.Println("\n⚠️ Testing Error Handling...")
	
	// Test invalid SQL
	_, err := db.Exec("INVALID MYSQL SYNTAX")
	if err == nil {
		log.Fatalf("Expected error for invalid SQL, but got none")
	}
	fmt.Printf("✅ Invalid SQL properly rejected: %v\n", err)
	
	// Test constraint violation
	_, err = db.Exec("INSERT INTO users (name, email, age, salary, profile) VALUES (?, ?, ?, ?, ?)", 
		"Duplicate User", "john@example.com", 99, 100000.00, "{}")
	if err == nil {
		log.Fatalf("Expected error for duplicate email, but got none")
	}
	fmt.Printf("✅ Constraint violation properly detected: %v\n", err)
	
	// Test query with no results
	var name string
	err = db.QueryRow("SELECT name FROM users WHERE email = ?", "nonexistent@example.com").Scan(&name)
	if err == nil {
		log.Fatalf("Expected error for no results, but got none")
	}
	if err != sql.ErrNoRows {
		log.Fatalf("Expected sql.ErrNoRows, got: %v", err)
	}
	fmt.Println("✅ No results properly handled with sql.ErrNoRows")
}

func testMySQLSpecificFeatures(db *sql.DB) {
	fmt.Println("\n🚀 Testing MySQL-Specific Features...")
	
	// Test MySQL variables
	var version string
	err := db.QueryRow("SELECT VERSION()").Scan(&version)
	if err != nil {
		log.Fatalf("Failed to get MySQL version: %v", err)
	}
	fmt.Printf("✅ MySQL/MariaDB version: %s\n", version)
	
	// Test MySQL system variables
	var maxConnections int
	err = db.QueryRow("SHOW VARIABLES LIKE 'max_connections'").Scan(&maxConnections, &maxConnections)
	if err != nil {
		log.Printf("Warning: Could not get max_connections: %v", err)
	} else {
		fmt.Printf("✅ MySQL max_connections: %d\n", maxConnections)
	}
	
	// Test JSON operations
	_, err = db.Exec(`
		INSERT INTO users (name, email, age, salary, profile) 
		VALUES (?, ?, ?, ?, JSON_OBJECT('test', true, 'timestamp', NOW()))`,
		"JSON Test User", "json@example.com", 33, 80000.00)
	if err != nil {
		log.Fatalf("Failed to insert JSON user: %v", err)
	}
	
	var jsonValue string
	err = db.QueryRow(`
		SELECT JSON_EXTRACT(profile, '$.test') 
		FROM users 
		WHERE email = ?`, "json@example.com").Scan(&jsonValue)
	if err != nil {
		log.Fatalf("Failed to extract JSON: %v", err)
	}
	fmt.Printf("✅ JSON extraction successful: %s\n", jsonValue)
	
	// Test INSERT ... ON DUPLICATE KEY UPDATE
	_, err = db.Exec(`
		INSERT INTO users (name, email, age, salary) 
		VALUES (?, ?, ?, ?) 
		ON DUPLICATE KEY UPDATE 
		salary = VALUES(salary), 
		updated_at = CURRENT_TIMESTAMP`,
		"John Doe", "john@example.com", 31, 82500.00)
	if err != nil {
		log.Fatalf("Failed to execute UPSERT: %v", err)
	}
	fmt.Println("✅ INSERT ... ON DUPLICATE KEY UPDATE successful")
	
	// Test stored procedure creation and execution
	_, err = db.Exec(`
		CREATE PROCEDURE GetUserCount(IN min_age INT)
		BEGIN
			SELECT COUNT(*) FROM users WHERE age >= min_age;
		END`)
	if err != nil {
		log.Printf("Warning: Could not create stored procedure: %v", err)
	} else {
		var userCount int
		err = db.QueryRow("CALL GetUserCount(?)", 30).Scan(&userCount)
		if err != nil {
			log.Printf("Warning: Could not call stored procedure: %v", err)
		} else {
			fmt.Printf("✅ Stored procedure executed successfully: %d users aged 30+\n", userCount)
		}
		
		// Clean up stored procedure
		db.Exec("DROP PROCEDURE GetUserCount")
	}
}
