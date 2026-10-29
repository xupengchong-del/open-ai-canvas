import type { Asset } from "@/stores/use-asset-store";

const EXPORT_COUNT_KEY = "exportCount";

/** 素材被导出（下载或打包导出）的次数，记录在 metadata 里随素材同步。 */
export function assetExportCount(asset: Asset): number {
    const value = asset.metadata?.[EXPORT_COUNT_KEY];
    return typeof value === "number" && Number.isFinite(value) && value > 0 ? Math.floor(value) : 0;
}

export function withAssetExported(asset: Asset): Asset {
    return { ...asset, metadata: { ...asset.metadata, [EXPORT_COUNT_KEY]: assetExportCount(asset) + 1 } };
}

/** 导入别人的素材包时不保留导出标记，避免把对方的导出历史算到自己头上。 */
export function withoutAssetExportCount(metadata: Record<string, unknown> | undefined): Record<string, unknown> | undefined {
    if (!metadata || !(EXPORT_COUNT_KEY in metadata)) return metadata;
    const next = { ...metadata };
    delete next[EXPORT_COUNT_KEY];
    return next;
}
