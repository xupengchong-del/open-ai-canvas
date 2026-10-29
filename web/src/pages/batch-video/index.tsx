import { App, Button, Tag } from "antd";
import { ChevronRight, Plus, RefreshCw, Trash2 } from "lucide-react";
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

function formatUpdatedAt(value: string): string {
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return "—";
    return date.toLocaleString("zh-CN", { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit" });
}

export default function BatchVideoListPage() {
    const navigate = useNavigate();
    const { modal, message } = App.useApp();
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

    const removeProject = (item: BatchVideoProjectSummary) => {
        modal.confirm({
            title: `删除项目「${item.project.name}」？`,
            content: "会同时删除该项目的素材批次、卖点、参考图、创意、生成任务与验收记录，并清理仅被该项目使用的素材文件，且不可恢复。仍被其他项目引用的素材会保留。项目内有进行中的任务时需先结束。",
            okText: "删除",
            okButtonProps: { danger: true },
            onOk: async () => {
                try {
                    await batchVideoApi.deleteProject(item.project.id);
                    message.success("项目已删除");
                    const nextPage = items.length === 1 && page > 1 ? page - 1 : page;
                    if (nextPage !== page) setPage(nextPage);
                    else await load(page, pageSize);
                } catch (err) {
                    message.error(err instanceof Error ? err.message : "删除项目失败");
                    throw err;
                }
            },
        });
    };

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
                <div className="mt-4 space-y-2">
                    {Array.from({ length: 4 }).map((_, index) => (
                        <div key={index} className="h-14 animate-pulse rounded-xl bg-surface-active/60" />
                    ))}
                </div>
            ) : items.length === 0 ? (
                <div className="mt-4">
                    <WorkspaceState
                        title="还没有批量视频项目"
                        description="一个项目对应一个产品。先创建项目，再在项目内追加素材、分析和策划。"
                        action={
                            <Button type="primary" icon={<Plus className="size-4" />} onClick={() => navigate("/batch-video/new")}>
                                新建项目
                            </Button>
                        }
                    />
                </div>
            ) : (
                <>
                    <div className="mt-4 overflow-hidden rounded-xl border border-border/70 bg-surface">
                        <div className="overflow-x-auto">
                            <table className="w-full min-w-[900px] text-left text-xs">
                                <thead className="bg-surface-active/40 text-foreground/50">
                                    <tr>
                                        <th className="px-4 py-2.5 font-medium">项目 / 产品</th>
                                        <th className="px-3 py-2.5 font-medium">阶段</th>
                                        <th className="px-3 py-2.5 text-right font-medium">创意</th>
                                        <th className="px-3 py-2.5 text-right font-medium">已通过</th>
                                        <th className="px-3 py-2.5 text-right font-medium">成功</th>
                                        <th className="px-3 py-2.5 text-right font-medium">失败</th>
                                        <th className="px-3 py-2.5 font-medium">参数</th>
                                        <th className="px-3 py-2.5 font-medium">更新时间</th>
                                        <th className="px-4 py-2.5 text-right font-medium">操作</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {items.map((item) => {
                                        const { project, counts, settings, validation } = item;
                                        return (
                                            <tr
                                                key={project.id}
                                                className="cursor-pointer border-t border-border/60 transition-colors hover:bg-surface-active/40"
                                                onClick={() => navigate(`/batch-video/${project.id}`)}
                                            >
                                                <td className="max-w-[260px] px-4 py-3">
                                                    <div className="truncate text-sm font-medium text-foreground">{project.name}</div>
                                                    <div className="mt-0.5 truncate text-[11px] text-foreground/50">{project.productName || "未填写产品名称"}</div>
                                                </td>
                                                <td className="px-3 py-3">
                                                    <Tag color={stageTone(project.stage)} bordered={false}>{batchVideoStageLabel(project.stage)}</Tag>
                                                </td>
                                                <td className="px-3 py-3 text-right tabular-nums">{counts.creativeTotal}</td>
                                                <td className="px-3 py-3 text-right tabular-nums">{counts.approved}</td>
                                                <td className={cn("px-3 py-3 text-right tabular-nums", counts.succeeded > 0 && "font-medium text-emerald-600 dark:text-emerald-400")}>{counts.succeeded}</td>
                                                <td className={cn("px-3 py-3 text-right tabular-nums", counts.failed > 0 && "font-medium text-rose-600 dark:text-rose-400")}>{counts.failed}</td>
                                                <td className="px-3 py-3 text-foreground/60">
                                                    <div>{settings.durationSeconds}s · {settings.aspectRatio}</div>
                                                    {project.massUnlockedAt ? (
                                                        <div className="mt-0.5 text-[11px] text-amber-600 dark:text-amber-400">量产已解锁</div>
                                                    ) : validation.passed ? (
                                                        <div className="mt-0.5 text-[11px] text-emerald-600 dark:text-emerald-400">验收通过，待解锁</div>
                                                    ) : null}
                                                </td>
                                                <td className="whitespace-nowrap px-3 py-3 text-foreground/55">{formatUpdatedAt(project.updatedAt)}</td>
                                                <td className="px-4 py-3">
                                                    <div className="flex items-center justify-end gap-1">
                                                        <Button
                                                            size="small"
                                                            type="text"
                                                            icon={<Trash2 className="size-3.5" />}
                                                            className="!text-rose-600 hover:!bg-rose-500/10"
                                                            onClick={(event) => {
                                                                event.stopPropagation();
                                                                removeProject(item);
                                                            }}
                                                        >
                                                            删除
                                                        </Button>
                                                        <ChevronRight className="size-4 text-foreground/30" aria-hidden />
                                                    </div>
                                                </td>
                                            </tr>
                                        );
                                    })}
                                </tbody>
                            </table>
                        </div>
                    </div>
                    <PaginationBar current={page} pageSize={pageSize} total={total} onChange={(nextPage, nextPageSize) => { setPage(nextPage); setPageSize(nextPageSize); }} itemLabel="个项目" />
                </>
            )}
        </WorkspacePage>
    );
}
