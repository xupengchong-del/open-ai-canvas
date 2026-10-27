import { Button, Drawer, Image, Input, Tag } from "antd";
import { ImageOff, RefreshCw, Save } from "lucide-react";
import { useState } from "react";

import { resourceFileUrl } from "@/services/api/resources";
import type { BatchVideoCreative, BatchVideoProjectDetail } from "@/services/api/batch-video";
import { jobStatusColor, jobStatusLabel, reviewStatusColor, reviewStatusLabel, templateLabel } from "../shared";

type CreativeDraft = Pick<BatchVideoCreative, "title" | "hook" | "copywriting" | "videoPrompt" | "imagePrompt">;

type ImageRequirement = {
    scene?: string;
    angle?: string;
    style?: string;
    aspectRatio?: string;
    requiredElements?: string[];
    forbiddenElements?: string[];
};

function parseRequirement(raw: string): ImageRequirement {
    try {
        return JSON.parse(raw || "{}") as ImageRequirement;
    } catch {
        return {};
    }
}

export function CreativeDetailDrawer({
    open,
    creative,
    detail,
    saving,
    rematching,
    onClose,
    onSave,
    onRematch,
}: {
    open: boolean;
    creative: BatchVideoCreative;
    detail: BatchVideoProjectDetail;
    saving: boolean;
    rematching: boolean;
    onClose: () => void;
    onSave: (draft: CreativeDraft) => void;
    onRematch: () => void;
}) {
    const [draft, setDraft] = useState<CreativeDraft>({
        title: creative.title,
        hook: creative.hook,
        copywriting: creative.copywriting,
        videoPrompt: creative.videoPrompt,
        imagePrompt: creative.imagePrompt,
    });
    const requirement = parseRequirement(creative.imageRequirementJson);
    const asset = detail.referenceAssets.find((item) => item.id === creative.matchedReferenceAssetId);
    const sellingPoint = detail.sellingPoints.find((item) => item.id === creative.sellingPointId);
    const jobs = detail.jobs.filter((job) => job.creativeId === creative.id);
    const similar = detail.creatives.filter((item) => item.id !== creative.id && item.sellingPointId === creative.sellingPointId).slice(0, 5);
    const submitted = jobs.length > 0;
    const update = (patch: Partial<CreativeDraft>) => setDraft((current) => ({ ...current, ...patch }));
    const valid = Object.values(draft).every((value) => value.trim());

    return (
        <Drawer
            open={open}
            onClose={onClose}
            title="创意详情"
            width={720}
            extra={
                <div className="flex gap-2">
                    <Button icon={<RefreshCw className="size-3.5" />} loading={rematching} disabled={submitted || creative.matchStatus === "generating"} onClick={onRematch}>重新匹配</Button>
                    <Button type="primary" icon={<Save className="size-3.5" />} loading={saving} disabled={submitted || !valid} onClick={() => onSave(draft)}>保存并转为待审</Button>
                </div>
            }
        >
            <div className="space-y-5">
                {submitted ? <div className="rounded-lg bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-300">该创意已提交生成，内容与参考图匹配已锁定。</div> : null}
                <div className="flex gap-4">
                    <div className="flex h-44 w-36 shrink-0 items-center justify-center overflow-hidden rounded-xl bg-surface-active/60">
                        {asset ? <Image src={resourceFileUrl(asset.resourceId)} alt={asset.title} className="h-full w-full object-contain" /> : <ImageOff className="size-6 text-foreground/30" />}
                    </div>
                    <div className="min-w-0 flex-1 space-y-2 text-xs">
                        <div className="flex flex-wrap gap-1.5">
                            <Tag bordered={false} color={reviewStatusColor(creative.reviewStatus)}>{reviewStatusLabel(creative.reviewStatus)}</Tag>
                            <Tag bordered={false}>{templateLabel(creative.templateId)}</Tag>
                            <Tag bordered={false}>{creative.durationSeconds}s · {creative.aspectRatio}</Tag>
                        </div>
                        <p><span className="text-foreground/45">卖点：</span>{sellingPoint?.canonicalText || "未关联"}</p>
                        <p><span className="text-foreground/45">匹配：</span>{creative.matchStatus} · {Math.round(creative.matchScore * 100)}% · {creative.matchMethod || "未匹配"}</p>
                        {asset ? <p><span className="text-foreground/45">参考图说明：</span>{asset.description || asset.title}</p> : null}
                        {creative.reviewNote ? <p className="text-rose-600 dark:text-rose-400"><span className="text-foreground/45">驳回原因：</span>{creative.reviewNote}</p> : null}
                    </div>
                </div>

                <label className="grid gap-1.5 text-sm">
                    <span className="font-medium">标题</span>
                    <Input value={draft.title} maxLength={160} disabled={submitted} onChange={(event) => update({ title: event.target.value })} />
                </label>
                <label className="grid gap-1.5 text-sm">
                    <span className="font-medium">钩子</span>
                    <Input.TextArea value={draft.hook} autoSize={{ minRows: 2, maxRows: 5 }} disabled={submitted} onChange={(event) => update({ hook: event.target.value })} />
                </label>
                <label className="grid gap-1.5 text-sm">
                    <span className="font-medium">完整文案</span>
                    <Input.TextArea value={draft.copywriting} autoSize={{ minRows: 5, maxRows: 12 }} disabled={submitted} onChange={(event) => update({ copywriting: event.target.value })} />
                </label>
                <label className="grid gap-1.5 text-sm">
                    <span className="font-medium">视频提示词</span>
                    <Input.TextArea value={draft.videoPrompt} autoSize={{ minRows: 4, maxRows: 10 }} disabled={submitted} onChange={(event) => update({ videoPrompt: event.target.value })} />
                </label>
                <label className="grid gap-1.5 text-sm">
                    <span className="font-medium">图片提示词</span>
                    <Input.TextArea value={draft.imagePrompt} autoSize={{ minRows: 3, maxRows: 8 }} disabled={submitted} onChange={(event) => update({ imagePrompt: event.target.value })} />
                </label>

                <section className="rounded-xl border border-border/70 p-4">
                    <h3 className="text-sm font-semibold">结构化图片需求</h3>
                    <dl className="mt-3 grid grid-cols-[90px_1fr] gap-y-2 text-xs">
                        <dt className="text-foreground/45">场景</dt><dd>{requirement.scene || "—"}</dd>
                        <dt className="text-foreground/45">角度</dt><dd>{requirement.angle || "—"}</dd>
                        <dt className="text-foreground/45">风格</dt><dd>{requirement.style || "—"}</dd>
                        <dt className="text-foreground/45">必须包含</dt><dd>{requirement.requiredElements?.join("、") || "—"}</dd>
                        <dt className="text-foreground/45">禁止出现</dt><dd>{requirement.forbiddenElements?.join("、") || "—"}</dd>
                    </dl>
                </section>

                <section>
                    <h3 className="text-sm font-semibold">生成历史（{jobs.length}）</h3>
                    {jobs.length > 0 ? <div className="mt-2 space-y-2">{jobs.map((job) => (
                        <div key={job.id} className="flex items-center justify-between gap-3 rounded-lg bg-surface-active/40 px-3 py-2 text-xs">
                            <span>{job.modelId || "自动路由"} · 尝试 {job.attemptsUsed} · 切换 {job.failoverCount}</span>
                            <Tag bordered={false} color={jobStatusColor(job.status)}>{jobStatusLabel(job.status)}</Tag>
                        </div>
                    ))}</div> : <p className="mt-2 text-xs text-foreground/45">尚未提交生成。</p>}
                </section>

                <section>
                    <h3 className="text-sm font-semibold">同卖点创意</h3>
                    {similar.length > 0 ? <ul className="mt-2 space-y-1.5 text-xs text-foreground/65">{similar.map((item) => <li key={item.id}>#{item.seq} {item.title}</li>)}</ul> : <p className="mt-2 text-xs text-foreground/45">没有其他同卖点创意。</p>}
                </section>
            </div>
        </Drawer>
    );
}
