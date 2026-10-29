import assert from "node:assert/strict";
import test from "node:test";

// @ts-expect-error -- Node 原生 TypeScript 测试运行器需要保留扩展名。
import { assetFileName, generatedAssetTitle, normalizeGeneratedAssetTitles } from "./asset-naming.ts";

type TestAsset = {
    id: string;
    kind: "image" | "video" | "audio" | "model";
    title: string;
    createdAt: string;
    metadata: Record<string, unknown>;
    data: Record<string, unknown>;
};

function asset(input: Partial<TestAsset> & Pick<TestAsset, "id" | "kind" | "title" | "createdAt">): TestAsset {
    return { metadata: { source: "generation-task" }, data: { mimeType: `${input.kind}/mp4` }, ...input };
}

function localIso(month: number, day: number, hour: number): string {
    return new Date(2026, month - 1, day, hour).toISOString();
}

test("生成素材按日期和类型续号", () => {
    const assets = [asset({ id: "a", kind: "video", title: "20261029-001", createdAt: localIso(10, 29, 1) })];
    const now = new Date(2026, 9, 29, 9);
    assert.equal(generatedAssetTitle("video", assets as never, now), "20261029-002");
    assert.equal(generatedAssetTitle("image", assets as never, now), "20261029-001");
});

test("历史固定标题按创建时间重新编号，且不覆盖已有序号", () => {
    const normalized = normalizeGeneratedAssetTitles([
        asset({ id: "b", kind: "video", title: "生成视频", createdAt: localIso(10, 13, 8) }),
        asset({ id: "c", kind: "video", title: "生成视频", createdAt: localIso(10, 13, 7) }),
        asset({ id: "d", kind: "video", title: "20261013-002", createdAt: localIso(10, 13, 6) }),
        asset({ id: "e", kind: "image", title: "生成图片", createdAt: localIso(10, 13, 5) }),
    ] as never);
    assert.deepEqual(
        normalized.map((item) => [item.id, item.title]),
        [
            ["b", "20261013-004"],
            ["c", "20261013-003"],
            ["d", "20261013-002"],
            ["e", "20261013-001"],
        ],
    );
});

test("非生成来源的自定义标题不被改写", () => {
    const input = [{ ...asset({ id: "f", kind: "video", title: "生成视频", createdAt: localIso(10, 13, 8) }), metadata: {} }];
    const normalized = normalizeGeneratedAssetTitles(input as never);
    assert.equal(normalized, input);
});

test("导出文件名去掉标题里的旧后缀并补上真实后缀", () => {
    assert.equal(assetFileName({ kind: "video", title: "20261029-001", data: { mimeType: "video/mp4" } } as never), "20261029-001.mp4");
    assert.equal(assetFileName({ kind: "image", title: "09.jpg", data: { mimeType: "image/jpeg" } } as never), "09.jpg");
    assert.equal(assetFileName({ kind: "video", title: "demo.mov", data: { mimeType: "video/mp4" } } as never), "demo.mp4");
});
