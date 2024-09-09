package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/joho/godotenv/autoload"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Service represents a service that interacts with a database.
type Service interface {
	// Health returns a map of health status information.
	// The keys and values in the map are service-specific.
	Health() map[string]string

	// Close terminates the database connection.
	// It returns an error if the connection cannot be closed.
	Close() error

	// GetUsers returns a list of users from the database.
	GetUsers() []User

	// GetUser returns a user from the database.
	GetUser(id int) User

	// CreateUser creates a new user in the database.
	CreateUser(user User) error

	// GetUserByEmail returns a user from the database based on the email.
	GetUserByEmail(email string) User
}

type service struct {
	db     *gorm.DB
	baseDB *sql.DB
}

var (
	database   = os.Getenv("DB_DATABASE")
	password   = os.Getenv("DB_PASSWORD")
	username   = os.Getenv("DB_USERNAME")
	port       = os.Getenv("DB_PORT")
	host       = os.Getenv("DB_HOST")
	schema     = os.Getenv("DB_SCHEMA")
	dbInstance *service
)

func Get() service {
	return *dbInstance
}

func New() Service {
	// Reuse Connection
	if dbInstance != nil {
		return dbInstance
	}
	connStr := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable&search_path=%s", username, password, host, port, database, schema)
	db, err := sql.Open("pgx", connStr)

	if err != nil {
		log.Fatal(err)
	}

	gormDB, err := gorm.Open(postgres.New(postgres.Config{
		Conn: db,
	}), &gorm.Config{})

	if err != nil {
		log.Fatal("Error connecting with gorm: ", err)
	}

	err = gormDB.AutoMigrate(&User{}, &BankAccount{}, &Transaction{}, &Budget{}, &Notification{})
	if err != nil {
		log.Fatal("Error with migration: ", err)
	}

	dbInstance = &service{
		db:     gormDB,
		baseDB: db,
	}
	return dbInstance
}

// Health checks the health of the database connection by pinging the database.
// It returns a map with keys indicating various health statistics.
func (s *service) Health() map[string]string {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	stats := make(map[string]string)

	// Ping the database
	err := s.baseDB.PingContext(ctx)
	if err != nil {
		stats["status"] = "down"
		stats["error"] = fmt.Sprintf("db down: %v", err)
		log.Fatalf("db down: %v", err) // Log the error and terminate the program
		return stats
	}

	// Database is up, add more statistics
	stats["status"] = "up"
	stats["message"] = "It's healthy"

	// Get database stats (like open connections, in use, idle, etc.)
	dbStats := s.baseDB.Stats()
	stats["open_connections"] = strconv.Itoa(dbStats.OpenConnections)
	stats["in_use"] = strconv.Itoa(dbStats.InUse)
	stats["idle"] = strconv.Itoa(dbStats.Idle)
	stats["wait_count"] = strconv.FormatInt(dbStats.WaitCount, 10)
	stats["wait_duration"] = dbStats.WaitDuration.String()
	stats["max_idle_closed"] = strconv.FormatInt(dbStats.MaxIdleClosed, 10)
	stats["max_lifetime_closed"] = strconv.FormatInt(dbStats.MaxLifetimeClosed, 10)

	// Evaluate stats to provide a health message
	if dbStats.OpenConnections > 40 { // Assuming 50 is the max for this example
		stats["message"] = "The database is experiencing heavy load."
	}

	if dbStats.WaitCount > 1000 {
		stats["message"] = "The database has a high number of wait events, indicating potential bottlenecks."
	}

	if dbStats.MaxIdleClosed > int64(dbStats.OpenConnections)/2 {
		stats["message"] = "Many idle connections are being closed, consider revising the connection pool settings."
	}

	if dbStats.MaxLifetimeClosed > int64(dbStats.OpenConnections)/2 {
		stats["message"] = "Many connections are being closed due to max lifetime, consider increasing max lifetime or revising the connection usage pattern."
	}

	return stats
}

// Close closes the database connection.
// It logs a message indicating the disconnection from the specific database.
// If the connection is successfully closed, it returns nil.
// If an error occurs while closing the connection, it returns the error.
func (s *service) Close() error {
	log.Printf("Disconnected from database: %s", database)
	return s.baseDB.Close()
}

func (s *service) Migrate() error {
	err := s.db.AutoMigrate(&User{}, &BankAccount{}, &Transaction{}, &Budget{}, &Notification{})

	if err != nil {
		return err
	}

	return nil
}

func (s *service) GetUsers() []User {
	var users []User
	s.db.Find(&users)
	return users
}

func (s *service) GetUser(id int) User {
	var user User
	s.db.First(&user, id)
	return user
}

func (s *service) CreateUser(user User) error {
	// Check if user already exists
	var existingUser User
	result := s.db.Where("email = ?", user.Email).First(&existingUser)
	if result.RowsAffected > 0 {
		return fmt.Errorf("user with email %s already exists", user.Email)
	}
	// Create the new user
	s.db.Create(&user)
	return nil
}

func (s *service) GetUserByEmail(email string) User {
	var user User
	s.db.Where("email = ?", email).First(&user)
	return user
}

func (s *service) CreateTransaction(transaction *Transaction) error {
	s.db.Create(transaction)
	if s.db.Error != nil {
		return s.db.Error
	}
	return nil
}

func (s *service) GetTransactions(user *User) []Transaction {
	var transactions []Transaction
	s.db.Where("user_id = ?", user.ID).Find(&transactions)

	if s.db.Error != nil {
		log.Println("Error fetching transactions: ", s.db.Error)
		return nil
	}
	return transactions
}

func (s *service) GetTransactionByID(id string) Transaction {
	var transaction Transaction
	s.db.First(&transaction, id)

	if s.db.Error != nil {
		log.Println("Error fetching transaction: ", s.db.Error)
		return Transaction{}
	}
	return transaction
}
