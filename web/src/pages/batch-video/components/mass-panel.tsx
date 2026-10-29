import { Button, Tag } from "antd";
import { CheckCircle2, Clapperboard, Clock3, LockKeyhole, Rocket } from "lucide-react";

import type { BatchVideoProjectDetail } from "@/services/api/batch-video";

export function MassPanel({
    detail,
    actionLoading,
    onGenerate,
    onUnlock,
}: {
    detail: BatchVideoProjectDetail;
    actionLoading: string;
    onGenerate: () => void;
    onUnlock: () => void;
}) {
    const { project, validation, counts, settings } = detail;
    const unlocked = Boolean(project.massUnlockedAt);
    const unmet = validation.checks.filter((check) => !check.passed);
    return (
        <div className="grid gap-5 xl:grid-cols-[1.1fr_0.9fr]">
            <div className="space-y-5">
                <section className="rounded-xl border border-border/70 bg-surface p-5">
                    <div className="flex items-start gap-3">
                        <span className={`grid size-10 shrink-0 place-items-center rounded-full ${unlocked ? "bg-emerald-500/10 text-emerald-600 dark:text-emerald-400" : "bg-primary/10 text-primary"}`}>
                            {unlocked ? <Rocket className="size-5" /> : validation.passed ? <CheckCircle2 className="size-5" /> : <LockKeyhole className="size-5" />}
                        </span>
                        <div className="min-w-0">
                            <div className="flex flex-wrap items-center gap-2">
                                <h2 className="text-base font-semibold">量产放量</h2>
                                <Tag color={unlocked ? "success" : validation.passed ? "processing" : "default"} bordered={false}>
                                    {unlocked ? "已解锁" : validation.passed ? "待解锁" : "未达标"}
                                </Tag>
                            </div>
                            <p className="mt-1 text-xs leading-5 text-foreground/55">
                                {unlocked
                                    ? "量产已解锁，可按卖点批量生成创意并提交视频任务。"
                                    : validation.passed
                                      ? "标准验收已通过。确认验收结果后解锁量产，单轮最多 1000 条创意。"
                                      : "完成标准验收后解锁量产。下方列出尚未满足的验收条件。"}
                            </p>
                        </div>
                    </div>

                    {unmet.length > 0 ? (
                        <div className="mt-4 space-y-1.5 rounded-lg bg-surface-active/40 p-3">
                            {unmet.map((check) => (
                                <div key={check.key} className="flex items-center gap-2 text-xs text-foreground/55">
                                    <Clock3 className="size-3.5 shrink-0 text-foreground/35" />
                                    <span>{check.label}</span>
                                    <span className="ml-auto tabular-nums text-foreground/35">{String(check.current)}/{String(check.required)}</span>
                                </div>
                            ))}
                        </div>
                    ) : null}

                    <div className="mt-4 flex flex-wrap gap-2">
                        {unlocked ? (
                            <Button type="primary" icon={<Clapperboard className="size-4" />} onClick={onGenerate}>
                                生成量产创意
                            </Button>
                        ) : (
                            <Button type="primary" icon={<Rocket className="size-4" />} disabled={!validation.passed} loading={actionLoading === "unlock"} onClick={onUnlock}>
                                解锁量产
                            </Button>
                        )}
                    </div>
                </section>

                <section className="grid grid-cols-2 gap-3 sm:grid-cols-4">
                    <div className="rounded-xl border border-border/70 bg-surface px-4 py-3">
                        <div className="text-xl font-semibold tabular-nums">{counts.creativeTotal}</div>
                        <div className="mt-0.5 text-[11px] text-foreground/50">创意总数</div>
                    </div>
                    <div className="rounded-xl border border-border/70 bg-surface px-4 py-3">
                        <div className="text-xl font-semibold tabular-nums text-emerald-600 dark:text-emerald-400">{counts.approved}</div>
                        <div className="mt-0.5 text-[11px] text-foreground/50">已通过审核</div>
                    </div>
                    <div className="rounded-xl border border-border/70 bg-surface px-4 py-3">
                        <div className="text-xl font-semibold tabular-nums text-emerald-600 dark:text-emerald-400">{counts.succeeded}</div>
                        <div className="mt-0.5 text-[11px] text-foreground/50">生成成功</div>
                    </div>
                    <div className="rounded-xl border border-border/70 bg-surface px-4 py-3">
                        <div className={`text-xl font-semibold tabular-nums ${counts.failed > 0 ? "text-rose-600 dark:text-rose-400" : ""}`}>{counts.failed}</div>
                        <div className="mt-0.5 text-[11px] text-foreground/50">生成失败</div>
                    </div>
                </section>
            </div>

            <section className="rounded-xl border border-border/70 bg-surface p-4">
                <h2 className="text-sm font-semibold">量产参数</h2>
                <div className="mt-3 space-y-2 text-xs text-foreground/60">
                    <div className="flex items-center justify-between gap-3"><span>视频时长</span><span className="tabular-nums">{settings.durationSeconds}s</span></div>
                    <div className="flex items-center justify-between gap-3"><span>画面比例</span><span className="tabular-nums">{settings.aspectRatio}</span></div>
                    <div className="flex items-center justify-between gap-3"><span>标准轮创意数量</span><span className="tabular-nums">{settings.standardCreativeCount} 条</span></div>
                    <div className="flex items-center justify-between gap-3"><span>量产状态</span><span>{unlocked ? "已解锁，单轮最多 1000 条" : "未解锁，单轮最多 20 条"}</span></div>
                </div>
                <p className="mt-3 border-t border-border/60 pt-3 text-[11px] leading-5 text-foreground/40">量产仍受模型、预算和服务端队列约束；生成任务可在“标准验收”步骤查看进度。</p>
            </section>
        </div>
    );
}
