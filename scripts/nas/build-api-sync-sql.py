#!/usr/bin/env python3
"""把本机 SQLite 开发库里的模型渠道 / 模型 / 管理员账号导出为 PostgreSQL 语句。

用法：
    build-api-sync-sql.py <sqlite.db> <target-schema.txt>

target-schema.txt 每行形如 "table|column|data_type"，由 NAS 上的 PostgreSQL 查询得到；
脚本按目标库真实类型生成字面量，并自动跳过目标库不存在的列（模式漂移时不会写坏数据）。

输出为幂等 SQL：已存在的行不会被覆盖（ON CONFLICT DO NOTHING）。
"""

import sqlite3
import sys

# 同步顺序：先账号，再渠道，最后渠道下的模型、系统设置及存储位置（外键指向 users / model_channels）。
TABLES = ["users", "model_channels", "channel_models", "system_settings", "storage_locations", "user_oss_settings"]


def load_target_schema(path):
    schema = {}
    with open(path, encoding="utf-8") as handle:
        for line in handle:
            line = line.rstrip("\n")
            if not line or line.count("|") != 2:
                continue
            table, column, data_type = line.split("|")
            schema.setdefault(table.strip(), {})[column.strip()] = data_type.strip()
    return schema


def sql_literal(value, data_type):
    if value is None:
        return "NULL"
    lowered = data_type.lower()
    if "timestamp" in lowered or lowered in ("date", "time"):
        return "'%s'::%s" % (str(value).replace("'", "''"), data_type)
    if lowered == "boolean":
        text = str(value).strip().lower()
        return "TRUE" if text in ("1", "t", "true", "y", "yes") else "FALSE"
    if lowered in ("smallint", "integer", "bigint", "numeric", "double precision", "real"):
        text = str(value).strip()
        if text == "":
            return "NULL"
        try:
            float(text)
        except ValueError:
            return "NULL"
        return text
    if lowered in ("json", "jsonb"):
        return "'%s'::%s" % (str(value).replace("'", "''"), data_type)
    return "'%s'" % str(value).replace("'", "''")


def quote_ident(name):
    return '"%s"' % name.replace('"', '""')


def main():
    if len(sys.argv) != 3:
        sys.stderr.write("用法: build-api-sync-sql.py <sqlite.db> <target-schema.txt>\n")
        return 2

    source_path, schema_path = sys.argv[1], sys.argv[2]
    target = load_target_schema(schema_path)
    source = sqlite3.connect("file:%s?mode=ro" % source_path, uri=True)
    source.row_factory = sqlite3.Row

    out = ["BEGIN;"]
    for table in TABLES:
        columns = target.get(table)
        if not columns:
            sys.stderr.write("跳过 %s：目标库没有这张表\n" % table)
            continue

        existing = [row[1] for row in source.execute("PRAGMA table_info(%s)" % table)]
        if not existing:
            sys.stderr.write("跳过 %s：源库没有这张表\n" % table)
            continue

        usable = [name for name in existing if name in columns]
        dropped = [name for name in existing if name not in columns]
        if dropped:
            sys.stderr.write("%s：目标库缺少列 %s，已跳过\n" % (table, ", ".join(dropped)))

        rows = source.execute(
            "SELECT %s FROM %s" % (", ".join(quote_ident(c) for c in usable), table)
        ).fetchall()
        if not rows:
            continue

        column_sql = ", ".join(quote_ident(c) for c in usable)
        out.append("-- %s: %d 行" % (table, len(rows)))
        for row in rows:
            values = ", ".join(sql_literal(row[c], columns[c]) for c in usable)
            out.append(
                "INSERT INTO %s (%s) VALUES (%s) ON CONFLICT DO NOTHING;"
                % (quote_ident(table), column_sql, values)
            )

    out.append("COMMIT;")
    sys.stdout.write("\n".join(out) + "\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
