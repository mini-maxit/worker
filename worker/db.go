package worker

import (
	"fmt"
	"log"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Connect to the database using GORM
func connectToDatabase() *gorm.DB {
	DATABASE_USER := os.Getenv("DATABASE_USER")
	DATABASE_PASSWORD := os.Getenv("DATABASE_PASSWORD")
	DATABASE_NAME := os.Getenv("DATABASE_NAME")
	DATABASE_SSL_MODE := os.Getenv("DATABASE_SSL_MODE")

	dsn := fmt.Sprintf("host=localhost user=%s password=%s dbname=%s sslmode=%s",
		DATABASE_USER, DATABASE_PASSWORD, DATABASE_NAME, DATABASE_SSL_MODE)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}

	// Migrate the schema if needed
	err = db.AutoMigrate(&Solution{}, &SolutionResult{})
	if err != nil {
		log.Fatalf("failed to migrate database schema: %v", err)
	}

	return db
}

// Create a new solution in the database marked as processing
func createSolution(db *gorm.DB, solution *Solution) error {
	solution.Status = "processing"
	err := db.Create(solution).Error
	return err
}

// Mark the solution as complete
func markSolutionComplete(db *gorm.DB, solution_id int) error {
	return db.Model(&Solution{}).Where("id = ?", solution_id).
		Update("status", "complete").Error
}

// Store the result of the solution in the database
func storeSolutionResult(db *gorm.DB, solutionResult SolutionResult) error {
	return db.Create(&solutionResult).Error
}