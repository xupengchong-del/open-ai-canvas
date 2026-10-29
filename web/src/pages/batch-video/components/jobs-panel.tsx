import { App, Button, DatePicker, Modal, Progress, Tag } from "antd";
import dayjs, { type Dayjs } from "dayjs";
import { AlertCircle, CalendarClock, ExternalLink, History, Loader2, Pause, Play, RotateCcw, Video, XCircle } from "lucide-react";
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
    onCancelPending,
    onPause,
    onResume,
    onStart,
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
    onCancelPending: () => void;
    onPause: () => void;
    onResume: () => void;
    onStart: (scheduledAt?: string) => void;
}) {
    const { message } = App.useApp();
    const [attemptJob, setAttemptJob] = useState<BatchVideoGenerationJob | null>(null);
    const [attempts, setAttempts] = useState<BatchVideoAttempt[]>([]);
    const [attemptsLoading, setAttemptsLoading] = useState(false);
    const [scheduleOpen, setScheduleOpen] = useState(false);
    const [scheduleAt, setScheduleAt] = useState<Dayjs | null>(null);
    const { counts, project } = detail;
    const creativeById = new Map(detail.creatives.map((creative) => [creative.id, creative]));
    const paused = project.status === "paused";
    const finished = counts.succeeded + counts.failed + counts.cancelled;
    const generationPercent = counts.jobsTotal > 0 ? Math.round((finished / counts.jobsTotal) * 100) : 0;
    const generationActive = counts.waiting + counts.running > 0;
    const pendingTotal = counts.submitted + counts.waiting;
    const confirmSchedule = () => {
        if (!scheduleAt || !scheduleAt.isAfter(dayjs())) {
            message.warning("请选择晚于当前时间的开始时间");
            return;
        }
        setScheduleOpen(false);
        onStart(scheduleAt.toISOString());
    };
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
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-4 xl:grid-cols-7">
                <JobStat label="任务总数" value={counts.jobsTotal} />
                <JobStat label="待开始" value={counts.submitted} tone={counts.submitted > 0 ? "text-amber-600 dark:text-amber-400" : ""} />
                <JobStat label="等待 / 运行" value={counts.waiting + counts.running} tone="text-amber-600 dark:text-amber-400" />
                <JobStat label="成功" value={counts.succeeded} tone="text-emerald-600 dark:text-emerald-400" />
                <JobStat label="失败" value={counts.failed} tone={counts.failed > 0 ? "text-rose-600 dark:text-rose-400" : ""} />
                <JobStat label="重试成功" value={counts.retrySucceeded} tone="text-emerald-600 dark:text-emerald-400" />
                <JobStat label="重试失败" value={counts.retryFailed} tone={counts.retryFailed > 0 ? "text-rose-600 dark:text-rose-400" : ""} />
            </div>

            {counts.submitted > 0 ? (
                <div className="flex flex-col gap-3 rounded-xl border border-amber-500/30 bg-amber-500/10 px-4 py-3 lg:flex-row lg:items-center lg:justify-between">
                    <div className="text-xs leading-5 text-amber-800 dark:text-amber-200">
                        <div className="text-sm font-semibold">已提交 {counts.submitted} 条，等待开始</div>
                        <div>开始前不会调用模型；开始后按模型优先级从高到低调度。</div>
                        {project.scheduledStartAt ? <div>已定时：{new Date(project.scheduledStartAt).toLocaleString("zh-CN")}（到点自动开始）</div> : null}
                    </div>
                    <div className="flex flex-wrap items-center gap-2">
                        <Button type="primary" size="small" icon={<Play className="size-3.5" />} loading={actionLoading === "start-generation"} onClick={() => onStart()}>
                            {project.scheduledStartAt ? "立即开始" : "开始生成"}
                        </Button>
                        <Button size="small" icon={<CalendarClock className="size-3.5" />} disabled={actionLoading === "start-generation"} onClick={() => { setScheduleAt(project.scheduledStartAt ? dayjs(project.scheduledStartAt) : dayjs().add(1, "hour").startOf("minute")); setScheduleOpen(true); }}>
                            {project.scheduledStartAt ? "修改定时" : "定时生成"}
                        </Button>
                    </div>
                </div>
            ) : null}

            {counts.jobsTotal > 0 ? (
                <div className="rounded-xl border border-border/70 bg-surface px-4 py-3">
                    <div className="flex items-center justify-between text-xs text-foreground/55">
                        <span>生成本轮进度</span>
                        <span className="tabular-nums">已完成 {finished}/{counts.jobsTotal}</span>
                    </div>
                    <Progress
                        className="!mb-0 !mt-1"
                        percent={generationPercent}
                        status={generationActive ? "active" : counts.failed > 0 ? "exception" : "success"}
                        size="small"
                    />
                </div>
            ) : null}

            <div className="flex flex-col gap-3 rounded-xl border border-border/70 bg-surface p-3 lg:flex-row lg:items-center lg:justify-between">
                <div className="flex flex-wrap items-center gap-2 text-xs text-foreground/55">
                    <Video className="size-4" />
                    <span>{counts.approved} 条已通过创意可提交</span>
                    {paused ? <Tag color="warning" bordered={false}>项目已暂停</Tag> : null}
                    {project.massUnlockedAt ? <Tag color="success" bordered={false}>量产已解锁</Tag> : null}
                </div>
                <div className="flex flex-wrap items-center gap-2">
                    <Button size="small" icon={paused ? <Play className="size-3.5" /> : <Pause className="size-3.5" />} onClick={paused ? onResume : onPause} loading={actionLoading === "pause" || actionLoading === "resume"}>
                        {paused ? "继续调度" : "暂停调度"}
                    </Button>
                    <Button size="small" danger icon={<XCircle className="size-3.5" />} disabled={pendingTotal === 0} loading={actionLoading === "cancel-pending"} onClick={onCancelPending}>
                        取消未开始项
                    </Button>
                    <Button size="small" icon={<RotateCcw className="size-3.5" />} disabled={counts.failed === 0} loading={actionLoading === "retry-failed"} onClick={onRetryFailed}>
                        重试全部失败
                    </Button>
                </div>
            </div>

            <div className={`overflow-hidden rounded-xl border border-border/70 bg-surface ${loading ? "pointer-events-none opacity-60" : ""}`}>
                {jobs.length === 0 ? (
                    <div className="px-4 py-12 text-center">
                        <p className="text-sm font-medium">还没有生成任务</p>
                        <p className="mt-1 text-xs text-foreground/45">审核通过创意后，点击页面上方“提交生成”；提交后还需要点击“开始生成”或设置定时。离开页面不会中断已开始的任务。</p>
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
                                    const active = job.status === "waiting" || job.status === "dispatching" || job.status === "queued" || job.status === "running";
                                    return (
                                        <tr key={job.id} className="border-t border-border/50 align-top">
                                            <td className="max-w-[220px] px-3 py-2.5">
                                                <div className="line-clamp-2 font-medium">{creative?.title || `创意 ${job.creativeId.slice(0, 6)}`}</div>
                                                <div className="mt-0.5 text-[10px] text-foreground/35">{creative ? `${creative.durationSeconds}s · ${creative.aspectRatio}` : job.creativeId.slice(0, 8)}</div>
                                            </td>
                                            <td className="px-3 py-2.5 text-foreground/65">
                                                <div>{job.modelId || "自动路由"}</div>
                                                <div className="mt-0.5 text-[10px] text-foreground/35">优先级 {job.modelPriority || 50}</div>
                                            </td>
                                            <td className="px-3 py-2.5 text-foreground/55">{job.mode === "multi" ? "多模型" : "单模型"}</td>
                                            <td className="px-3 py-2.5">
                                                <Tag bordered={false} color={jobStatusColor(job.status)} icon={active ? <Loader2 className="size-3 animate-spin" /> : undefined}>
                                                    {jobStatusLabel(job.status)}
                                                </Tag>
                                                {active ? (
                                                    <div className="max-w-[140px]" title={job.stage || undefined}>
                                                        <Progress
                                                            className="!mb-0 !mt-1"
                                                            percent={job.status === "running" ? Math.min(95, Math.max(5, job.progress || 5)) : job.status === "dispatching" ? 10 : 5}
                                                            status="active"
                                                            showInfo={false}
                                                            size="small"
                                                        />
                                                    </div>
                                                ) : null}
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
            <Modal
                open={scheduleOpen}
                title="定时开始生成"
                okText="保存定时"
                onOk={confirmSchedule}
                onCancel={() => setScheduleOpen(false)}
            >
                <div className="grid gap-3 py-2">
                    <p className="text-sm text-foreground/60">到达设定时间后，已提交的 {counts.submitted} 条任务会自动开始；开始前仍可取消未开始项。</p>
                    <DatePicker
                        showTime={{ format: "HH:mm" }}
                        format="YYYY-MM-DD HH:mm"
                        value={scheduleAt}
                        disabledDate={(value) => value.isBefore(dayjs(), "day")}
                        onChange={(value) => setScheduleAt(value)}
                        className="w-full"
                    />
                </div>
            </Modal>
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
