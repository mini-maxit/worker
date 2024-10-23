package repositories

import (
	"github.com/mini-maxit/worker/models"
	"gorm.io/gorm"
)

// Store the result of the solution in the database
func StoreSolutionResult(tx *gorm.DB, solutionResult models.SolutionResult) error {
	return tx.Create(&solutionResult).Error
}
