package database

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// 模拟 NAS 上合并前的数据库：v1-v68 均已应用，其中 v61-v68 是本机扩展迁移。
// 部署合并后的程序时，MigrateSchema 必须自动后移编号到 v93-v100，并执行官方 v61-v92，
// 最终达到 CurrentSchemaVersion 且不报「名称不一致」。
func TestMigrateSchemaRenumberLocalExtensionMigrations(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:local-extension-renumber?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&schemaMigration{}); err != nil {
		t.Fatal(err)
	}
	// 先跑一遍建立官方 v1-v60 的表结构（避免本机迁移 apply 时找不到表）
	if err := db.Exec("CREATE TABLE IF NOT EXISTS channel_models (id text primary key)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("ALTER TABLE channel_models ADD COLUMN upstream_price_note text").Error; err != nil {
		t.Fatal(err)
	}
	// 造旧布局记录：v1-v60 用占位名（不会被校验，因为归一化后 plan 会重新执行；这里只造 v61-v68）
	now := time.Now().UTC()
	for offset, name := range localExtensionMigrationNames {
		row := schemaMigration{Version: 61 + int64(offset), Name: name, Checksum: "sha256:old-" + name, AppliedAt: now}
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := normalizeLocalExtensionMigrations(db); err != nil {
		t.Fatalf("归一化失败：%v", err)
	}

	// 校验：v61-v68 已清空，v93-v100 存在且 name/checksum 未变
	var oldCount int64
	db.Model(&schemaMigration{}).Where("version BETWEEN 61 AND 68").Count(&oldCount)
	if oldCount != 0 {
		t.Fatalf("旧编号仍残留 %d 条", oldCount)
	}
	for offset, name := range localExtensionMigrationNames {
		var row schemaMigration
		if err := db.First(&row, "version = ?", localExtensionMigrationBase+1+int64(offset)).Error; err != nil {
			t.Fatalf("新编号 %d 缺失：%v", 93+offset, err)
		}
		if row.Name != name || row.Checksum != "sha256:old-"+name {
			t.Fatalf("新编号 %d 的 name/checksum 被改动：%#v", 93+offset, row)
		}
	}

	// 幂等：再跑一次不应报错、不应重复移动
	if err := normalizeLocalExtensionMigrations(db); err != nil {
		t.Fatalf("重复归一化失败：%v", err)
	}
	var newCount int64
	db.Model(&schemaMigration{}).Where("version BETWEEN ? AND ?", localExtensionMigrationBase+1, localExtensionMigrationBase+localExtensionMigrationCount).Count(&newCount)
	if newCount != 8 {
		t.Fatalf("重复归一化后新编号数量 = %d，期望 8", newCount)
	}
}
