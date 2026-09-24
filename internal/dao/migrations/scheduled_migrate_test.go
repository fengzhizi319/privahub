package migration

import (
	"testing"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestMigrateScheduledData_LegacyStatuses(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	// No table yet: no-op.
	if err := MigrateScheduledData(db); err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.ProjectScheduleTaskDO{}, &model.ProjectScheduleDO{}, &model.ProjectScheduleJobDO{}); err != nil {
		t.Fatal(err)
	}
	in := map[string]string{
		"t1": "SUCCESS", "t2": "PAUSED", "t3": "OFFLINE", "t4": "TO_BE_RUN",
		"t5": "RUNNING", "t6": "FAILED", "t7": "Succeed",
	}
	for id, st := range in {
		if err := db.Create(&model.ProjectScheduleTaskDO{ProjectID: "p", GraphID: "g", ScheduleTaskID: id, Cron: "c", ScheduleTaskExpectStart: time.Now(), Status: st}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := MigrateScheduledData(db); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"t1": "SUCCEED", "t2": "STOPPED", "t3": "STOPPED", "t4": "TO_BE_RUN",
		"t5": "RUNNING", "t6": "FAILED", "t7": "SUCCEED",
	}
	for id, w := range want {
		var tk model.ProjectScheduleTaskDO
		db.Where("schedule_task_id = ?", id).First(&tk)
		if tk.Status != w {
			t.Errorf("%s: got %s want %s", id, tk.Status, w)
		}
	}
	// Idempotent.
	if err := MigrateScheduledData(db); err != nil {
		t.Fatal(err)
	}
}
