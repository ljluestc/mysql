package main

import (
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	// Try different connection strings
	connectionStrings := []string{
		"testuser:testpass@unix(/var/run/mysqld/mysqld.sock)/mysql",
		"testuser:testpass@tcp(127.0.0.1:3306)/mysql",
		"testuser:testpass@/mysql",
		"root:rootpassword@unix(/var/run/mysqld/mysqld.sock)/mysql",
		"root:rootpassword@tcp(127.0.0.1:3306)/mysql",
	}

	for i, dsn := range connectionStrings {
		fmt.Printf("Trying connection string %d: %s\n", i+1, dsn)
		
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			fmt.Printf("  ❌ Failed to open: %v\n", err)
			continue
		}
		
		err = db.Ping()
		if err != nil {
			fmt.Printf("  ❌ Failed to ping: %v\n", err)
			db.Close()
			continue
		}
		
		fmt.Printf("  ✅ Connected successfully!\n")
		
		// Test a simple query
		var version string
		err = db.QueryRow("SELECT VERSION()").Scan(&version)
		if err != nil {
			fmt.Printf("  ❌ Failed to query version: %v\n", err)
		} else {
			fmt.Printf("  📊 MySQL/MariaDB version: %s\n", version)
		}
		
		db.Close()
		fmt.Println()
	}
}
