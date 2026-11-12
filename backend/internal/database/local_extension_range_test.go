package database

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// 本机扩展迁移必须一直待在独立高位号段，且 CurrentSchemaVersion 等于该号段末尾。
// 合并上游时官方迁移号段会继续增长，这个测试保证两者永不相交。
func TestLocalExtensionMigrationsUseDedicatedRange(t *testing.T) {
	names := map[string]int64{}
	for _, m := range schemaMigrations {
		names[m.name] = m.version
	}
	for i, name := range localExtensionMigrationNames {
		want := localExtensionMigrationBase + 1 + int64(i)
		got, ok := names[name]
		if !ok {
			t.Fatalf("本机迁移 %s 不在迁移计划里", name)
		}
		if got != want {
			t.Fatalf("本机迁移 %s 版本 = %d，期望 %d", name, got, want)
		}
	}
	// 所有非本机迁移都必须严格小于独立号段，避免未来官方号段增长撞车。
	localNames := map[string]bool{}
	for _, name := range localExtensionMigrationNames {
		localNames[name] = true
	}
	for _, m := range schemaMigrations {
		if !localNames[m.name] && m.version >= localExtensionMigrationBase {
			t.Fatalf("官方迁移 %s（v%d）侵入了本机独立号段", m.name, m.version)
		}
	}
	last := schemaMigrations[len(schemaMigrations)-1].version
	if CurrentSchemaVersion != last {
		t.Fatalf("CurrentSchemaVersion=%d 与迁移计划末项 %d 不一致", CurrentSchemaVersion, last)
	}
	wantLast := localExtensionMigrationBase + localExtensionMigrationCount
	if last != wantLast {
		t.Fatalf("迁移计划末项 = %d，期望 %d", last, wantLast)
	}
}

// 模拟官方未来把迁移号段推进到 v160：本机编号、归一化、最终 ready 都不应受影响。
func TestLocalExtensionRangeSurvivesOfficialGrowth(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:local-range-growth?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	// 官方迁移的 apply 会碰业务表，空库演练会误报 no such table；真实库这些表由 baseline 建好。
	if err := db.AutoMigrate(Models()...); err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&schemaMigration{}); err != nil {
		t.Fatal(err)
	}
	stamp := time.Now().UTC()
	// 真实 NAS 历史布局：v1-v60 官方、v61-v68 本机、v69-v101 官方。
	for _, m := range schemaMigrations {
		if m.version > 101 || m.version >= 61 && m.version <= 68 {
			continue
		}
		if err := db.Create(&schemaMigration{Version: m.version, Name: m.name, Checksum: m.checksum, AppliedAt: stamp}).Error; err != nil {
			t.Fatal(err)
		}
	}
	// checksum 必须与迁移计划一致：归一化只搬版本号，不碰校验和。
	checksums := map[string]string{}
	for _, m := range schemaMigrations {
		if m.version >= localExtensionMigrationBase {
			checksums[m.name] = m.checksum
		}
	}
	for i, name := range localExtensionMigrationNames {
		row := schemaMigration{Version: 61 + int64(i), Name: name, Checksum: checksums[name], AppliedAt: stamp}
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	// 人为把官方号段推进到 v160，模拟未来若干次官方升级。
	for v := int64(102); v <= 160; v++ {
		if err := db.Create(&schemaMigration{Version: v, Name: "future_official", Checksum: "sha256:future", AppliedAt: stamp}).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := normalizeLocalExtensionMigrations(db); err != nil {
		t.Fatalf("官方号段增长后归一化失败：%v", err)
	}
	for i, name := range localExtensionMigrationNames {
		var row schemaMigration
		want := localExtensionMigrationBase + 1 + int64(i)
		if err := db.First(&row, "version = ?", want).Error; err != nil {
			t.Fatalf("本机迁移 %s 未搬到 %d：%v", name, want, err)
		}
		if row.Name != name {
			t.Fatalf("v%d 名称 = %s，期望 %s", want, row.Name, name)
		}
	}
	if err := MigrateSchema(db); err != nil {
		t.Fatalf("官方号段增长后完整迁移失败：%v", err)
	}
	status, err := ReadSchemaStatus(db)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Ready || status.Current != CurrentSchemaVersion {
		t.Fatalf("迁移后状态异常：%#v", status)
	}
}
