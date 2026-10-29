import { Button, Modal, Progress, Tag } from "antd";
import { AlertCircle, Check, FileText, ImageIcon, Layers, Loader2, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";

import { resourceFileUrl } from "@/services/api/resources";
import type { BatchVideoMaterialInput, BatchVideoProjectDetail, BatchVideoReferenceAsset } from "@/services/api/batch-video";
import { parseTags } from "../shared";
import { cn } from "@/lib/utils";

function materialAnalysisLabel(status?: string): string {
    if (status === "succeeded") return "已分析";
    if (status === "failed") return "分析失败";
    if (status === "running" || status === "queued") return "分析中";
    if (status === "cancelled") return "已中止";
    return "待分析";
}

function MaterialThumbnail({
    material,
    index,
    asset,
    disabled,
    onOpen,
    onDelete,
}: {
    material: BatchVideoMaterialInput;
    index: number;
    asset?: BatchVideoReferenceAsset;
    disabled: boolean;
    onOpen: () => void;
    onDelete?: () => void;
}) {
    const status = asset?.analysisStatus || "pending";
    const running = status === "running" || status === "queued";
    return (
        <div
            className={cn(
                "group relative aspect-square overflow-hidden rounded-md border bg-surface-active/40",
                running ? "border-primary ring-2 ring-primary/25" : status === "failed" ? "border-rose-500/60" : "border-border/60",
            )}
            title={`${index + 1}. ${material.title || "未命名素材"} · ${materialAnalysisLabel(status)}`}
        >
            <button type="button" className="block h-full w-full" onClick={onOpen}>
                <img src={resourceFileUrl(material.resourceId)} alt={material.title || "素材"} className={cn("h-full w-full object-cover", status === "pending" ? "opacity-55" : "")} loading="lazy" />
            </button>
            <span className="pointer-events-none absolute left-1 top-1 rounded bg-black/55 px-1 text-[9px] tabular-nums text-white">{index + 1}</span>
            <span className={cn("pointer-events-none absolute bottom-1 right-1 grid size-4 place-items-center rounded-full text-white", status === "succeeded" ? "bg-emerald-600" : status === "failed" ? "bg-rose-600" : running ? "bg-black/55" : "bg-black/30")}>
                {status === "succeeded" ? <Check className="size-2.5" /> : status === "failed" ? <AlertCircle className="size-2.5" /> : running ? <Loader2 className="size-2.5 animate-spin" /> : null}
            </span>
            {onDelete ? (
                <button
                    type="button"
                    className="absolute right-1 top-1 grid size-6 place-items-center rounded-full bg-black/55 text-white opacity-0 transition-opacity hover:bg-black/75 focus-visible:opacity-100 group-hover:opacity-100"
                    aria-label={`删除素材 ${material.title || ""}`}
                    disabled={disabled}
                    onClick={onDelete}
                >
                    <Trash2 className="size-3" />
                </button>
            ) : null}
        </div>
    );
}

export function MaterialsPanel({
    detail,
    actionLoading = "",
    onDeleteMaterial,
    onDeleteBatch,
    onClearAll,
}: {
    detail: BatchVideoProjectDetail;
    actionLoading?: string;
    onDeleteMaterial?: (batchId: string, resourceId: string) => void;
    onDeleteBatch?: (batchId: string, label: string) => void;
    onClearAll?: () => void;
}) {
    const hasAny = detail.materialBatches.some((batch) => batch.materials.length > 0 || batch.documents.length > 0);
    const [preview, setPreview] = useState<{ batchId: string; material: BatchVideoMaterialInput; asset?: BatchVideoReferenceAsset } | null>(null);
    const assetByResource = useMemo(() => new Map(detail.referenceAssets.map((asset) => [asset.resourceId, asset])), [detail.referenceAssets]);
    return (
        <div className="space-y-5">
            <div className="flex flex-wrap items-center justify-between gap-2">
                <div>
                    <h2 className="text-sm font-semibold">素材批次</h2>
                    <p className="mt-0.5 text-xs text-foreground/45">
                        {hasAny ? "通过页面上方“追加素材”添加新批次；图片会并行分析并逐张保存结果，点击缩略图查看大图和单图结果。" : "通过页面上方“上传素材”添加首批素材；图片会并行分析并逐张保存结果，点击缩略图查看大图和单图结果。"}
                    </p>
                </div>
                {hasAny && onClearAll ? (
                    <Button size="small" danger icon={<Trash2 className="size-3.5" />} loading={actionLoading === "clear-materials"} onClick={onClearAll}>
                        清空素材
                    </Button>
                ) : null}
            </div>

            {detail.materialBatches.map((batch) => (
                <section key={batch.id} className="rounded-xl border border-border/70 bg-surface p-4">
                    <div className="flex flex-wrap items-center gap-2">
                        <Layers className="size-4 text-foreground/45" />
                        <span className="text-sm font-medium">{batch.label || `批次 ${batch.batchNo}`}</span>
                        <Tag bordered={false}>{batch.status === "ready" ? "已入库" : "草稿"}</Tag>
                        <span className="text-[11px] text-foreground/40">{batch.materials.length} 图 · {batch.documents.length} 文档</span>
                        {onDeleteBatch ? (
                            <Button
                                className="ml-auto"
                                size="small"
                                type="text"
                                danger
                                icon={<Trash2 className="size-3.5" />}
                                loading={actionLoading === `delete-batch-${batch.id}`}
                                onClick={() => onDeleteBatch(batch.id, batch.label)}
                            >
                                删除批次
                            </Button>
                        ) : null}
                    </div>
                    {batch.materials.length > 0 ? (
                        <div className="mt-3 grid grid-cols-[repeat(auto-fill,88px)] gap-2">
                            {batch.materials.map((material, index) => {
                                const asset = assetByResource.get(material.resourceId);
                                return (
                                    <MaterialThumbnail
                                        key={material.resourceId}
                                        material={material}
                                        index={index}
                                        asset={asset}
                                        disabled={actionLoading === `delete-material-${material.resourceId}`}
                                        onOpen={() => setPreview({ batchId: batch.id, material, asset })}
                                        onDelete={onDeleteMaterial ? () => onDeleteMaterial(batch.id, material.resourceId) : undefined}
                                    />
                                );
                            })}
                        </div>
                    ) : null}
                    {batch.documents.length > 0 ? (
                        <div className="mt-3 space-y-2">
                            {batch.documents.map((document, index) => (
                                <details key={index} className="rounded-lg border border-border/60 bg-surface-active/30 px-3 py-2">
                                    <summary className="flex cursor-pointer items-center gap-2 text-xs font-medium">
                                        <FileText className="size-3.5" /> {document.name || `文档 ${index + 1}`}
                                        <span className="ml-auto text-[10px] font-normal text-foreground/40">{document.text.length} 字</span>
                                    </summary>
                                    <pre className="mt-2 max-h-48 overflow-auto whitespace-pre-wrap break-words text-[11px] leading-5 text-foreground/60">{document.text}</pre>
                                </details>
                            ))}
                        </div>
                    ) : null}
                </section>
            ))}

            <section className="rounded-xl border border-border/70 bg-surface p-4">
                <div className="flex items-center justify-between">
                    <h2 className="flex items-center gap-2 text-sm font-semibold"><ImageIcon className="size-4" /> 参考图库</h2>
                    <span className="text-[11px] text-foreground/45">{detail.referenceAssets.length} 张 · 全项目共享</span>
                </div>
                <p className="mt-1 text-xs text-foreground/45">创意只引用这里的一张图；描述与标签由分析模型补全。</p>
                {detail.referenceAssets.length === 0 ? (
                    <p className="mt-4 text-xs text-foreground/45">暂无参考图，上传素材后自动生成。</p>
                ) : (
                    <div className="mt-3 grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
                        {detail.referenceAssets.map((asset) => (
                            <article key={asset.id} className="flex gap-3 rounded-lg border border-border/60 p-2.5">
                                <img src={resourceFileUrl(asset.resourceId)} alt={asset.title} className="size-20 shrink-0 rounded-md object-cover" loading="lazy" />
                                <div className="min-w-0">
                                    <div className="flex items-center gap-1.5">
                                        <h3 className="truncate text-xs font-semibold">{asset.title || "参考图"}</h3>
                                        <Tag bordered={false} color={asset.status === "approved" ? "success" : asset.status === "failed" ? "error" : asset.status === "generating" ? "processing" : "default"} className="shrink-0" icon={asset.status === "generating" ? <Loader2 className="size-3 animate-spin" /> : undefined}>
                                            {asset.status === "approved" ? "可用" : asset.status === "failed" ? "生成失败" : asset.status === "generating" ? "生成中" : asset.status}
                                        </Tag>
                                    </div>
                                    {asset.status === "generating" ? <Progress className="!mb-0 !mt-1" percent={100} showInfo={false} status="active" size="small" /> : null}
                                    <p className="mt-1 line-clamp-3 text-[11px] leading-4 text-foreground/55">{asset.description || (asset.status === "generating" ? "正在生成参考图，完成后自动回填描述。" : "等待分析模型补充描述")}</p>
                                    <div className="mt-1.5 flex flex-wrap gap-1">
                                        {parseTags(asset.tagsJson).slice(0, 4).map((tag) => <Tag key={tag} bordered={false} className="!text-[10px]">{tag}</Tag>)}
                                    </div>
                                </div>
                            </article>
                        ))}
                    </div>
                )}
            </section>

            <section className="rounded-xl border border-border/70 bg-surface p-4">
                <div className="flex items-center justify-between">
                    <h2 className="text-sm font-semibold">卖点清单</h2>
                    <span className="text-[11px] text-foreground/45">{detail.sellingPoints.length} 条</span>
                </div>
                {detail.sellingPoints.length === 0 ? (
                    <p className="mt-3 text-xs text-foreground/45">{detail.project.analysisStatus === "queued" || detail.project.analysisStatus === "running" ? "分析中，完成后自动生成卖点。" : detail.project.analysisStatus === "awaiting_decision" ? "有图片分析失败，请在进度窗确认继续或中止。" : "暂无卖点，上传素材后会自动分析生成。"}</p>
                ) : (
                    <ol className="mt-3 grid gap-2 sm:grid-cols-2">
                        {detail.sellingPoints.map((point, index) => (
                            <li key={point.id} className="flex items-start gap-2 rounded-lg bg-surface-active/35 px-3 py-2 text-xs">
                                <span className="tabular-nums text-foreground/35">{index + 1}</span>
                                <span className="text-foreground/70">{point.canonicalText}</span>
                                <span className="ml-auto shrink-0 text-[10px] text-foreground/30">{Math.round(point.confidence * 100)}%</span>
                            </li>
                        ))}
                    </ol>
                )}
            </section>

            <Modal
                open={Boolean(preview)}
                title={preview?.material.title || "素材"}
                width={760}
                onCancel={() => setPreview(null)}
                footer={
                    <>
                        <Button onClick={() => setPreview(null)}>关闭</Button>
                        {preview && onDeleteMaterial ? (
                            <Button
                                danger
                                icon={<Trash2 className="size-3.5" />}
                                disabled={actionLoading === `delete-material-${preview.material.resourceId}`}
                                onClick={() => {
                                    onDeleteMaterial(preview.batchId, preview.material.resourceId);
                                    setPreview(null);
                                }}
                            >
                                删除这张素材
                            </Button>
                        ) : null}
                    </>
                }
            >
                {preview ? (
                    <div className="grid gap-4 py-2 md:grid-cols-[minmax(0,1fr)_260px]">
                        <img src={resourceFileUrl(preview.material.resourceId)} alt={preview.material.title || "素材"} className="max-h-[60vh] w-full rounded-lg bg-surface-active/40 object-contain" />
                        <div className="space-y-3 text-xs">
                            <div className="flex flex-wrap items-center gap-1.5">
                                <Tag bordered={false} color={preview.asset?.analysisStatus === "succeeded" ? "success" : preview.asset?.analysisStatus === "failed" ? "error" : preview.asset?.analysisStatus === "running" || preview.asset?.analysisStatus === "queued" ? "processing" : "default"} icon={preview.asset?.analysisStatus === "running" || preview.asset?.analysisStatus === "queued" ? <Loader2 className="size-3 animate-spin" /> : undefined}>
                                    {materialAnalysisLabel(preview.asset?.analysisStatus)}
                                </Tag>
                                {preview.asset?.analyzedAt ? <span className="text-[11px] text-foreground/40">{new Date(preview.asset.analyzedAt).toLocaleString("zh-CN")}</span> : null}
                            </div>
                            {preview.asset?.analysisError ? <p className="rounded-md bg-rose-500/10 px-2 py-1.5 text-rose-600 dark:text-rose-400">{preview.asset.analysisError}</p> : null}
                            <div>
                                <div className="text-[11px] font-medium text-foreground/45">单图分析结果</div>
                                <p className="mt-1 whitespace-pre-wrap leading-5 text-foreground/70">{preview.asset?.description || "还没有分析结果，点击页面上方“分析素材”后会逐张生成。"}</p>
                            </div>
                            {parseTags(preview.asset?.tagsJson || "").length > 0 ? (
                                <div>
                                    <div className="text-[11px] font-medium text-foreground/45">标签</div>
                                    <div className="mt-1 flex flex-wrap gap-1">
                                        {parseTags(preview.asset?.tagsJson || "").map((tag) => <Tag key={tag} bordered={false} className="!text-[10px]">{tag}</Tag>)}
                                    </div>
                                </div>
                            ) : null}
                            {preview.material.description ? (
                                <div>
                                    <div className="text-[11px] font-medium text-foreground/45">上传备注</div>
                                    <p className="mt-1 whitespace-pre-wrap leading-5 text-foreground/60">{preview.material.description}</p>
                                </div>
                            ) : null}
                        </div>
                    </div>
                ) : null}
            </Modal>
        </div>
    );
}
