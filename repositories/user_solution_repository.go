package repositories

import (
	"log"

	"github.com/mini-maxit/worker/models"
	"gorm.io/gorm"
)

func CreateUserSolution(tx *gorm.DB, userSolution models.UserSolution) (uint, error) {
	err := tx.Create(userSolution).Error
	if err != nil {
		return 0, err
	}
	return userSolution.ID, nil
}

func MarkUserSolutionProcessing(tx *gorm.DB, userSolutionID int) error {
	err := tx.Model(&models.UserSolution{}).Where("id = ?", userSolutionID).Update("status", "processing").Error
	return err
}

func MarkUserSolutionComplete(tx *gorm.DB, userSolutionID uint) error {
	err := tx.Model(&models.UserSolution{}).Where("id = ?", userSolutionID).Update("status", "completed").Error
	return err
}

func MarkUserSolutionFailed(db *gorm.DB, userSolutionID uint, errorMsg error) error {
	stringErrorMsg := errorMsg.Error()
	log.Print(stringErrorMsg, userSolutionID)
	err := db.Model(&models.UserSolution{}).Where("id = ?", userSolutionID).Updates(map[string]interface{}{
		"status":     "failed",
		"status_message": stringErrorMsg,
	}).Error
	return err
}
