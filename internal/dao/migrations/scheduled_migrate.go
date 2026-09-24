package migration

import (
	"github.com/fengzhizi319/privahub/internal/dao/model"
	"gorm.io/gorm"
)

// legacyScheduledStatus maps status values written by the old in-memory cron
// implementation to the Java ScheduledStatus enum names.
var legacyScheduledStatus = map[string]string{
	"SUCCESS":   model.ScheduledStatusSucceed,
	"SUCCEEDED": model.ScheduledStatusSucceed,
	"Succeed":   model.ScheduledStatusSucceed,
	"Running":   model.ScheduledStatusRunning,
	"Failed":    model.ScheduledStatusFailed,
	"Stopped":   model.ScheduledStatusStopped,
	"Stopping":  model.ScheduledStatusStopping,
	"PAUSED":    model.ScheduledStatusStopped,
	"OFFLINE":   model.ScheduledStatusStopped,
}

// MigrateScheduledData normalizes legacy project_schedule_task status values.
// It is idempotent and safe to run on every start-up.
func MigrateScheduledData(db *gorm.DB) error {
	if !db.Migrator().HasTable(&model.ProjectScheduleTaskDO{}) {
		return nil
	}
	for from, to := range legacyScheduledStatus {
		if err := db.Model(&model.ProjectScheduleTaskDO{}).
			Where("status = ?", from).
			Update("status", to).Error; err != nil {
			return err
		}
	}
	return nil
}
