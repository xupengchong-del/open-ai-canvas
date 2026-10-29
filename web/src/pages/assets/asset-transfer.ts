import { saveAs } from "file-saver";
import { readZip } from "@/lib/zip";
import { MediaPackage } from "@/lib/media-package";
import { assetFileName } from "@/lib/asset-naming";
import { setMediaBlob } from "@/services/file-storage";
import { setImageBlob } from "@/services/image-storage";
import { getResourceAccess, queryKnownResourceProvider, resourceIdFromStorageKey } from "@/services/api/resources";
import type { Asset } from "@/stores/use-asset-store";
import type { CanvasExportAsset } from "@/types/canvas-export";

type AssetExportFile = { app: "yingce"; version: 1; exportedAt: string; assets: Asset[]; files: CanvasExportAsset[] };
export type AssetExportIssue = { id: string; title: string };
export type AssetExportResult = {
    /** 真正写进压缩包、可以记一次导出的素材 */
    exportedIds: string[];
    /** 本地字节读取失败、没有进包的素材 */
    failed: AssetExportIssue[];
    /** 存放在对象存储、按要求不参与导出的素材 */
    blockedOss: AssetExportIssue[];
    /** 本次是否生成了压缩包 */
    saved: boolean;
};

function mediaStorageKey(asset: Asset): string {
    return "storageKey" in asset.data && typeof asset.data.storageKey === "string" ? asset.data.storageKey : "";
}

/**
 * 浏览器直连对象存储读字节会被桶的 CORS 拦下，这类素材仍在资源库保留但导出时直接跳过，
 * 只有平台本地存储（provider=local）才随包导出。
 */
async function ossAssetIds(assets: Asset[]): Promise<Set<string>> {
    const blocked = new Set<string>();
    await Promise.all(
        assets.map(async (asset) => {
            const storageKey = mediaStorageKey(asset);
            if (!resourceIdFromStorageKey(storageKey)) return;
            const known = queryKnownResourceProvider(storageKey);
            if (known?.provider) {
                if (known.provider !== "local") blocked.add(asset.id);
                return;
            }
            try {
                const access = await getResourceAccess(storageKey, "display");
                const provider = access.storageProvider || (access.delivery === "platform-local" ? "local" : "oss");
                if (provider !== "local") blocked.add(asset.id);
            } catch {
                // 判定不了存储位置时按服务器资源处理，读取失败另有兜底。
            }
        }),
    );
    return blocked;
}

export async function exportAssets(assets: Asset[]): Promise<AssetExportResult> {
    const media = assets.filter((asset) => ["image", "video", "audio", "model"].includes(asset.kind));
    const blockedIds = await ossAssetIds(media);
    const issueOf = (asset: Asset): AssetExportIssue => ({ id: asset.id, title: asset.title || asset.id });
    const blockedOss = assets.filter((asset) => blockedIds.has(asset.id)).map(issueOf);
    const exportable = assets.filter((asset) => !blockedIds.has(asset.id));
    if (!exportable.length) return { exportedIds: [], failed: [], blockedOss, saved: false };

    const images = new Set<string>();
    const keys: string[] = [];
    // 导出包里的文件名用素材标题 + 真实后缀，storageKey 只留在清单里。
    const byStorageKey = new Map<string, Asset>();
    const idsByStorageKey = new Map<string, string[]>();
    for (const asset of exportable) {
        if (!["image", "video", "audio", "model"].includes(asset.kind)) continue;
        const storageKey = mediaStorageKey(asset);
        if (!storageKey) continue;
        keys.push(storageKey);
        if (!byStorageKey.has(storageKey)) byStorageKey.set(storageKey, asset);
        const ids = idsByStorageKey.get(storageKey) ?? [];
        ids.push(asset.id);
        idsByStorageKey.set(storageKey, ids);
        if (asset.kind === "image") images.add(storageKey);
    }
    const archive = new MediaPackage();
    const failedByStorageKey = new Map<string, unknown>();
    const files = await archive.collect(
        keys,
        "files",
        images,
        (storageKey) => {
            const asset = byStorageKey.get(storageKey);
            return asset ? assetFileName(asset) : storageKey;
        },
        (storageKey, error) => failedByStorageKey.set(storageKey, error),
    );
    if (!files.length && keys.length) {
        // 浏览器拿不到任何字节时不生成只有清单的空包，直接把可读的原因交给页面。
        throw new Error("素材文件读取失败：服务器或对象存储无法访问，本次没有生成导出包");
    }
    const manifest: AssetExportFile = {
        app: "yingce",
        version: 1,
        assets: exportable,
        exportedAt: new Date().toISOString(),
        files,
    };
    saveAs(await archive.finish("assets.json", manifest), "我的素材.zip");
    // 缺本地字节或读取失败的素材不会进包，只给真正写进压缩包的素材记一次导出。
    const exportedKeys = new Set(files.map((file) => file.storageKey));
    const exportedIds = [...idsByStorageKey].filter(([storageKey]) => exportedKeys.has(storageKey)).flatMap(([, ids]) => ids);
    const failed = [...failedByStorageKey.keys()].flatMap((storageKey) => {
        const ids = idsByStorageKey.get(storageKey) ?? [];
        return ids.map((id) => ({ id, title: byStorageKey.get(storageKey)?.title || id }));
    });
    return { exportedIds, failed, blockedOss, saved: true };
}

export async function readAssetPackage(file: File): Promise<Asset[]> {
    const archive = await readZip(file);
    const manifestBlob = archive.get("assets.json");
    if (!manifestBlob) throw new Error("素材包缺少 assets.json");
    const manifest: unknown = JSON.parse(await manifestBlob.text());
    if (
        !manifest ||
        typeof manifest !== "object" ||
        !("app" in manifest) ||
        manifest.app !== "yingce" ||
        !("version" in manifest) ||
        manifest.version !== 1 ||
        !("assets" in manifest) ||
        !Array.isArray(manifest.assets) ||
        !("files" in manifest) ||
        !Array.isArray(manifest.files)
    )
        throw new Error("素材包格式或版本不受支持");
    const images = new Set<string>();
    for (const asset of manifest.assets) {
        if (!asset || typeof asset !== "object" || typeof asset.id !== "string" || !asset.data || typeof asset.data !== "object" || typeof asset.kind !== "string") throw new Error("素材包包含无效素材");
        if (asset.kind === "image" && typeof asset.data.storageKey === "string") images.add(asset.data.storageKey);
    }
    // Validate the whole manifest before making the first persistent write.
    const writes = manifest.files.map((entry: unknown) => {
        if (
            !entry ||
            typeof entry !== "object" ||
            !("storageKey" in entry) ||
            typeof entry.storageKey !== "string" ||
            !entry.storageKey ||
            !("path" in entry) ||
            typeof entry.path !== "string" ||
            !("mimeType" in entry) ||
            typeof entry.mimeType !== "string"
        )
            throw new Error("素材包文件索引无效");
        const blob = archive.get(entry.path);
        if (!blob) throw new Error(`素材包缺少文件：${entry.path}`);
        if ("bytes" in entry && entry.bytes !== blob.size) throw new Error(`素材包文件大小不匹配：${entry.path}`);
        return { key: entry.storageKey, blob: blob.slice(0, blob.size, entry.mimeType) };
    });
    for (const { key, blob } of writes) await (images.has(key) || key.startsWith("image:") ? setImageBlob(key, blob) : setMediaBlob(key, blob));
    return manifest.assets as Asset[];
}
