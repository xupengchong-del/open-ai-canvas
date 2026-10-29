import { Button, Modal, Progress } from "antd";
import { useEffect, useState } from "react";

export type TaskProgressState = {
    status: string;
    stage?: string;
    progress?: number;
    startedAt?: string | null;
};

function percentOf(state: TaskProgressState): number {
    if (state.status === "succeeded" || state.status === "failed" || state.status === "cancelled") return 100;
    if (state.status === "running") return Math.min(95, Math.max(5, state.progress ?? 50));
    return 5;
}

function progressStatusOf(state: TaskProgressState): "success" | "exception" | "active" | "normal" {
    if (state.status === "succeeded") return "success";
    if (state.status === "failed") return "exception";
    if (state.status === "running" || state.status === "queued") return "active";
    return "normal";
}

function formatElapsed(startedAt: string | null | undefined, now: number): string {
    const started = startedAt ? new Date(startedAt).getTime() : now;
    const seconds = Math.max(0, Math.floor((now - started) / 1000));
    if (seconds < 60) return `${seconds} 秒`;
    const minutes = Math.floor(seconds / 60);
    return `${minutes} 分 ${seconds % 60} 秒`;
}

export function TaskProgressDialog({
    open,
    title,
    description,
    state,
    aborting,
    onAbort,
    onClose,
}: {
    open: boolean;
    title: string;
    description: string;
    state: TaskProgressState;
    aborting?: boolean;
    onAbort?: () => void;
    onClose: () => void;
}) {
    const [now, setNow] = useState(Date.now());
    useEffect(() => {
        if (!open) return;
        setNow(Date.now());
        const timer = window.setInterval(() => setNow(Date.now()), 1000);
        return () => window.clearInterval(timer);
    }, [open]);
    const active = state.status === "queued" || state.status === "running";
    const stage = state.stage || (state.status === "queued" ? "排队中" : state.status === "running" ? "模型处理中" : "");
    return (
        <Modal
            open={open}
            title={title}
            onCancel={onClose}
            width={480}
            footer={
                <>
                    <Button onClick={onClose}>后台运行</Button>
                    {active && onAbort ? (
                        <Button danger loading={aborting} onClick={onAbort}>
                            中止任务
                        </Button>
                    ) : null}
                </>
            }
        >
            <div className="space-y-3 py-2">
                <p className="text-xs leading-5 text-foreground/55">{description}</p>
                <Progress percent={percentOf(state)} status={progressStatusOf(state)} />
                <div className="flex items-center justify-between text-xs text-foreground/55">
                    <span>{stage}</span>
                    <span className="tabular-nums">{active ? `已运行 ${formatElapsed(state.startedAt, now)}` : "已完成"}</span>
                </div>
                <p className="text-[11px] text-foreground/40">关闭此窗口不会中断任务，服务端队列会在后台继续运行。</p>
            </div>
        </Modal>
    );
}
