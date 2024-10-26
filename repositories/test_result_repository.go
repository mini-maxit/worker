package repositories

import (
	"github.com/mini-maxit/worker/models"
	"gorm.io/gorm"
)


func CreateTestResults(tx *gorm.DB, testResult models.TestResult)  error {
	err := tx.Create(&testResult).Error
	return err
}
