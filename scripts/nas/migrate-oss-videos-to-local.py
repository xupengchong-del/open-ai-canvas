#!/usr/bin/env python3
"""把 NAS 上已经落在对象存储里的视频迁移回服务器本地。

背景：画布生成的视频由浏览器经普通上传接口回传，旧版本没有读取
「生成结果存储策略」，于是这一批文件被写进了 OSS。后端修复只影响新写入，
存量需要逐条调用管理端 /api/admin/resources/:id/move-to-local：
服务端会从 OSS 流式下载、写本地磁盘，成功后再删除远端对象。

用法：
    migrate-oss-videos-to-local.py --host 192.168.1.56 --port 3050 \
        --username xupc --password … --dry-run
    migrate-oss-videos-to-local.py …            # 真正迁移

退出码：0 成功；2 参数错误；4 接口失败；5 存在未迁移成功的资源。
"""

import argparse
import importlib.util
import os
import sys
import urllib.parse

HERE = os.path.dirname(os.path.abspath(__file__))


def load_nas_client():
    """复用 fetch-nas-models.py 里的登录与请求实现（文件名带连字符，无法直接 import）。"""
    path = os.path.join(HERE, "fetch-nas-models.py")
    spec = importlib.util.spec_from_file_location("nas_fetch_models", path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module.NasClient, module.ApiError


def collect_resources(client, kind, page_size):
    items = []
    page = 1
    while True:
        query = urllib.parse.urlencode({"kind": kind, "page": page, "pageSize": page_size})
        data = client.get("/api/admin/resources?" + query)
        batch = data.get("items") or []
        items.extend(batch)
        total = int(data.get("total") or 0)
        if not batch or len(items) >= total:
            return items
        page += 1


def main():
    parser = argparse.ArgumentParser(description="把 NAS 上 OSS 里的视频迁移回本地磁盘")
    parser.add_argument("--host", required=True)
    parser.add_argument("--port", default="3050")
    parser.add_argument("--username", required=True)
    parser.add_argument("--password", required=True)
    parser.add_argument("--kind", default="video", help="资源类型，默认 video")
    parser.add_argument("--page-size", type=int, default=100)
    parser.add_argument("--max", type=int, default=0, help="本次最多迁移多少个，0 表示不限")
    parser.add_argument("--dry-run", action="store_true", help="只列出待迁移资源")
    args = parser.parse_args()

    NasClient, ApiError = load_nas_client()
    client = NasClient(args.host, args.port, args.username, args.password)
    try:
        client.login()
        items = collect_resources(client, args.kind, args.page_size)
    except ApiError as exc:
        sys.stderr.write("错误：%s\n" % exc)
        return 4

    targets = [item for item in items if item.get("provider") not in ("", "local")]
    total_bytes = sum(int(item.get("size") or 0) for item in targets)
    print("扫描 %d 条 %s 资源，其中 %d 条在对象存储（%.1f MB）" % (
        len(items), args.kind, len(targets), total_bytes / 1024.0 / 1024.0), flush=True)

    if args.dry_run:
        for item in targets:
            print("  %s  %-6s  %8.2f MB  %s" % (
                item.get("id"), item.get("provider"),
                int(item.get("size") or 0) / 1024.0 / 1024.0, item.get("objectKey")), flush=True)
        print("预览结束：去掉 --dry-run 才会真正迁移", flush=True)
        return 0

    if args.max:
        targets = targets[: args.max]

    migrated = 0
    failed = []
    for index, item in enumerate(targets, start=1):
        resource_id = item.get("id")
        size_mb = int(item.get("size") or 0) / 1024.0 / 1024.0
        try:
            client._request(
                "/api/admin/resources/%s/move-to-local" % urllib.parse.quote(resource_id),
                data={},
            )
        except ApiError as exc:
            failed.append((resource_id, str(exc)))
            print("[%d/%d] 失败 %s：%s" % (index, len(targets), resource_id, exc), flush=True)
            continue
        migrated += 1
        print("[%d/%d] 已迁移 %s（%.2f MB）" % (index, len(targets), resource_id, size_mb), flush=True)

    print("完成：成功 %d，失败 %d" % (migrated, len(failed)), flush=True)
    for resource_id, message in failed[:10]:
        print("  %s：%s" % (resource_id, message), flush=True)
    return 5 if failed else 0


if __name__ == "__main__":
    raise SystemExit(main())
