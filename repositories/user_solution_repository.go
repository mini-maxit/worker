package repositories

import (
	"github.com/mini-maxit/worker/models"
	"gorm.io/gorm"
)

func CreateUserSolution(tx *gorm.DB, userSolution models.UserSolution) (uint,error) {
	err := tx.Create(userSolution).Error
	if err != nil {
		return 0, err
	}
	return userSolution.ID, nil
}

func MarkUserSolutionComplete(tx *gorm.DB, userSolutionID uint) error {
	err := tx.Model(&models.UserSolution{}).Where("id = ?", userSolutionID).Update("status", "completed").Error
	return err
}

func MarkUserSolutionFailed(tx *gorm.DB, userSolutionID uint, errorMsg error) error {
	err := tx.Model(&models.UserSolution{}).Where("id = ?", userSolutionID).Updates(map[string]interface{}{
		"status":     "failed",
		"status_message": errorMsg,
	}).Error
	return err
}
