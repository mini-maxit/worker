package repositories

import (
	"github.com/mini-maxit/worker/models"
	"gorm.io/gorm"
)

// Create a new solution in the database marked as processing
func CreateSolution(tx *gorm.DB, solution *models.Solution) error {
	solution.Status = "processing"
	err := tx.Create(solution).Error
	return err
}

// Mark the solution as complete
func MarkSolutionComplete(tx *gorm.DB, solution_id int) error {
	return tx.Model(&models.Solution{}).Where("id = ?", solution_id).
		Update("status", "complete").Update("checked_at", gorm.Expr("NOW()")).Error
}
