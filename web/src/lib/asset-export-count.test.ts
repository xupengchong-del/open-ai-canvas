import assert from "node:assert/strict";
import test from "node:test";

// @ts-expect-error -- Node 原生 TypeScript 测试运行器需要保留扩展名。
import { assetExportCount, withAssetExported, withoutAssetExportCount } from "./asset-export-count.ts";

type TestAsset = {
    id: string;
    kind: "video";
    title: string;
    createdAt: string;
    metadata?: Record<string, unknown>;
    data: Record<string, unknown>;
};

function video(metadata?: Record<string, unknown>): TestAsset {
    return { id: "v1", kind: "video", title: "20261029-001", createdAt: "2026-10-29T01:00:00Z", metadata, data: { mimeType: "video/mp4" } };
}

test("未导出过的素材计数为 0，非法计数被忽略", () => {
    assert.equal(assetExportCount(video() as never), 0);
    assert.equal(assetExportCount(video({ source: "generation-task" }) as never), 0);
    assert.equal(assetExportCount(video({ exportCount: -2 }) as never), 0);
    assert.equal(assetExportCount(video({ exportCount: "3" }) as never), 0);
    assert.equal(assetExportCount(video({ exportCount: Number.NaN }) as never), 0);
});

test("每次导出加一，并保留其余 metadata", () => {
    const first = withAssetExported(video({ source: "generation-task" }) as never);
    assert.equal(assetExportCount(first as never), 1);
    assert.equal(first.metadata?.source, "generation-task");
    const second = withAssetExported(first as never);
    assert.equal(assetExportCount(second as never), 2);
    assert.equal(assetExportCount(withAssetExported(second as never) as never), 3);
});

test("导入素材包时去掉导出标记且不动其他字段", () => {
    assert.deepEqual(withoutAssetExportCount({ exportCount: 4, source: "generation-task" }), { source: "generation-task" });
    assert.equal(withoutAssetExportCount(undefined), undefined);
    const untouched = { source: "manual" };
    assert.equal(withoutAssetExportCount(untouched), untouched);
});
