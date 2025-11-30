#!/bin/bash

# Quick Database Setup Script for MySQL/MariaDB Tests
# This script sets up the test database and user for the transaction tests

echo "🚀 Quick Database Setup for MySQL/MariaDB Tests"
echo "=============================================="

# Check if MariaDB/MySQL is running
if ! systemctl is-active --quiet mariadb 2>/dev/null && ! systemctl is-active --quiet mysql 2>/dev/null; then
    echo "❌ MariaDB/MySQL is not running"
    echo "Please start it with:"
    echo "  sudo systemctl start mariadb"
    echo "  sudo systemctl enable mariadb"
    exit 1
fi

echo "✅ MariaDB/MySQL is running"

# Try to connect and create database
echo "📝 Setting up test database..."

# Try without password first (for fresh installations)
if mysql -u root -e "SELECT 1;" >/dev/null 2>&1; then
    echo "🔓 Connecting as root without password..."
    mysql -u root -e "
        CREATE DATABASE IF NOT EXISTS testdb;
        CREATE USER IF NOT EXISTS 'testuser'@'localhost' IDENTIFIED BY 'testpass';
        GRANT ALL PRIVILEGES ON testdb.* TO 'testuser'@'localhost';
        FLUSH PRIVILEGES;
        SELECT 'Database setup completed successfully' as status;
    "
elif mysql -u root -p'' -e "SELECT 1;" >/dev/null 2>&1; then
    echo "🔓 Connecting as root with empty password..."
    mysql -u root -p'' -e "
        CREATE DATABASE IF NOT EXISTS testdb;
        CREATE USER IF NOT EXISTS 'testuser'@'localhost' IDENTIFIED BY 'testpass';
        GRANT ALL PRIVILEGES ON testdb.* TO 'testuser'@'localhost';
        FLUSH PRIVILEGES;
        SELECT 'Database setup completed successfully' as status;
    "
else
    echo "❌ Cannot connect to MySQL/MariaDB as root"
    echo "Please run manually:"
    echo "  sudo mysql -e \""
    echo "    CREATE DATABASE IF NOT EXISTS testdb;"
    echo "    CREATE USER IF NOT EXISTS 'testuser'@'localhost' IDENTIFIED BY 'testpass';"
    echo "    GRANT ALL PRIVILEGES ON testdb.* TO 'testuser'@'localhost';"
    echo "    FLUSH PRIVILEGES;\""
    exit 1
fi

# Test the connection
echo "🧪 Testing database connection..."
if mysql -u testuser -ptestpass testdb -e "SELECT 'Test connection successful' as status;" 2>/dev/null; then
    echo "✅ Database connection test passed!"
    echo ""
    echo "🎯 Ready to run transaction tests:"
    echo "  go test -v -run TestTransactionPureUnitTests    # Pure unit tests (no DB needed)"
    echo "  go test -v -run TestTransaction                # All transaction tests"
    echo "  go test -bench=BenchmarkTransaction -benchmem  # Performance benchmarks"
else
    echo "❌ Database connection test failed"
    exit 1
fi
