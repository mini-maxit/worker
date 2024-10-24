package repositories

import (
	"github.com/mini-maxit/worker/models"
	"gorm.io/gorm"
)

// Store the result of the solution in the database
func CreateUserSolutionResult(tx *gorm.DB, solutionResult models.UserSolutionResult) (uint, error) {
	if err := tx.Create(&solutionResult).Error; err != nil {
		return 0, err
	}
	return solutionResult.ID, nil
}
