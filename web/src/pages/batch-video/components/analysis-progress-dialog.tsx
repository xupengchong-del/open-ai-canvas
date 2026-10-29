import { Alert, Button, Checkbox, Modal, Progress } from "antd";
import { AlertCircle, Check, Loader2 } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import { resourceFileUrl } from "@/services/api/resources";
import type { BatchVideoAnalysisTask, BatchVideoReferenceAsset } from "@/services/api/batch-video";
import { cn } from "@/lib/utils";

function formatElapsed(startedAt: string | null | undefined, now: number): string {
    const started = startedAt ? new Date(startedAt).getTime() : now;
    const seconds = Math.max(0, Math.floor((now - started) / 1000));
    if (seconds < 60) return `${seconds} 秒`;
    const minutes = Math.floor(seconds / 60);
    return `${minutes} 分 ${seconds % 60} 秒`;
}

function assetStateLabel(status: string): string {
    if (status === "succeeded") return "已完成";
    if (status === "failed") return "失败";
    if (status === "cancelled") return "已中止";
    if (status === "running" || status === "queued") return "分析中";
    return "待分析";
}

export function AnalysisProgressDialog({
    open,
    task,
    assets,
    analysisError,
    awaitingDecision,
    aborting,
    continuing,
    onAbort,
    onContinue,
}: {
    open: boolean;
    task?: BatchVideoAnalysisTask;
    assets: BatchVideoReferenceAsset[];
    analysisError?: string;
    awaitingDecision: boolean;
    aborting: boolean;
    continuing: boolean;
    onAbort: (askAgain: boolean) => void;
    onContinue: (askAgain: boolean) => void;
}) {
    const [now, setNow] = useState(Date.now());
    const [askAgain, setAskAgain] = useState(true);
    useEffect(() => {
        if (!open) return;
        setNow(Date.now());
        const timer = window.setInterval(() => setNow(Date.now()), 1000);
        return () => window.clearInterval(timer);
    }, [open]);
    useEffect(() => {
        if (awaitingDecision) setAskAgain(true);
    }, [awaitingDecision, task?.assetId]);
    const uploads = useMemo(() => assets.filter((asset) => asset.source !== "generated" && asset.resourceId), [assets]);
    const done = uploads.filter((asset) => asset.analysisStatus === "succeeded" || asset.analysisStatus === "failed").length;
    const failed = uploads.filter((asset) => asset.analysisStatus === "failed").length;
    const runningCount = uploads.filter((asset) => asset.analysisStatus === "running" || asset.analysisStatus === "queued").length;
    const total = uploads.length;
    const current = uploads.find((asset) => asset.id === task?.assetId) || uploads.find((asset) => asset.analysisStatus === "running" || asset.analysisStatus === "queued");
    const currentIndex = current ? uploads.findIndex((asset) => asset.id === current.id) + 1 : done;
    const percent = total > 0 ? Math.round((done / total) * 100) : Math.min(95, Math.max(5, task?.progress || 5));
    const elapsed = formatElapsed(task?.startedAt || task?.createdAt, now);
    return (
        <Modal
            open={open}
            title="素材分析"
            width={620}
            closable={false}
            maskClosable={false}
            keyboard={false}
            footer={
                awaitingDecision ? (
                    <div className="flex w-full flex-wrap items-center justify-between gap-3">
                        <Checkbox checked={askAgain} onChange={(event) => setAskAgain(event.target.checked)}>
                            下次失败时仍询问
                        </Checkbox>
                        <div className="flex items-center gap-2">
                            <Button danger loading={aborting} disabled={continuing} onClick={() => onAbort(askAgain)}>
                                中止分析
                            </Button>
                            <Button type="primary" loading={continuing} disabled={aborting} onClick={() => onContinue(askAgain)}>
                                继续分析剩余图片
                            </Button>
                        </div>
                    </div>
                ) : (
                    <Button danger loading={aborting} onClick={() => onAbort(true)}>
                        中止分析
                    </Button>
                )
            }
        >
            <div className="space-y-4 py-2">
                {awaitingDecision ? (
                    <Alert
                        type="error"
                        showIcon
                        title="图片分析失败，等待确认"
                        description={
                            <div className="space-y-1">
                                <div>{analysisError || current?.analysisError || "未知原因"}</div>
                                <div className="text-foreground/55">可继续分析剩余图片，失败图片会保留失败标记；也可以中止后重新分析。已完成结果都会被保留。</div>
                            </div>
                        }
                    />
                ) : (
                    <p className="text-xs leading-5 text-foreground/55">
                        图片按后台设置的并行数同时调用视觉模型，单张结果独立保存。分析完成前窗口保持打开，可随时中止；中止后素材和已完成结果会保留。
                    </p>
                )}
                <div>
                    <div className="flex items-center justify-between text-xs text-foreground/55">
                        <span>
                            {awaitingDecision
                                ? `已分析 ${done}/${total} 张，等待确认`
                                : runningCount > 1
                                  ? `并行分析中：运行 ${runningCount} 张 · 已完成 ${done}/${total}`
                                  : current
                                    ? `正在分析第 ${currentIndex}/${total} 张：${current.title || "未命名素材"}`
                                    : `已分析 ${done}/${total} 张`}
                        </span>
                        <span className="tabular-nums">已运行 {elapsed}</span>
                    </div>
                    <Progress className="!mb-0 !mt-1" percent={percent} status={failed > 0 ? "exception" : "active"} />
                    <div className="mt-1 flex items-center justify-between text-[11px] text-foreground/40">
                        <span>已完成 {done}/{total} 张{failed > 0 ? ` · 失败 ${failed} 张` : ""}</span>
                        <span>每张结果会写入参考图描述和卖点</span>
                    </div>
                </div>
                {total > 0 ? (
                    <div className="grid max-h-72 grid-cols-5 gap-2 overflow-auto rounded-lg border border-border/60 bg-surface-active/25 p-2 sm:grid-cols-7">
                        {uploads.map((asset, index) => {
                            const status = asset.analysisStatus || "pending";
                            const running = status === "running" || status === "queued";
                            return (
                                <div
                                    key={asset.id}
                                    className={cn(
                                        "relative aspect-square overflow-hidden rounded-md border bg-surface",
                                        running ? "border-primary ring-2 ring-primary/30" : status === "failed" ? "border-rose-500/70" : status === "succeeded" ? "border-emerald-500/40" : "border-border/60",
                                    )}
                                    title={`第 ${index + 1} 张 · ${asset.title || "未命名素材"} · ${assetStateLabel(status)}${asset.analysisError ? `：${asset.analysisError}` : ""}`}
                                >
                                    <img src={resourceFileUrl(asset.resourceId)} alt={asset.title || "素材"} className={cn("h-full w-full object-cover", status === "pending" ? "opacity-45" : "")} loading="lazy" />
                                    <span className="absolute left-1 top-1 rounded bg-black/55 px-1 text-[9px] tabular-nums text-white">{index + 1}</span>
                                    <span className={cn("absolute bottom-1 right-1 grid size-4 place-items-center rounded-full text-white", status === "succeeded" ? "bg-emerald-600" : status === "failed" ? "bg-rose-600" : running ? "bg-black/55" : "bg-black/35")}>
                                        {status === "succeeded" ? <Check className="size-2.5" /> : status === "failed" ? <AlertCircle className="size-2.5" /> : running ? <Loader2 className="size-2.5 animate-spin" /> : null}
                                    </span>
                                </div>
                            );
                        })}
                    </div>
                ) : (
                    <div className="rounded-lg border border-border/60 bg-surface-active/25 px-4 py-6 text-center text-xs text-foreground/45">正在分析卖点文档，不涉及图片。</div>
                )}
            </div>
        </Modal>
    );
}
