#!/bin/bash

# Simple Test Runner for MySQL Transaction Tests
# This script sets up the environment and runs tests

set -e

echo "🚀 MySQL Transaction Test Setup"
echo "==============================="

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# MySQL configuration
MYSQL_DATABASE="mysql_test"
MYSQL_USER="testuser"
MYSQL_PASSWORD="testpass"

print_status "Setting up test environment..."

# Install Go dependencies
print_status "Installing Go dependencies..."
go get github.com/stretchr/testify/assert
go get github.com/stretchr/testify/require

print_success "Dependencies installed!"

# Check MySQL connection
print_status "Checking MySQL connection..."

# Try to connect to MySQL
if mysqladmin ping -h localhost --silent 2>/dev/null; then
    print_success "MySQL is running!"
    
    # Set up database and user
    print_status "Setting up test database and user..."
    
    # Create database
    mysql -u root -e "CREATE DATABASE IF NOT EXISTS $MYSQL_DATABASE;" 2>/dev/null || {
        print_warning "Please enter MySQL root password to create database:"
        mysql -u root -p -e "CREATE DATABASE IF NOT EXISTS $MYSQL_DATABASE;"
    }
    
    # Create user
    mysql -u root -e "CREATE USER IF NOT EXISTS '$MYSQL_USER'@'localhost' IDENTIFIED BY '$MYSQL_PASSWORD';" 2>/dev/null || {
        mysql -u root -p -e "CREATE USER IF NOT EXISTS '$MYSQL_USER'@'localhost' IDENTIFIED BY '$MYSQL_PASSWORD';"
    }
    
    # Grant privileges
    mysql -u root -e "GRANT ALL PRIVILEGES ON $MYSQL_DATABASE.* TO '$MYSQL_USER'@'localhost';" 2>/dev/null || {
        mysql -u root -p -e "GRANT ALL PRIVILEGES ON $MYSQL_DATABASE.* TO '$MYSQL_USER'@'localhost';"
    }
    
    # Flush privileges
    mysql -u root -e "FLUSH PRIVILEGES;" 2>/dev/null || {
        mysql -u root -p -e "FLUSH PRIVILEGES;"
    }
    
    print_success "Database setup complete!"
    
    # Test connection
    if mysql -u $MYSQL_USER -p$MYSQL_PASSWORD -e "SELECT 'Connection successful!' as status;" $MYSQL_DATABASE 2>/dev/null; then
        print_success "Database connection verified!"
        
        # Set environment variables
        export TEST_DSN="$MYSQL_USER:$MYSQL_PASSWORD@tcp(localhost:3306)/$MYSQL_DATABASE?parseTime=true&timeout=30s&readTimeout=30s&writeTimeout=30s"
        
        print_status "Environment variable set: TEST_DSN=$TEST_DSN"
        
        echo
        echo "🧪 Running Tests"
        echo "================"
        
        # Run unit tests
        print_status "Running unit tests..."
        go test -v -timeout=60s ./...
        
        print_success "All tests completed!"
        
    else
        print_error "Database connection failed!"
        exit 1
    fi
    
else
    print_error "MySQL is not running!"
    echo
    echo "Please start MySQL and run this script again:"
    echo
    echo "On Ubuntu/Debian:"
    echo "  sudo systemctl start mysql"
    echo "  sudo systemctl enable mysql"
    echo
    echo "On CentOS/RHEL:"
    echo "  sudo systemctl start mysqld"
    echo "  sudo systemctl enable mysqld"
    echo
    echo "On macOS (with Homebrew):"
    echo "  brew services start mysql"
    echo
    echo "After starting MySQL, run this script again:"
    echo "  ./setup_and_run_tests.sh"
    echo
    exit 1
fi

echo
echo "🎉 Setup Complete!"
echo "==================="
print_success "MySQL transaction tests are ready to run!"

echo
echo "Quick test commands:"
echo "  go test -v -run TestTransaction"
echo "  go test -v -tags=integration"
echo "  go test -bench=BenchmarkTransaction"
