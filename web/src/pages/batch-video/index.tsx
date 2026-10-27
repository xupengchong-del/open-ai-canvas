import { Button, Tag } from "antd";
import { ArrowRight, Clapperboard, Plus, RefreshCw } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { useNavigate } from "react-router";

import { PageHeader, PaginationBar, WorkspacePage } from "@/components/layout/workspace-page";
import { WorkspaceErrorState, WorkspaceState } from "@/components/layout/workspace-state";
import { batchVideoApi, batchVideoStageLabel, type BatchVideoProjectSummary } from "@/services/api/batch-video";
import { cn } from "@/lib/utils";

function stageTone(stage: string): "default" | "processing" | "success" | "warning" {
    if (stage === "completed") return "success";
    if (stage === "standard_reviewing" || stage === "mass_reviewing" || stage === "draft") return "warning";
    if (stage.includes("planning") || stage === "analyzing" || stage.includes("generating")) return "processing";
    return "default";
}

function SummaryCard({ item, onOpen }: { item: BatchVideoProjectSummary; onOpen: () => void }) {
    const { project, counts, settings, validation } = item;
    const finished = counts.succeeded + counts.failed;
    return (
        <button
            type="button"
            onClick={onOpen}
            className="group flex w-full flex-col gap-4 rounded-xl border border-border/70 bg-surface p-5 text-left transition-colors hover:border-primary/40 hover:bg-surface-active/40"
        >
            <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                    <div className="flex items-center gap-2">
                        <h2 className="truncate text-base font-semibold text-foreground">{project.name}</h2>
                        <Tag color={stageTone(project.stage)} bordered={false}>
                            {batchVideoStageLabel(project.stage)}
                        </Tag>
                    </div>
                    <p className="mt-1 truncate text-xs text-foreground/55">{project.productName || "未填写产品名称"}</p>
                </div>
                <ArrowRight className="size-4 shrink-0 text-foreground/35 transition-transform group-hover:translate-x-0.5" />
            </div>
            <div className="grid grid-cols-4 gap-2 text-center">
                <div className="rounded-lg bg-surface-active/60 px-2 py-2">
                    <div className="text-lg font-semibold tabular-nums">{counts.creativeTotal}</div>
                    <div className="text-[11px] text-foreground/50">创意</div>
                </div>
                <div className="rounded-lg bg-surface-active/60 px-2 py-2">
                    <div className="text-lg font-semibold tabular-nums">{counts.approved}</div>
                    <div className="text-[11px] text-foreground/50">已通过</div>
                </div>
                <div className="rounded-lg bg-surface-active/60 px-2 py-2">
                    <div className={cn("text-lg font-semibold tabular-nums", counts.succeeded > 0 && "text-emerald-600 dark:text-emerald-400")}>{counts.succeeded}</div>
                    <div className="text-[11px] text-foreground/50">成功</div>
                </div>
                <div className="rounded-lg bg-surface-active/60 px-2 py-2">
                    <div className={cn("text-lg font-semibold tabular-nums", counts.failed > 0 && "text-rose-600 dark:text-rose-400")}>{counts.failed}</div>
                    <div className="text-[11px] text-foreground/50">失败</div>
                </div>
            </div>
            <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-foreground/50">
                <span>
                    {settings.durationSeconds}s · {settings.aspectRatio}
                </span>
                {finished > 0 ? <span>生成完成 {finished} 条</span> : null}
                {project.massUnlockedAt ? <span className="text-amber-600 dark:text-amber-400">已解锁量产</span> : null}
                {validation.passed && !project.massUnlockedAt ? <span>标准验收通过，待解锁</span> : null}
            </div>
        </button>
    );
}

export default function BatchVideoListPage() {
    const navigate = useNavigate();
    const [items, setItems] = useState<BatchVideoProjectSummary[]>([]);
    const [page, setPage] = useState(1);
    const [pageSize, setPageSize] = useState(20);
    const [total, setTotal] = useState(0);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState("");

    const load = useCallback(
        async (nextPage = page, nextPageSize = pageSize) => {
            setLoading(true);
            setError("");
            try {
                const list = await batchVideoApi.listProjects(nextPage, nextPageSize);
                setItems(list);
                setTotal(list.length < nextPageSize ? (nextPage - 1) * nextPageSize + list.length : nextPage * nextPageSize + 1);
            } catch (err) {
                setError(err instanceof Error ? err.message : "加载失败");
            } finally {
                setLoading(false);
            }
        },
        [page, pageSize],
    );

    useEffect(() => {
        void load(page, pageSize);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [page, pageSize]);

    return (
        <WorkspacePage>
            <PageHeader
                title="批量生成视频"
                description="上传产品素材和卖点，由策划模型产出创意，人工审核后批量提交视频生成。"
                actions={
                    <>
                        <Button icon={<RefreshCw className="size-4" />} onClick={() => load(page, pageSize)} loading={loading}>
                            刷新
                        </Button>
                        <Button type="primary" icon={<Plus className="size-4" />} onClick={() => navigate("/batch-video/new")}>
                            新建项目
                        </Button>
                    </>
                }
            />

            {error ? (
                <div className="mt-4">
                    <WorkspaceErrorState description={error} onRetry={() => load(page, pageSize)} />
                </div>
            ) : loading && items.length === 0 ? (
                <div className="mt-4 grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
                    {Array.from({ length: 3 }).map((_, index) => (
                        <div key={index} className="h-44 animate-pulse rounded-xl bg-surface-active/60" />
                    ))}
                </div>
            ) : items.length === 0 ? (
                <div className="mt-4">
                    <WorkspaceState
                        title="还没有批量视频项目"
                        description="一个项目对应一个产品。先创建项目、上传素材，再进行卖点分析和创意策划。"
                        action={
                            <Button type="primary" icon={<Plus className="size-4" />} onClick={() => navigate("/batch-video/new")}>
                                新建项目
                            </Button>
                        }
                    />
                </div>
            ) : (
                <>
                    <div className="mt-4 grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
                        {items.map((item) => (
                            <SummaryCard key={item.project.id} item={item} onOpen={() => navigate(`/batch-video/${item.project.id}`)} />
                        ))}
                    </div>
                    <PaginationBar current={page} pageSize={pageSize} total={total} onChange={(nextPage, nextPageSize) => { setPage(nextPage); setPageSize(nextPageSize); }} itemLabel="个项目" />
                </>
            )}
        </WorkspacePage>
    );
}
