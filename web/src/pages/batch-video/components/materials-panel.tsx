import { Button, Tag } from "antd";
import { FileText, ImageIcon, Layers, Plus } from "lucide-react";

import { resourceFileUrl } from "@/services/api/resources";
import type { BatchVideoProjectDetail } from "@/services/api/batch-video";
import { parseTags } from "../shared";

export function MaterialsPanel({ detail, onAddBatch }: { detail: BatchVideoProjectDetail; onAddBatch: () => void }) {
    return (
        <div className="space-y-5">
            <div className="flex items-center justify-between">
                <div>
                    <h2 className="text-sm font-semibold">素材批次</h2>
                    <p className="mt-0.5 text-xs text-foreground/45">追加新批次后建议重新分析，让卖点和参考图库覆盖新素材。</p>
                </div>
                <Button size="small" icon={<Plus className="size-3.5" />} onClick={onAddBatch}>追加素材批次</Button>
            </div>

            {detail.materialBatches.map((batch) => (
                <section key={batch.id} className="rounded-xl border border-border/70 bg-surface p-4">
                    <div className="flex flex-wrap items-center gap-2">
                        <Layers className="size-4 text-foreground/45" />
                        <span className="text-sm font-medium">{batch.label || `批次 ${batch.batchNo}`}</span>
                        <Tag bordered={false}>{batch.status === "ready" ? "已入库" : "草稿"}</Tag>
                        <span className="text-[11px] text-foreground/40">{batch.materials.length} 图 · {batch.documents.length} 文档</span>
                    </div>
                    {batch.materials.length > 0 ? (
                        <div className="mt-3 grid grid-cols-3 gap-2 sm:grid-cols-5 lg:grid-cols-7">
                            {batch.materials.map((material) => (
                                <figure key={material.resourceId} className="overflow-hidden rounded-lg border border-border/60 bg-surface-active/40">
                                    <img src={resourceFileUrl(material.resourceId)} alt={material.title || "素材"} className="aspect-square w-full object-cover" loading="lazy" />
                                    <figcaption className="truncate px-2 py-1 text-[10px] text-foreground/50" title={material.title}>{material.title || "未命名素材"}</figcaption>
                                </figure>
                            ))}
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
                                        <Tag bordered={false} color={asset.status === "approved" ? "success" : "default"} className="shrink-0">{asset.status === "approved" ? "可用" : asset.status}</Tag>
                                    </div>
                                    <p className="mt-1 line-clamp-3 text-[11px] leading-4 text-foreground/55">{asset.description || "等待分析模型补充描述"}</p>
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
                    <p className="mt-3 text-xs text-foreground/45">暂无卖点，请分析素材。</p>
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
        </div>
    );
}
