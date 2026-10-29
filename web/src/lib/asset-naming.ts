import type { Asset, AssetKind } from "@/stores/use-asset-store";

// 生成素材的固定标题既无法区分作品，导出时也会撞名；统一改成「日期-序号」。
export const LEGACY_GENERATED_TITLES: Partial<Record<AssetKind, string>> = {
    image: "生成图片",
    video: "生成视频",
    audio: "生成音频",
};

const GENERATED_TITLE_PATTERN = /^(\d{8})-(\d{3,})$/;
const MEDIA_EXTENSIONS = new Set(["png", "jpg", "jpeg", "webp", "gif", "bmp", "avif", "heic", "svg", "mp4", "m4v", "mov", "webm", "mkv", "avi", "mp3", "wav", "m4a", "aac", "ogg", "flac", "opus", "glb", "gltf"]);
const MIME_EXTENSIONS: Record<string, string> = {
    "image/png": "png",
    "image/jpeg": "jpg",
    "image/webp": "webp",
    "image/gif": "gif",
    "image/bmp": "bmp",
    "image/avif": "avif",
    "image/heic": "heic",
    "image/svg+xml": "svg",
    "video/mp4": "mp4",
    "video/quicktime": "mov",
    "video/webm": "webm",
    "video/x-matroska": "mkv",
    "video/x-m4v": "m4v",
    "audio/mpeg": "mp3",
    "audio/mp3": "mp3",
    "audio/wav": "wav",
    "audio/x-wav": "wav",
    "audio/mp4": "m4a",
    "audio/aac": "aac",
    "audio/ogg": "ogg",
    "audio/flac": "flac",
    "model/gltf-binary": "glb",
    "model/gltf+json": "gltf",
};
const FALLBACK_EXTENSIONS: Record<AssetKind, string> = { image: "png", video: "mp4", audio: "mp3", model: "glb", text: "txt", entity: "json" };

export function assetTitleDay(date: Date): string {
    const pad = (value: number) => String(value).padStart(2, "0");
    return `${date.getFullYear()}${pad(date.getMonth() + 1)}${pad(date.getDate())}`;
}

function assetCreatedDay(asset: Asset): string | undefined {
    const time = Date.parse(asset.createdAt);
    return Number.isNaN(time) ? undefined : assetTitleDay(new Date(time));
}

function sequenceKey(kind: AssetKind, day: string): string {
    return `${kind}|${day}`;
}

function titleSequence(title: string, day: string): number {
    const match = GENERATED_TITLE_PATTERN.exec(title.trim());
    if (!match || match[1] !== day) return 0;
    return Number(match[2]);
}

function nextSequence(assets: readonly Asset[], kind: AssetKind, day: string): number {
    let max = 0;
    for (const asset of assets) {
        if (asset.kind !== kind) continue;
        max = Math.max(max, titleSequence(asset.title, day));
    }
    return max + 1;
}

export function generatedAssetTitle(kind: AssetKind, assets: readonly Asset[], now: Date = new Date()): string {
    const day = assetTitleDay(now);
    return `${day}-${String(nextSequence(assets, kind, day)).padStart(3, "0")}`;
}

function isLegacyGeneratedAsset(asset: Asset): boolean {
    const legacy = LEGACY_GENERATED_TITLES[asset.kind];
    return Boolean(legacy) && asset.title.trim() === legacy && asset.metadata?.source === "generation-task";
}

/**
 * 历史生成素材统一是「生成视频/图片/音频」，按创建日期重新编号。
 * 只在标题精确等于旧默认值且来源是生成任务时改写，正常标题不会被触碰。
 */
export function normalizeGeneratedAssetTitles(assets: Asset[]): Asset[] {
    const pending = assets
        .filter(isLegacyGeneratedAsset)
        .map((asset) => ({ asset, day: assetCreatedDay(asset) }))
        .filter((entry): entry is { asset: Asset; day: string } => Boolean(entry.day))
        .sort((a, b) => a.asset.createdAt.localeCompare(b.asset.createdAt) || a.asset.id.localeCompare(b.asset.id));
    if (!pending.length) return assets;

    const sequences = new Map<string, number>();
    for (const asset of assets) {
        const day = assetCreatedDay(asset);
        if (!day) continue;
        const key = sequenceKey(asset.kind, day);
        sequences.set(key, Math.max(sequences.get(key) ?? 0, titleSequence(asset.title, day)));
    }
    const renamed = new Map<string, string>();
    for (const { asset, day } of pending) {
        const key = sequenceKey(asset.kind, day);
        const sequence = (sequences.get(key) ?? 0) + 1;
        sequences.set(key, sequence);
        renamed.set(asset.id, `${day}-${String(sequence).padStart(3, "0")}`);
    }
    return assets.map((asset) => {
        const title = renamed.get(asset.id);
        return title ? { ...asset, title } : asset;
    });
}

export function assetFileExtension(asset: Asset): string {
    const mimeType = "mimeType" in asset.data && typeof asset.data.mimeType === "string" ? asset.data.mimeType.toLowerCase() : "";
    if (asset.kind === "model" && asset.data.fileName) {
        const extension = asset.data.fileName.split(".").pop()?.toLowerCase();
        if (extension) return extension;
    }
    const mapped = MIME_EXTENSIONS[mimeType];
    if (mapped) return mapped;
    const subtype = mimeType.split("/")[1];
    if (subtype && subtype !== "octet-stream") return subtype.replace(/^x-/, "");
    return FALLBACK_EXTENSIONS[asset.kind];
}

/** 素材标题里常带上传文件后缀，导出时统一去掉再补真实后缀，避免 `09.jpg.png`。 */
export function assetFileStem(asset: Asset): string {
    const title = asset.title.trim();
    const stripped = title.replace(/\.([a-z0-9]{1,5})$/i, (match, extension: string) => (MEDIA_EXTENSIONS.has(extension.toLowerCase()) ? "" : match));
    return sanitizeFileName(stripped || LEGACY_GENERATED_TITLES[asset.kind] || asset.kind);
}

export function assetFileName(asset: Asset): string {
    return `${assetFileStem(asset)}.${assetFileExtension(asset)}`;
}

function sanitizeFileName(value: string): string {
    return value.replace(/[\x00-\x1f\\/:*?"<>|]/g, "_").trim();
}
