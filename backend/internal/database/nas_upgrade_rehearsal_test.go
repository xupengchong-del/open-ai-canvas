package database

import (
	"path/filepath"
	"strconv"
	"os"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// 端到端演练：把 NAS 真实的 schema_migrations 记录灌进空库，再跑完整 MigrateSchema。
// 验证「部署合并版到已有 NAS 库」时归一化、官方迁移补跑、版本就绪整条链路。
// 数据来自 .local/merge-conflicts/nas-migrations-snapshot.txt（从 NAS 导出）。
func TestRehearseUpgradeFromNASSchemaMigrations(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", ".local", "merge-conflicts", "nas-migrations-snapshot.txt"))
	if err != nil {
		t.Skipf("没有 NAS 迁移快照，跳过演练：%v", err)
	}
	db, err := gorm.Open(sqlite.Open("file:nas-upgrade-rehearsal?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	// 真实 NAS 库里 v1（baseline）已建好全部基础业务表；空库演练会误报「no such table」。
	if err := db.AutoMigrate(Models()...); err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&schemaMigration{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, "|") {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) != 3 {
			continue
		}
		var version int64
		if err := parseVersion(parts[0], &version); err != nil {
			t.Fatalf("解析版本号 %q：%v", parts[0], err)
		}
		row := schemaMigration{Version: version, Name: parts[1], Checksum: parts[2], AppliedAt: now}
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := normalizeLocalExtensionMigrations(db); err != nil {
		t.Fatalf("归一化失败：%v", err)
	}
	// 归一化后：官方 v61-v92 应无记录（待执行），本机 v93-v100 应在
	var oldCount int64
	db.Model(&schemaMigration{}).Where("version BETWEEN 61 AND 92").Count(&oldCount)
	if oldCount != 0 {
		t.Fatalf("归一化后 v61-v92 仍有 %d 条记录", oldCount)
	}
	var newCount int64
	db.Model(&schemaMigration{}).Where("version BETWEEN 102 AND 109").Count(&newCount)
	if newCount != 8 {
		t.Fatalf("归一化后 v93-v100 有 %d 条记录，期望 8", newCount)
	}

	// 完整迁移：会执行官方 v61-v92（AutoMigrate/带守卫 AddColumn），本机 v93-v100 校验后跳过
	if err := MigrateSchema(db); err != nil {
		t.Fatalf("完整迁移失败：%v", err)
	}
	status, err := ReadSchemaStatus(db)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Ready || status.Current != CurrentSchemaVersion {
		t.Fatalf("迁移后状态异常：%#v", status)
	}
	// v93-v100 的 name/checksum 必须与代码定义一致（否则这里会报校验错误）
	for offset, name := range localExtensionMigrationNames {
		var row schemaMigration
		if err := db.First(&row, "version = ?", 102+int64(offset)).Error; err != nil {
			t.Fatal(err)
		}
		if row.Name != name {
			t.Fatalf("v%d 名称 = %s，期望 %s", 93+offset, row.Name, name)
		}
	}
}

func parseVersion(text string, out *int64) error {
	v, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
	if err != nil {
		return err
	}
	*out = v
	return nil
}
