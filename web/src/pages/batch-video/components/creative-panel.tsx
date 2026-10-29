import { Button, Checkbox, Image, Tag, Tooltip } from "antd";
import { Check, ImageOff, Loader2, Pencil, X } from "lucide-react";

import { resourceFileUrl } from "@/services/api/resources";
import type { BatchVideoCreative, BatchVideoProjectDetail } from "@/services/api/batch-video";
import { PaginationBar } from "@/components/layout/workspace-page";
import { Select } from "@/components/ui/base/select";
import { reviewStatusColor, reviewStatusLabel, templateLabel } from "../shared";

const REVIEW_OPTIONS = [
    { value: "", label: "全部创意" },
    { value: "pending", label: "待审核" },
    { value: "approved", label: "已通过" },
    { value: "rejected", label: "已驳回" },
];

function creativeImage(detail: BatchVideoProjectDetail, creative: BatchVideoCreative): string {
    if (creative.matchedReferenceAssetId) {
        const asset = detail.referenceAssets.find((item) => item.id === creative.matchedReferenceAssetId);
        if (asset) return resourceFileUrl(asset.resourceId);
    }
    return "";
}

export function CreativePanel({
    detail,
    creatives,
    total,
    page,
    pageSize,
    reviewFilter,
    loading,
    reviewSelected,
    submitSelected,
    generatingRound,
    actionLoading,
    mode = "review",
    onReviewSelectChange,
    onSubmitSelectChange,
    onPageChange,
    onFilterChange,
    onReview,
    onBatchReview,
    onGenerateReference,
    onOpen,
}: {
    detail: BatchVideoProjectDetail;
    creatives: BatchVideoCreative[];
    total: number;
    page: number;
    pageSize: number;
    reviewFilter: string;
    loading: boolean;
    reviewSelected: Set<string>;
    submitSelected: Set<string>;
    generatingRound: boolean;
    actionLoading: string;
    mode?: "plan" | "review";
    onReviewSelectChange: (next: Set<string>) => void;
    onSubmitSelectChange: (next: Set<string>) => void;
    onPageChange: (page: number, pageSize: number) => void;
    onFilterChange: (value: string) => void;
    onReview: (creative: BatchVideoCreative, status: "approved" | "rejected") => void;
    onBatchReview: (status: "approved" | "rejected") => void;
    onGenerateReference: (creative: BatchVideoCreative) => void;
    onOpen: (creative: BatchVideoCreative) => void;
}) {
    const reviewMode = mode === "review";
    const reviewable = creatives.filter((creative) => creative.reviewStatus === "pending");
    const submittable = creatives.filter((creative) => creative.reviewStatus === "approved");
    const allReviewSelected = reviewable.length > 0 && reviewable.every((creative) => reviewSelected.has(creative.id));
    const allSubmitSelected = submittable.length > 0 && submittable.every((creative) => submitSelected.has(creative.id));
    const toggleAll = (items: BatchVideoCreative[], selected: Set<string>, allSelected: boolean, onChange: (next: Set<string>) => void) => {
        const next = new Set(selected);
        if (allSelected) items.forEach((creative) => next.delete(creative.id));
        else items.forEach((creative) => next.add(creative.id));
        onChange(next);
    };
    const toggleOne = (id: string, selected: Set<string>, onChange: (next: Set<string>) => void) => {
        const next = new Set(selected);
        if (next.has(id)) next.delete(id);
        else next.add(id);
        onChange(next);
    };

    return (
        <div className="space-y-4">
            <div className="flex flex-col gap-3 rounded-xl border border-border/70 bg-surface p-3 lg:flex-row lg:items-center lg:justify-between">
                <div className="flex flex-wrap items-center gap-2">
                    <Select value={reviewFilter} onChange={(value) => onFilterChange(String(value))} options={REVIEW_OPTIONS} size="sm" />
                    <span className="text-xs text-foreground/45">{total} 条创意</span>
                    {generatingRound ? <Tag color="processing" bordered={false} icon={<Loader2 className="size-3 animate-spin" />}>策划模型生成中</Tag> : null}
                </div>
                {reviewMode ? (
                    <div className="flex flex-wrap items-center gap-2">
                        <Button size="small" disabled={reviewable.length === 0} onClick={() => toggleAll(reviewable, reviewSelected, allReviewSelected, onReviewSelectChange)}>
                            {allReviewSelected ? "取消待审全选" : "全选本页待审"}
                        </Button>
                        <Button size="small" type="primary" icon={<Check className="size-3.5" />} disabled={reviewSelected.size === 0} loading={actionLoading === "batch-approved"} onClick={() => onBatchReview("approved")}>
                            批量通过
                        </Button>
                        <Button size="small" danger icon={<X className="size-3.5" />} disabled={reviewSelected.size === 0} loading={actionLoading === "batch-rejected"} onClick={() => onBatchReview("rejected")}>
                            批量驳回
                        </Button>
                        <Tooltip title="勾选已通过创意后，在页面上方“提交生成”中选择“当前勾选”">
                            <Button size="small" disabled={submittable.length === 0} onClick={() => toggleAll(submittable, submitSelected, allSubmitSelected, onSubmitSelectChange)}>
                                {allSubmitSelected ? "取消生成全选" : "全选本页已通过"}
                            </Button>
                        </Tooltip>
                        {submittable.length > 0 ? (
                            <span className="text-[11px] text-foreground/40">
                                已勾选 {submitSelected.size} 条，提交时在顶部“提交生成”选择“当前勾选”
                            </span>
                        ) : null}
                    </div>
                ) : (
                    <span className="text-[11px] text-foreground/40">策划完成后，到“人工审核”逐条通过或驳回创意。</span>
                )}
            </div>

            {creatives.length === 0 && !loading ? (
                <div className="rounded-xl border border-dashed border-border/80 bg-surface px-4 py-12 text-center">
                    <p className="text-sm font-medium">当前筛选下没有创意</p>
                    <p className="mt-1 text-xs text-foreground/45">{reviewFilter ? "切换审核状态查看其他创意。" : "先完成素材分析，再点击页面上方“生成创意”。"}</p>
                </div>
            ) : (
                <div className={loading ? "pointer-events-none opacity-60" : ""}>
                    <div className="grid gap-3 xl:grid-cols-2">
                        {creatives.map((creative) => {
                            const imageUrl = creativeImage(detail, creative);
                            const pending = creative.reviewStatus === "pending";
                            return (
                                <article key={creative.id} className="flex gap-3 rounded-xl border border-border/70 bg-surface p-3">
                                    {reviewMode ? (
                                        <div className="flex w-12 shrink-0 flex-col items-center gap-3 pt-1 text-[10px] text-foreground/45">
                                            <label className="grid justify-items-center gap-1">
                                                <Checkbox checked={reviewSelected.has(creative.id)} disabled={!pending} onChange={() => toggleOne(creative.id, reviewSelected, onReviewSelectChange)} aria-label="选择审核创意" />
                                                审核
                                            </label>
                                            <label className="grid justify-items-center gap-1">
                                                <Checkbox checked={submitSelected.has(creative.id)} disabled={creative.reviewStatus !== "approved"} onChange={() => toggleOne(creative.id, submitSelected, onSubmitSelectChange)} aria-label="选择生成创意" />
                                                生成
                                            </label>
                                        </div>
                                    ) : null}
                                    <div className="relative h-36 w-28 shrink-0 overflow-hidden rounded-lg bg-surface-active/60">
                                        {imageUrl ? (
                                            <Image src={imageUrl} alt={creative.title} className="h-full w-full object-contain" preview={{ mask: "查看参考图" }} />
                                        ) : (
                                            <div className="flex h-full flex-col items-center justify-center gap-1 text-foreground/35">
                                                <ImageOff className="size-5" />
                                                <span className="text-[10px]">缺少参考图</span>
                                            </div>
                                        )}
                                    </div>
                                    <div className="flex min-w-0 flex-1 flex-col">
                                        <div className="flex items-start justify-between gap-2">
                                            <h3 className="line-clamp-2 text-sm font-semibold leading-5">{creative.title}</h3>
                                            <Tag bordered={false} color={reviewStatusColor(creative.reviewStatus)}>{reviewStatusLabel(creative.reviewStatus)}</Tag>
                                        </div>
                                        <p className="mt-1 line-clamp-2 text-xs leading-5 text-foreground/65">
                                            <span className="font-medium text-foreground/45">钩子：</span>{creative.hook}
                                        </p>
                                        <p className="mt-1 line-clamp-3 text-xs leading-5 text-foreground/60">
                                            <span className="font-medium text-foreground/45">文案：</span>{creative.copywriting}
                                        </p>
                                        <div className="mt-1.5 flex flex-wrap items-center gap-x-2 gap-y-1 text-[10px] text-foreground/40">
                                            <span>{creative.durationSeconds}s · {creative.aspectRatio}</span>
                                            <span>·</span>
                                            <span>{templateLabel(creative.templateId)}</span>
                                            {creative.matchStatus === "matched" ? (
                                                <span>· 参考图匹配 {Math.round(creative.matchScore * 100)}%</span>
                                            ) : creative.matchStatus === "generating" ? (
                                                <span className="inline-flex items-center gap-1 text-sky-600 dark:text-sky-400"><Loader2 className="size-3 animate-spin" />参考图生成中</span>
                                            ) : (
                                                <span className="text-amber-600 dark:text-amber-400">· 未匹配参考图</span>
                                            )}
                                        </div>
                                        <div className="mt-auto flex items-center justify-end gap-1.5 pt-2">
                                            <Button size="small" type="text" icon={<Pencil className="size-3.5" />} onClick={() => onOpen(creative)}>详情 / 编辑</Button>
                                            {creative.matchStatus !== "matched" && creative.matchStatus !== "generating" ? (
                                                <Button size="small" type="text" icon={<ImageOff className="size-3.5" />} loading={actionLoading === `generate-ref-${creative.id}`} onClick={() => onGenerateReference(creative)}>
                                                    生成参考图
                                                </Button>
                                            ) : null}
                                            {reviewMode ? (
                                                creative.reviewStatus === "approved" || creative.reviewStatus === "rejected" ? (
                                                    <Tooltip title="切换审核结论">
                                                        <Button size="small" type="text" onClick={() => onReview(creative, "rejected" === creative.reviewStatus ? "approved" : "rejected")}>
                                                            {creative.reviewStatus === "approved" ? "改为驳回" : "改为通过"}
                                                        </Button>
                                                    </Tooltip>
                                                ) : (
                                                    <>
                                                        <Button size="small" danger icon={<X className="size-3.5" />} loading={actionLoading === `rejected-${creative.id}`} onClick={() => onReview(creative, "rejected")}>
                                                            驳回
                                                        </Button>
                                                        <Button
                                                            size="small"
                                                            type="primary"
                                                            icon={<Check className="size-3.5" />}
                                                            disabled={creative.matchStatus !== "matched"}
                                                            loading={actionLoading === `approved-${creative.id}`}
                                                            onClick={() => onReview(creative, "approved")}
                                                        >
                                                            通过
                                                        </Button>
                                                    </>
                                                )
                                            ) : null}
                                        </div>
                                    </div>
                                </article>
                            );
                        })}
                    </div>
                    <PaginationBar current={page} pageSize={pageSize} total={total} onChange={onPageChange} itemLabel="条创意" alwaysShow />
                </div>
            )}
        </div>
    );
}
