import { Tag, Tooltip } from "antd";
import { AlertTriangle, CheckCircle2, Circle, Clock3, ImageIcon, Layers, Sparkles } from "lucide-react";

import type { BatchVideoProjectDetail } from "@/services/api/batch-video";
import { parseBatchVideoProfile } from "@/services/api/batch-video";
import { formatPercent, workflowSteps } from "../shared";

function Metric({ label, value, tone = "" }: { label: string; value: string | number; tone?: string }) {
    return (
        <div className="rounded-xl border border-border/70 bg-surface px-4 py-3">
            <div className={`text-xl font-semibold tabular-nums ${tone}`}>{value}</div>
            <div className="mt-0.5 text-[11px] text-foreground/50">{label}</div>
        </div>
    );
}

export function OverviewPanel({ detail, onAnalyze }: { detail: BatchVideoProjectDetail; onAnalyze: () => void }) {
    const { project, settings, counts, validation, sellingPoints, referenceAssets } = detail;
    const profile = parseBatchVideoProfile(project);
    const analysisRunning = project.analysisStatus === "queued" || project.analysisStatus === "running";
    return (
        <div className="grid gap-5 xl:grid-cols-[1.35fr_0.65fr]">
            <div className="space-y-5">
                <section className="rounded-xl border border-border/70 bg-surface p-4">
                    <div className="flex items-center justify-between gap-3">
                        <h2 className="text-sm font-semibold">流程进度</h2>
                        <span className="text-[11px] text-foreground/45">
                            {settings.durationSeconds}s · {settings.aspectRatio} · 标准轮 {settings.standardCreativeCount} 条
                        </span>
                    </div>
                    <div className="mt-4 space-y-3">
                        {workflowSteps(detail).map((step, index) => (
                            <div key={step.title} className="flex items-start gap-3">
                                <div className={`mt-0.5 grid size-6 shrink-0 place-items-center rounded-full border ${step.done ? "border-emerald-500/40 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400" : step.active ? "border-primary/50 bg-primary/10 text-primary" : "border-border/70 text-foreground/30"}`}>
                                    {step.done ? <CheckCircle2 className="size-3.5" /> : step.active ? <Sparkles className="size-3.5" /> : <Circle className="size-3" />}
                                </div>
                                <div className="min-w-0 flex-1">
                                    <div className="flex items-center gap-2 text-sm font-medium">
                                        <span>{index + 1}. {step.title}</span>
                                        {step.inProgress ? <Tag color="processing" bordered={false}>进行中</Tag> : step.active ? <Tag bordered={false}>下一步</Tag> : null}
                                    </div>
                                    <p className="text-xs text-foreground/50">{step.description}</p>
                                </div>
                            </div>
                        ))}
                    </div>
                </section>

                <section className="grid grid-cols-2 gap-3 sm:grid-cols-4">
                    <Metric label="创意总数" value={counts.creativeTotal} />
                    <Metric label="已通过审核" value={counts.approved} tone="text-emerald-600 dark:text-emerald-400" />
                    <Metric label="生成成功" value={counts.succeeded} tone="text-emerald-600 dark:text-emerald-400" />
                    <Metric label="生成失败" value={counts.failed} tone={counts.failed > 0 ? "text-rose-600 dark:text-rose-400" : ""} />
                </section>

                <section className="rounded-xl border border-border/70 bg-surface p-4">
                    <div className="flex items-center justify-between">
                        <h2 className="text-sm font-semibold">产品档案</h2>
                        {analysisRunning ? <Tag color="processing" bordered={false}>分析中</Tag> : project.analysisStatus === "failed" ? <Tag color="error" bordered={false}>分析失败</Tag> : null}
                    </div>
                    {project.analysisStatus === "failed" && project.analysisError ? (
                        <div className="mt-3 flex items-start gap-2 rounded-lg bg-rose-500/10 p-3 text-xs text-rose-700 dark:text-rose-300">
                            <AlertTriangle className="mt-0.5 size-3.5 shrink-0" />
                            <span>{project.analysisError}</span>
                        </div>
                    ) : null}
                    {profile.productSummary ? (
                        <p className="mt-3 text-sm leading-6 text-foreground/75">{profile.productSummary}</p>
                    ) : (
                        <p className="mt-3 text-xs text-foreground/45">还没有产品摘要。上传素材后点击“分析素材”，由策划模型生成。</p>
                    )}
                    {profile.complianceNotes && profile.complianceNotes.length > 0 ? (
                        <div className="mt-3 flex flex-wrap gap-1.5">
                            {profile.complianceNotes.map((note, index) => (
                                <Tag key={index} color="warning" bordered={false}>{note}</Tag>
                            ))}
                        </div>
                    ) : null}
                    {project.analysisStatus === "failed" ? (
                        <button type="button" className="mt-3 text-xs font-medium text-primary hover:underline" onClick={onAnalyze}>
                            重新分析
                        </button>
                    ) : null}
                </section>

                <section className="rounded-xl border border-border/70 bg-surface p-4">
                    <div className="flex items-center justify-between">
                        <h2 className="text-sm font-semibold">卖点库</h2>
                        <span className="text-[11px] text-foreground/45">{sellingPoints.length} 条</span>
                    </div>
                    {sellingPoints.length === 0 ? (
                        <p className="mt-3 text-xs text-foreground/45">暂无卖点。分析素材或手动补充卖点后再策划。</p>
                    ) : (
                        <div className="mt-3 space-y-2">
                            {sellingPoints.slice(0, 20).map((point) => (
                                <div key={point.id} className="flex items-start gap-2 text-xs">
                                    <span className="mt-1 size-1.5 shrink-0 rounded-full bg-primary/60" />
                                    <span className="text-foreground/75">{point.canonicalText}</span>
                                    <span className="ml-auto shrink-0 text-foreground/35">{Math.round(point.confidence * 100)}%</span>
                                </div>
                            ))}
                        </div>
                    )}
                </section>
            </div>

            <div className="space-y-5">
                <section className="rounded-xl border border-border/70 bg-surface p-4">
                    <h2 className="text-sm font-semibold">标准验收</h2>
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
                </section>

                <section className="rounded-xl border border-border/70 bg-surface p-4">
                    <h2 className="text-sm font-semibold">资产概况</h2>
                    <div className="mt-3 grid grid-cols-2 gap-3 text-xs">
                        <div className="flex items-center gap-2 rounded-lg bg-surface-active/50 px-3 py-2">
                            <ImageIcon className="size-4 text-foreground/45" />
                            <div><div className="font-semibold">{referenceAssets.length}</div><div className="text-foreground/45">参考图</div></div>
                        </div>
                        <div className="flex items-center gap-2 rounded-lg bg-surface-active/50 px-3 py-2">
                            <Layers className="size-4 text-foreground/45" />
                            <div><div className="font-semibold">{detail.materialBatches.length}</div><div className="text-foreground/45">素材批次</div></div>
                        </div>
                    </div>
                </section>

                <section className="rounded-xl border border-border/70 bg-surface p-4">
                    <h2 className="text-sm font-semibold">策划轮次</h2>
                    {detail.rounds.length === 0 ? (
                        <p className="mt-3 text-xs text-foreground/45">还没有策划轮次。</p>
                    ) : (
                        <div className="mt-3 space-y-2">
                            {detail.rounds.slice(-6).reverse().map((round) => (
                                <div key={round.id} className="flex items-center justify-between gap-2 text-xs">
                                    <span className="text-foreground/70">第 {round.roundNo} 轮 · {round.mode === "mass" ? "量产" : "标准"} · {round.requestedCount} 条</span>
                                    <Tag bordered={false} color={round.status === "failed" ? "error" : round.status === "reviewing" ? "success" : "processing"}>{round.status === "reviewing" ? "待审核" : round.status === "failed" ? "失败" : "进行中"}</Tag>
                                </div>
                            ))}
                        </div>
                    )}
                </section>
            </div>
        </div>
    );
}
