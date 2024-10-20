package repositories

import (
	"github.com/mini-maxit/worker/models"
	"gorm.io/gorm"
)

// Create a new solution in the database marked as processing
func CreateSolution(db *gorm.DB, solution *models.Solution) error {
	solution.Status = "processing"
	err := db.Create(solution).Error
	return err
}

// Mark the solution as complete
func MarkSolutionComplete(db *gorm.DB, solution_id int) error {
	return db.Model(&models.Solution{}).Where("id = ?", solution_id).
		Update("status", "complete").Update("checked_at", gorm.Expr("NOW()")).Error
}