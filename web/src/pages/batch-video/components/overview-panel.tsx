import { Tag, Tooltip } from "antd";
import { AlertTriangle, CheckCircle2, Clock3 } from "lucide-react";

import type { BatchVideoProjectDetail } from "@/services/api/batch-video";
import { parseBatchVideoProfile } from "@/services/api/batch-video";
import { formatPercent } from "../shared";

export function ProductProfilePanel({ detail }: { detail: BatchVideoProjectDetail }) {
    const { project, settings } = detail;
    const profile = parseBatchVideoProfile(project);
    const analysisAwaiting = project.analysisStatus === "awaiting_decision";
    const analysisRunning = project.analysisStatus === "queued" || project.analysisStatus === "running" || analysisAwaiting;
    return (
        <section className="rounded-xl border border-border/70 bg-surface p-4">
            <div className="flex items-center justify-between">
                <h2 className="text-sm font-semibold">产品档案</h2>
                <div className="flex items-center gap-2">
                    <span className="text-[11px] text-foreground/45">{settings.durationSeconds}s · {settings.aspectRatio}</span>
                    {analysisAwaiting ? <Tag color="warning" bordered={false}>等待确认</Tag> : analysisRunning ? <Tag color="processing" bordered={false}>分析中</Tag> : project.analysisStatus === "failed" ? <Tag color="error" bordered={false}>分析失败</Tag> : project.analysisStatus === "cancelled" ? <Tag color="warning" bordered={false}>已中止</Tag> : project.analysisStatus === "stale" ? <Tag color="warning" bordered={false}>素材已变更</Tag> : null}
                </div>
            </div>
            {(project.analysisStatus === "failed" || project.analysisStatus === "cancelled" || analysisAwaiting) && project.analysisError ? (
                <div className="mt-3 flex items-start gap-2 rounded-lg bg-rose-500/10 p-3 text-xs text-rose-700 dark:text-rose-300">
                    <AlertTriangle className="mt-0.5 size-3.5 shrink-0" />
                    <span>{project.analysisError}</span>
                </div>
            ) : null}
            {profile.productSummary ? (
                <div className="mt-3 space-y-2">
                    <p className="text-sm leading-6 text-foreground/75">{profile.productSummary}</p>
                    {project.analysisStatus === "stale" ? (
                        <p className="text-xs text-amber-700 dark:text-amber-300">素材已增删，当前档案可能过期；点击页面上方“分析素材”重新生成。</p>
                    ) : null}
                </div>
            ) : analysisRunning ? (
                <p className="mt-3 text-xs text-foreground/45">分析任务已提交，完成后自动更新卖点与产品档案。</p>
            ) : detail.materialBatches.length > 0 ? (
                <p className="mt-3 text-xs text-foreground/45">还没有产品摘要；请检查后台文本模型后点击页面上方“分析素材”重试。</p>
            ) : (
                <p className="mt-3 text-xs text-foreground/45">上传素材后会自动开始分析，由策划模型生成产品摘要与卖点。</p>
            )}
            {profile.complianceNotes && profile.complianceNotes.length > 0 ? (
                <div className="mt-3 flex flex-wrap gap-1.5">
                    {profile.complianceNotes.map((note, index) => (
                        <Tag key={index} color="warning" bordered={false}>{note}</Tag>
                    ))}
                </div>
            ) : null}
        </section>
    );
}

export function ValidationPanel({ detail }: { detail: BatchVideoProjectDetail }) {
    const { validation } = detail;
    return (
        <section className="rounded-xl border border-border/70 bg-surface p-4">
            <div className="flex items-center justify-between gap-3">
                <h2 className="text-sm font-semibold">标准验收</h2>
                {validation.passed ? <Tag color="success" bordered={false}>验收通过</Tag> : <Tag color="processing" bordered={false}>待验收</Tag>}
            </div>
            <div className="mt-3 flex items-center gap-2 text-xs text-foreground/55">
                <span>成功率</span>
                <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-surface-active">
                    <div className="h-full rounded-full bg-primary" style={{ width: `${Math.min(100, validation.successRate * 100)}%` }} />
                </div>
                <span className="tabular-nums">{formatPercent(validation.successRate)}</span>
            </div>
            <div className="mt-3 space-y-2">
                {validation.checks.map((check) => (
                    <div key={check.key} className="flex items-start gap-2 text-xs">
                        {check.passed ? <CheckCircle2 className="mt-0.5 size-3.5 shrink-0 text-emerald-600 dark:text-emerald-400" /> : <Clock3 className="mt-0.5 size-3.5 shrink-0 text-foreground/35" />}
                        <span className={check.passed ? "text-foreground/70" : "text-foreground/45"}>{check.label}</span>
                        <Tooltip title={`当前 ${String(check.current)} / 要求 ${String(check.required)}`}>
                            <span className="ml-auto shrink-0 tabular-nums text-foreground/40">{String(check.current)}/{String(check.required)}</span>
                        </Tooltip>
                    </div>
                ))}
            </div>
            <p className="mt-3 text-[11px] leading-5 text-foreground/40">标准轮创意全部通过审核，并至少有一条视频生成成功后，才能解锁量产。</p>
        </section>
    );
}
