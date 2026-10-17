#!/usr/bin/env python3
"""把 fetch-nas-models.py 导出的 JSON 回灌进本机 SQLite 开发库。

用法：
    apply-nas-models.py --db .local/project-workbench-debug/open_ai_canvas.db \
        --input models.json [--dry-run]

行为：
  * 只处理 channel_models 表，别的表一律不碰（账号 / 渠道 / OSS 仍由 sync-api 单向推送）。
  * 按 id 判断：NAS 有而本机没有 → 插入；两边都有 → 用 NAS 的配置覆盖本机。
  * enabled / deleted_at 以 NAS 为准：NAS 上已停用或已删除的视频模型，本机会同步成同样状态，
    这样"两边一致"才有意义，而不是只往本机堆新行。
  * --dry-run 只打印将要发生的变更，不写库。
"""

import argparse
import json
import sqlite3
import sys

SCHEMA_VERSION = 1

# NAS 的 camelCase 字段 -> 本机 SQLite 列
FIELD_MAP = {
    "id": "id",
    "channelId": "channel_id",
    "modelKey": "model_key",
    "providerModelKey": "provider_model_key",
    "displayName": "display_name",
    "channelLabel": "channel_label",
    "description": "description",
    "upstreamPriceNote": "upstream_price_note",
    "sortOrder": "sort_order",
    "icon": "icon",
    "capability": "capability",
    "protocol": "protocol",
    "billingMode": "billing_mode",
    "unitPriceMicrocredits": "unit_price_microcredits",
    "inputTokenPriceMicrocredits": "input_token_price_microcredits",
    "outputTokenPriceMicrocredits": "output_token_price_microcredits",
    "cachedTokenPriceMicrocredits": "cached_token_price_microcredits",
    "priceConfigured": "price_configured",
    "enabled": "enabled",
    "priceVersion": "price_version",
    "capabilityVersion": "capability_version",
    "createdAt": "created_at",
    "updatedAt": "updated_at",
}

# 需要序列化成 JSON 文本落库的字段（NAS 侧是对象，本机是 TEXT）
JSON_FIELDS = {
    "tags": "tags",
    "capabilityConfig": "capability_config_json",
}


def to_text(value):
    if value is None:
        return None
    if isinstance(value, (dict, list)):
        return json.dumps(value, ensure_ascii=False, separators=(",", ":"))
    return str(value)


def to_bool(value):
    if value is None:
        return None
    return 1 if value else 0


def build_row(model, columns):
    row = {}
    for source_field, column in FIELD_MAP.items():
        if column not in columns:
            continue
        value = model.get(source_field)
        if source_field == "capabilityVersion":
            row[column] = value if value is not None else (model.get("capabilityConfig") or {}).get("version")
        else:
            row[column] = to_text(value)
    for source_field, column in JSON_FIELDS.items():
        if column not in columns:
            continue
        row[column] = to_text(model.get(source_field))
    for column in ("price_configured", "enabled"):
        if column in row:
            row[column] = to_bool(row[column])
    return row


def main():
    parser = argparse.ArgumentParser(description="把 NAS 模型配置回灌本机 SQLite")
    parser.add_argument("--db", required=True)
    parser.add_argument("--input", required=True)
    parser.add_argument("--dry-run", action="store_true")
    parser.add_argument("--capability", default="", help="只回灌某类模型，默认全部")
    args = parser.parse_args()

    with open(args.input, encoding="utf-8") as handle:
        payload = json.load(handle)
    if payload.get("schemaVersion") != SCHEMA_VERSION:
        sys.stderr.write("错误：数据包 schemaVersion=%s，本脚本支持 %s\n"
                         % (payload.get("schemaVersion"), SCHEMA_VERSION))
        return 2

    models = payload.get("models") or []
    if args.capability:
        models = [m for m in models if m.get("capability") == args.capability]
    if not models:
        sys.stderr.write("没有可回灌的模型\n")
        return 3

    conn = sqlite3.connect(args.db)
    conn.row_factory = sqlite3.Row
    columns = {row["name"] for row in conn.execute("PRAGMA table_info(channel_models)")}

    inserted, updated, skipped = [], [], []
    for model in models:
        row = build_row(model, columns)
        model_id = row.get("id")
        if not model_id:
            skipped.append((model.get("modelKey", "?"), "缺少 id"))
            continue
        # NAS 已删除的模型：本机同步软删除，不物理清除历史任务引用。
        if model.get("deletedAt"):
            row["deleted_at"] = to_text(model.get("deletedAt"))
        existing = conn.execute(
            "SELECT id, updated_at FROM channel_models WHERE id = ?", (model_id,)
        ).fetchone()
        if existing:
            assignments = ", ".join("%s = ?" % c for c in row if c != "id")
            conn.execute(
                "UPDATE channel_models SET %s WHERE id = ?" % assignments,
                [row[c] for c in row if c != "id"] + [model_id],
            )
            updated.append((model_id, row.get("model_key", "")))
        else:
            names = ", ".join(row)
            placeholders = ", ".join("?" for _ in row)
            conn.execute(
                "INSERT INTO channel_models (%s) VALUES (%s)" % (names, placeholders),
                [row[c] for c in row],
            )
            inserted.append((model_id, row.get("model_key", "")))

    if args.dry_run:
        conn.rollback()
    else:
        conn.commit()

    prefix = "[预览] " if args.dry_run else ""
    for model_id, key in inserted:
        print("%s新增 %-14s %s" % (prefix, model_id, key))
    for model_id, key in updated:
        print("%s更新 %-14s %s" % (prefix, model_id, key))
    for key, reason in skipped:
        print("%s跳过 %-14s %s" % (prefix, key, reason))
    print("%s合计：新增 %d，更新 %d，跳过 %d" % (prefix, len(inserted), len(updated), len(skipped)))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
