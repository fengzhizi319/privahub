package migration

import (
	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/graphjob"
	"gorm.io/gorm"
)

// MigrateJobStatusData maps legacy Go job / task status values (PENDING,
// SUCCEEDED, IDLE, Kuscia-cased states ...) to the Java GraphJobStatus /
// GraphNodeTaskStatus enum names. It is idempotent and runs on every start-up.
func MigrateJobStatusData(db *gorm.DB) error {
	if db.Migrator().HasTable(&model.ProjectJobDO{}) {
		for from, to := range graphjob.LegacyJobStatusMapping {
			if err := db.Model(&model.ProjectJobDO{}).Where("status = ?", from).Update("status", to).Error; err != nil {
				return err
			}
		}
	}
	if db.Migrator().HasTable(&model.ProjectJobTaskDO{}) {
		for from, to := range graphjob.LegacyTaskStatusMapping {
			if err := db.Model(&model.ProjectJobTaskDO{}).Where("status = ?", from).Update("status", to).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
