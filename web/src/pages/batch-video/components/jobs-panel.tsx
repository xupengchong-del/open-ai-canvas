import { App, Button, Modal, Tag } from "antd";
import { AlertCircle, ExternalLink, History, Loader2, Pause, Play, RefreshCw, Rocket, RotateCcw, Video, XCircle } from "lucide-react";
import { useState } from "react";

import { PaginationBar } from "@/components/layout/workspace-page";
import { resourceFileUrl } from "@/services/api/resources";
import { batchVideoApi, type BatchVideoAttempt, type BatchVideoGenerationJob, type BatchVideoProjectDetail } from "@/services/api/batch-video";
import { jobStatusColor, jobStatusLabel } from "../shared";

function JobStat({ label, value, tone = "" }: { label: string; value: number; tone?: string }) {
    return (
        <div className="rounded-xl border border-border/70 bg-surface px-4 py-3">
            <div className={`text-xl font-semibold tabular-nums ${tone}`}>{value}</div>
            <div className="mt-0.5 text-[11px] text-foreground/50">{label}</div>
        </div>
    );
}

export function JobsPanel({
    detail,
    jobs,
    page,
    pageSize,
    loading,
    actionLoading,
    onPageChange,
    onRetry,
    onRetryFailed,
    onSubmit,
    onCancelPending,
    onPause,
    onResume,
}: {
    detail: BatchVideoProjectDetail;
    jobs: BatchVideoGenerationJob[];
    page: number;
    pageSize: number;
    loading: boolean;
    actionLoading: string;
    onPageChange: (page: number, pageSize: number) => void;
    onRetry: (job: BatchVideoGenerationJob) => void;
    onRetryFailed: () => void;
    onSubmit: () => void;
    onCancelPending: () => void;
    onPause: () => void;
    onResume: () => void;
}) {
    const { message } = App.useApp();
    const [attemptJob, setAttemptJob] = useState<BatchVideoGenerationJob | null>(null);
    const [attempts, setAttempts] = useState<BatchVideoAttempt[]>([]);
    const [attemptsLoading, setAttemptsLoading] = useState(false);
    const { counts, project } = detail;
    const creativeById = new Map(detail.creatives.map((creative) => [creative.id, creative]));
    const paused = project.status === "paused";
    const showAttempts = async (job: BatchVideoGenerationJob) => {
        setAttemptJob(job);
        setAttemptsLoading(true);
        try {
            setAttempts(await batchVideoApi.listAttempts(project.id, job.id));
        } catch (error) {
            message.error(error instanceof Error ? error.message : "加载任务历史失败");
        } finally {
            setAttemptsLoading(false);
        }
    };
    return (
        <div className="space-y-4">
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-4 xl:grid-cols-6">
                <JobStat label="任务总数" value={counts.jobsTotal} />
                <JobStat label="等待 / 运行" value={counts.waiting + counts.running} tone="text-amber-600 dark:text-amber-400" />
                <JobStat label="成功" value={counts.succeeded} tone="text-emerald-600 dark:text-emerald-400" />
                <JobStat label="失败" value={counts.failed} tone={counts.failed > 0 ? "text-rose-600 dark:text-rose-400" : ""} />
                <JobStat label="重试成功" value={counts.retrySucceeded} tone="text-emerald-600 dark:text-emerald-400" />
                <JobStat label="重试失败" value={counts.retryFailed} tone={counts.retryFailed > 0 ? "text-rose-600 dark:text-rose-400" : ""} />
            </div>

            <div className="flex flex-col gap-3 rounded-xl border border-border/70 bg-surface p-3 lg:flex-row lg:items-center lg:justify-between">
                <div className="flex flex-wrap items-center gap-2 text-xs text-foreground/55">
                    <Video className="size-4" />
                    <span>{counts.approved} 条已通过创意可提交</span>
                    {paused ? <Tag color="warning" bordered={false}>项目已暂停</Tag> : null}
                    {project.massUnlockedAt ? <Tag color="success" bordered={false}>量产已解锁</Tag> : null}
                </div>
                <div className="flex flex-wrap items-center gap-2">
                    <Button size="small" icon={<RefreshCw className="size-3.5" />} loading={loading} onClick={() => onPageChange(page, pageSize)}>刷新</Button>
                    <Button size="small" icon={paused ? <Play className="size-3.5" /> : <Pause className="size-3.5" />} onClick={paused ? onResume : onPause} loading={actionLoading === "pause" || actionLoading === "resume"}>
                        {paused ? "继续调度" : "暂停调度"}
                    </Button>
                    <Button size="small" danger icon={<XCircle className="size-3.5" />} disabled={counts.waiting === 0} loading={actionLoading === "cancel-pending"} onClick={onCancelPending}>
                        取消未开始项
                    </Button>
                    <Button size="small" icon={<RotateCcw className="size-3.5" />} disabled={counts.failed === 0} loading={actionLoading === "retry-failed"} onClick={onRetryFailed}>
                        重试全部失败
                    </Button>
                    <Button size="small" type="primary" icon={<Rocket className="size-3.5" />} disabled={counts.approved === 0} onClick={onSubmit}>
                        提交生成
                    </Button>
                </div>
            </div>

            <div className="overflow-hidden rounded-xl border border-border/70 bg-surface">
                {jobs.length === 0 ? (
                    <div className="px-4 py-12 text-center">
                        <p className="text-sm font-medium">还没有生成任务</p>
                        <p className="mt-1 text-xs text-foreground/45">审核通过创意后点击“提交生成”。服务端队列会在后台持续运行，离开页面不会中断。</p>
                        {counts.approved > 0 ? <Button className="mt-4" type="primary" icon={<Rocket className="size-4" />} onClick={onSubmit}>提交生成</Button> : null}
                    </div>
                ) : (
                    <div className="overflow-x-auto">
                        <table className="w-full min-w-[860px] text-left text-xs">
                            <thead className="bg-surface-active/40 text-foreground/50">
                                <tr>
                                    <th className="px-3 py-2 font-medium">创意</th>
                                    <th className="px-3 py-2 font-medium">模型</th>
                                    <th className="px-3 py-2 font-medium">模式</th>
                                    <th className="px-3 py-2 font-medium">状态</th>
                                    <th className="px-3 py-2 font-medium">尝试 / 切换</th>
                                    <th className="px-3 py-2 font-medium">错误</th>
                                    <th className="px-3 py-2 text-right font-medium">操作</th>
                                </tr>
                            </thead>
                            <tbody>
                                {jobs.map((job) => {
                                    const creative = creativeById.get(job.creativeId);
                                    const active = job.status === "waiting" || job.status === "queued" || job.status === "running";
                                    return (
                                        <tr key={job.id} className="border-t border-border/50 align-top">
                                            <td className="max-w-[220px] px-3 py-2.5">
                                                <div className="line-clamp-2 font-medium">{creative?.title || `创意 ${job.creativeId.slice(0, 6)}`}</div>
                                                <div className="mt-0.5 text-[10px] text-foreground/35">{creative ? `${creative.durationSeconds}s · ${creative.aspectRatio}` : job.creativeId.slice(0, 8)}</div>
                                            </td>
                                            <td className="px-3 py-2.5 text-foreground/65">{job.modelId || "自动路由"}</td>
                                            <td className="px-3 py-2.5 text-foreground/55">{job.mode === "multi" ? "多模型" : "单模型"}</td>
                                            <td className="px-3 py-2.5">
                                                <Tag bordered={false} color={jobStatusColor(job.status)} icon={active ? <Loader2 className="size-3 animate-spin" /> : undefined}>
                                                    {jobStatusLabel(job.status)}
                                                </Tag>
                                            </td>
                                            <td className="px-3 py-2.5 tabular-nums text-foreground/55">{job.attemptsUsed} / {job.failoverCount}</td>
                                            <td className="max-w-[240px] px-3 py-2.5">
                                                {job.lastError ? (
                                                    <span className="line-clamp-2 text-rose-600 dark:text-rose-400" title={job.lastError}>{job.lastError}</span>
                                                ) : (
                                                    <span className="text-foreground/30">—</span>
                                                )}
                                            </td>
                                            <td className="px-3 py-2.5 text-right">
                                                <div className="inline-flex items-center gap-1">
                                                    {job.successResourceId ? (
                                                        <Button size="small" type="link" icon={<ExternalLink className="size-3.5" />} href={resourceFileUrl(job.successResourceId)} target="_blank">
                                                            成片
                                                        </Button>
                                                    ) : null}
                                                    <Button size="small" type="text" icon={<History className="size-3.5" />} onClick={() => void showAttempts(job)}>
                                                        历史
                                                    </Button>
                                                    {job.status === "failed" || job.status === "cancelled" ? (
                                                        <Button size="small" type="text" icon={<RotateCcw className="size-3.5" />} loading={actionLoading === `retry-${job.id}`} onClick={() => onRetry(job)}>
                                                            重试
                                                        </Button>
                                                    ) : null}
                                                    {job.lastError ? <AlertCircle className="size-3.5 text-foreground/25" aria-hidden /> : null}
                                                </div>
                                            </td>
                                        </tr>
                                    );
                                })}
                            </tbody>
                        </table>
                    </div>
                )}
            </div>
            <PaginationBar current={page} pageSize={pageSize} total={counts.jobsTotal} onChange={onPageChange} itemLabel="条任务" />
            <Modal open={Boolean(attemptJob)} title="任务尝试与模型切换历史" footer={null} width={860} onCancel={() => setAttemptJob(null)}>
                {attemptsLoading ? (
                    <div className="flex items-center justify-center gap-2 py-10 text-sm text-foreground/50"><Loader2 className="size-4 animate-spin" />加载历史</div>
                ) : (
                    <div className="overflow-x-auto py-2">
                        <table className="w-full min-w-[720px] text-left text-xs">
                            <thead className="text-foreground/45"><tr><th className="px-2 py-2">Attempt</th><th className="px-2 py-2">重试轮次</th><th className="px-2 py-2">模型</th><th className="px-2 py-2">任务 ID</th><th className="px-2 py-2">状态</th><th className="px-2 py-2">切换来源 / 错误</th><th className="px-2 py-2">开始 / 结束</th></tr></thead>
                            <tbody>
                                {attempts.map((attempt) => (
                                    <tr key={attempt.id} className="border-t border-border/50 align-top">
                                        <td className="px-2 py-2">#{attempt.attemptNo + 1}</td>
                                        <td className="px-2 py-2">{attempt.retryRound}</td>
                                        <td className="px-2 py-2">{attempt.modelId}</td>
                                        <td className="max-w-[130px] truncate px-2 py-2" title={attempt.taskId}>{attempt.taskId}</td>
                                        <td className="px-2 py-2"><Tag bordered={false} color={jobStatusColor(attempt.status)}>{jobStatusLabel(attempt.status)}</Tag></td>
                                        <td className="max-w-[240px] px-2 py-2">{attempt.switchedFromModelId ? <div>从 {attempt.switchedFromModelId} 切换</div> : null}<div className="text-rose-600 dark:text-rose-400">{attempt.error}</div></td>
                                        <td className="px-2 py-2 text-foreground/45"><div>{attempt.startedAt ? new Date(attempt.startedAt).toLocaleString("zh-CN") : "—"}</div><div>{attempt.finishedAt ? new Date(attempt.finishedAt).toLocaleString("zh-CN") : "—"}</div></td>
                                    </tr>
                                ))}
                                {attempts.length === 0 ? <tr><td colSpan={7} className="px-2 py-10 text-center text-foreground/45">还没有 attempt 记录</td></tr> : null}
                            </tbody>
                        </table>
                    </div>
                )}
            </Modal>
        </div>
    );
}
