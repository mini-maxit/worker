package worker

import (
	"fmt"
	"log"

	"github.com/mini-maxit/worker/internal/config"
	"github.com/mini-maxit/worker/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type postgresDataBase struct {
	Db *gorm.DB
}

func Connect(db * postgresDataBase) *gorm.DB {
	return db.Db
}

// Connect to the database using GORM
func NewPostgresDatabase(config config.Config) *postgresDataBase {
	DATABASE_USER := config.DBUser
	DATABASE_PASSWORD := config.DBPassword
	DATABASE_NAME := config.DBName
	DATABASE_SSL_MODE := config.DBSslMode

	dsn := fmt.Sprintf("host=postgres user=%s password=%s dbname=%s sslmode=%s",
		DATABASE_USER, DATABASE_PASSWORD, DATABASE_NAME, DATABASE_SSL_MODE)


	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if(err != nil) {
		log.Fatalf("Failed to connect to database: %s", err)
	}

	err = db.AutoMigrate(&models.InputOutput{}, &models.Task{}, &models.TestResult{}, &models.UserSolution{}, &models.UserSolutionResult{})
	if(err != nil) {
		log.Fatalf("Failed to migrate database: %s", err)
	}

	return &postgresDataBase{Db: db}
}
