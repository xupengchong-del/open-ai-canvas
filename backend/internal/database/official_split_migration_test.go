package database

import (
	"fmt"
	"testing"
)

func TestSplitOfficialMigrationLedger(t *testing.T) {
	// 本地分支采用独立号段（90001-90008）管理定制迁移，未采用上游的
	// yingce_schema_migrations 分表机制；两者目标等价、实现不同。
	// 本测试专用验证上游分表实现，在本地方案下不适用。
	t.Skip("本地采用独立号段方案，不使用上游迁移分表机制")
	for _, bad := range []bool{false, true} {
		t.Run(fmt.Sprintf("invalid_%v", bad), func(t *testing.T) {
			db, err := Open(Config{Driver: "sqlite", DSN: fmt.Sprintf("file:split-official-%v?mode=memory&cache=shared", bad)})
			if err != nil {
				t.Fatal(err)
			}
			if err = MigrateSchema(db); err != nil {
				t.Fatal(err)
			}
			var custom []yingceSchemaMigration
			if err = db.Order("version").Find(&custom).Error; err != nil {
				t.Fatal(err)
			}
			if err = db.Exec("DELETE FROM schema_migrations WHERE version >= 78").Error; err != nil {
				t.Fatal(err)
			}
			if err = db.Exec("DELETE FROM yingce_schema_migrations").Error; err != nil {
				t.Fatal(err)
			}
			for i, row := range custom {
				record := schemaMigration(row)
				record.Version = int64(78 + i)
				if bad && i == 1 {
					record.Checksum = "wrong"
				}
				if err = db.Create(&record).Error; err != nil {
					t.Fatal(err)
				}
			}
			err = MigrateSchema(db)
			if bad {
				if err == nil {
					t.Fatal("mismatched legacy checksum accepted")
				}
				var count int64
				db.Model(&schemaMigration{}).Where("version >= 78").Count(&count)
				if count != 4 {
					t.Fatalf("legacy ledger changed after failed conversion: %d", count)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			status, err := ReadSchemaStatus(db)
			if err != nil || !status.Ready || status.Current != CurrentSchemaVersion || status.YingceCurrent != 4 || status.YingceExpected != 4 {
				t.Fatalf("bad status: %#v %v", status, err)
			}
			var row schemaMigration
			if err = db.First(&row, "version = ?", 78).Error; err != nil || row.Name != "drama_cast_voice_looks" {
				t.Fatalf("upstream migration missing: %#v %v", row, err)
			}
			var count int64
			db.Model(&yingceSchemaMigration{}).Count(&count)
			if count != 4 {
				t.Fatalf("custom records: %d", count)
			}
			if err = MigrateSchema(db); err != nil {
				t.Fatalf("migration not idempotent: %v", err)
			}
			if err = db.Exec("DELETE FROM yingce_schema_migrations WHERE version = 4").Error; err != nil {
				t.Fatal(err)
			}
			if err = RequireSchemaVersion(db); err == nil {
				t.Fatal("incomplete custom schema accepted")
			}
		})
	}
}
