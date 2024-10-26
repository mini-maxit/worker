package repositories

import (
	"gorm.io/gorm"
)

func GetInputOutputId(tx *gorm.DB, task_id int, order int) (int, error){
	var input_outpu_id int
	err := tx.Table("input_outputs").Select("id").Where("task_id = ? AND input_output_order = ?", task_id, order).Scan(&input_outpu_id).Error
	return input_outpu_id, err
}
