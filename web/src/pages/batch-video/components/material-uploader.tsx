import { App, Button, Input } from "antd";
import { FileText, ImagePlus, Loader2, Trash2, Upload } from "lucide-react";
import { useRef, useState } from "react";

import { resourceFileUrl } from "@/services/api/resources";
import { uploadBatchVideoFile, type BatchVideoDocumentInput, type BatchVideoMaterialInput } from "@/services/api/batch-video";
import { cn } from "@/lib/utils";

async function imageSize(file: File): Promise<{ width: number; height: number }> {
    try {
        const bitmap = await createImageBitmap(file);
        const size = { width: bitmap.width, height: bitmap.height };
        bitmap.close();
        return size;
    } catch {
        return { width: 0, height: 0 };
    }
}

export function MaterialUploader({
    materials,
    onMaterialsChange,
    documents,
    onDocumentsChange,
    compact = false,
}: {
    materials: BatchVideoMaterialInput[];
    onMaterialsChange: (next: BatchVideoMaterialInput[]) => void;
    documents: BatchVideoDocumentInput[];
    onDocumentsChange: (next: BatchVideoDocumentInput[]) => void;
    compact?: boolean;
}) {
    const { message } = App.useApp();
    const imageInputRef = useRef<HTMLInputElement>(null);
    const [uploading, setUploading] = useState(false);
    const [uploadedCount, setUploadedCount] = useState(0);

    const uploadImages = async (files: FileList | null) => {
        if (!files || files.length === 0) return;
        setUploading(true);
        setUploadedCount(0);
        const created: BatchVideoMaterialInput[] = [];
        try {
            for (const file of Array.from(files)) {
                if (!file.type.startsWith("image/")) continue;
                const size = await imageSize(file);
                const resource = await uploadBatchVideoFile(file, "material_image", "image", size.width, size.height);
                created.push({ resourceId: resource.id, kind: "image", title: file.name.replace(/\.[^.]+$/, ""), description: "", tags: [] });
                setUploadedCount((value) => value + 1);
            }
        } catch (error) {
            message.error(error instanceof Error ? error.message : "图片上传失败");
        } finally {
            setUploading(false);
            if (imageInputRef.current) imageInputRef.current.value = "";
        }
        if (created.length > 0) {
            const seen = new Set(materials.map((item) => item.resourceId));
            onMaterialsChange([...materials, ...created.filter((item) => !seen.has(item.resourceId))]);
        }
    };

    const uploadDocuments = async (files: FileList | null) => {
        if (!files || files.length === 0) return;
        const next = [...documents];
        for (const file of Array.from(files)) {
            if (file.size > 1024 * 1024) {
                message.warning(`${file.name} 超过 1MB，已跳过`);
                continue;
            }
            try {
                const text = await file.text();
                next.push({ name: file.name.replace(/\.[^.]+$/, ""), text });
            } catch {
                message.warning(`${file.name} 读取失败，已跳过`);
            }
        }
        onDocumentsChange(next);
    };

    return (
        <div className="flex flex-col gap-4">
            <div className="rounded-xl border border-dashed border-border/80 bg-surface-active/25 p-4">
                <div className="flex flex-wrap items-center justify-between gap-2">
                    <div>
                        <div className="flex items-center gap-2 text-sm font-medium">
                            <ImagePlus className="size-4" /> 产品素材图片
                        </div>
                        <p className="mt-1 text-xs text-foreground/50">建议至少 1 张。用于提取卖点和匹配创意参考图；只有文档也可创建，同一文件不会重复上传。</p>
                    </div>
                    <Button icon={uploading ? <Loader2 className="size-4 animate-spin" /> : <Upload className="size-4" />} disabled={uploading} onClick={() => imageInputRef.current?.click()}>
                        {uploading ? `上传中 ${uploadedCount}` : "上传图片"}
                    </Button>
                    <input ref={imageInputRef} type="file" accept="image/*" multiple className="hidden" onChange={(event) => void uploadImages(event.target.files)} />
                </div>
                {materials.length > 0 ? (
                    <div className={cn("mt-4 grid gap-3", compact ? "grid-cols-3 sm:grid-cols-5" : "grid-cols-2 sm:grid-cols-3 lg:grid-cols-4")}>
                        {materials.map((material, index) => (
                            <div key={material.resourceId} className="group overflow-hidden rounded-lg border border-border/70 bg-surface">
                                <div className="relative aspect-square bg-surface-active/60">
                                    <img src={resourceFileUrl(material.resourceId)} alt={material.title || "素材"} className="h-full w-full object-cover" loading="lazy" />
                                    <button
                                        type="button"
                                        className="absolute right-1.5 top-1.5 grid size-7 place-items-center rounded-full bg-black/55 text-white opacity-0 transition-opacity group-hover:opacity-100"
                                        aria-label="移除素材"
                                        onClick={() => onMaterialsChange(materials.filter((_, itemIndex) => itemIndex !== index))}
                                    >
                                        <Trash2 className="size-3.5" />
                                    </button>
                                </div>
                                {!compact ? (
                                    <div className="space-y-1.5 p-2">
                                        <Input
                                            size="small"
                                            value={material.title}
                                            placeholder="素材名称"
                                            onChange={(event) => onMaterialsChange(materials.map((item, itemIndex) => (itemIndex === index ? { ...item, title: event.target.value } : item)))}
                                        />
                                        <Input
                                            size="small"
                                            value={material.description}
                                            placeholder="补充说明（可选）"
                                            onChange={(event) => onMaterialsChange(materials.map((item, itemIndex) => (itemIndex === index ? { ...item, description: event.target.value } : item)))}
                                        />
                                    </div>
                                ) : null}
                            </div>
                        ))}
                    </div>
                ) : (
                    <div className="mt-4 rounded-lg border border-border/70 bg-surface px-4 py-6 text-center text-xs text-foreground/45">还没有素材图片</div>
                )}
            </div>

            <div className="rounded-xl border border-border/80 bg-surface-active/25 p-4">
                <div className="flex flex-wrap items-center justify-between gap-2">
                    <div>
                        <div className="flex items-center gap-2 text-sm font-medium">
                            <FileText className="size-4" /> 卖点文档（可选）
                        </div>
                        <p className="mt-1 text-xs text-foreground/50">支持 txt、md，也可直接粘贴文本；内容只用于本项目的卖点分析，保存在服务器本地。</p>
                    </div>
                    <div className="flex gap-2">
                        <label className="inline-flex cursor-pointer items-center gap-1.5 rounded-md border border-border/80 px-3 py-1.5 text-xs font-medium hover:bg-surface-active">
                            <Upload className="size-3.5" /> 上传文档
                            <input type="file" accept=".txt,.md,text/plain,text/markdown" multiple className="hidden" onChange={(event) => void uploadDocuments(event.target.files)} />
                        </label>
                        <Button size="small" type="text" onClick={() => onDocumentsChange([...documents, { name: `卖点文档 ${documents.length + 1}`, text: "" }])}>
                            + 手动填写
                        </Button>
                    </div>
                </div>
                {documents.length > 0 ? (
                    <div className="mt-3 space-y-2">
                        {documents.map((document, index) => (
                            <div key={index} className="rounded-lg border border-border/70 bg-surface p-2">
                                <div className="flex items-center gap-2">
                                    <Input
                                        size="small"
                                        value={document.name}
                                        placeholder="文档名称"
                                        onChange={(event) => onDocumentsChange(documents.map((item, itemIndex) => (itemIndex === index ? { ...item, name: event.target.value } : item)))}
                                    />
                                    <Button size="small" type="text" danger icon={<Trash2 className="size-3.5" />} onClick={() => onDocumentsChange(documents.filter((_, itemIndex) => itemIndex !== index))} />
                                </div>
                                <Input.TextArea
                                    className="mt-2"
                                    value={document.text}
                                    autoSize={{ minRows: 2, maxRows: 8 }}
                                    placeholder="粘贴卖点、产品参数或使用场景"
                                    onChange={(event) => onDocumentsChange(documents.map((item, itemIndex) => (itemIndex === index ? { ...item, text: event.target.value } : item)))}
                                />
                            </div>
                        ))}
                    </div>
                ) : null}
            </div>
        </div>
    );
}
