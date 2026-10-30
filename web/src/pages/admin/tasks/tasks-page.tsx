// 管理端任务看板：跨用户查看排队中/运行中/成功/失败的任务，并按用户汇总。
//
// OSS 成本约束：这个页面每 10 秒轮询一次，对象存储按外出流量计费。
// 因此列表里不渲染任何缩略图/视频元素，只有管理员点击「播放」时才发出一次媒体请求；
// 视频使用 preload="none"，避免打开页面就缓冲整个文件。

import { Alert, Button, Modal } from "antd";
import type { ColumnsType } from "antd/es/table";
import { Download, Pause, Play, RefreshCw } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams } from "react-router";
import { saveAs } from "file-saver";

import { AdminSearchInput } from "../components/admin-search-input";
import { AdminPageFrame } from "../components/admin-shell";
import { AdminDataTable, AdminFilterChip, AdminStatusBadge, AdminTableEmpty, PaginationBar } from "../components/admin-ui";
import { Select } from "@/pages/admin/ui/controls";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
import { apiBaseURL } from "@/services/api/request";
import {
    listAdminAnalyticsUsers,
    listAdminTasks,
    listAdminTaskUserStats,
    type AdminTaskMonitorCounts,
    type AdminTaskMonitorRow,
    type AdminTaskUserStat,
} from "@/services/api/auth";

type RangePreset = "1h" | "24h" | "7d" | "today" | "custom";

const RANGE_OPTIONS: Array<{ label: string; value: RangePreset }> = [
    { label: "近 1 小时", value: "1h" },
    { label: "近 24 小时", value: "24h" },
    { label: "近 7 天", value: "7d" },
    { label: "今天", value: "today" },
    { label: "自定义", value: "custom" },
];

const STATUS_OPTIONS = [
    { label: "全部状态", value: "all" },
    { label: "排队中 + 运行中", value: "queued,running" },
    { label: "排队中", value: "queued" },
    { label: "运行中", value: "running" },
    { label: "成功", value: "succeeded" },
    { label: "失败", value: "failed" },
    { label: "已取消", value: "cancelled" },
];

const CAPABILITY_OPTIONS = [
    { label: "全部类型", value: "all" },
    { label: "图片", value: "image" },
    { label: "视频", value: "video" },
    { label: "文本", value: "text" },
    { label: "音频", value: "audio" },
];

const REFRESH_INTERVAL_MS = 10_000;

export default function TaskMonitorPage() {
    const [searchParams, setSearchParams] = useSearchParams();

    const preset = normalizePreset(searchParams.get("range"));
    const status = searchParams.get("status") || "all";
    const capability = searchParams.get("capability") || "all";
    const userId = searchParams.get("userId") || "";
    const keyword = searchParams.get("filter") || "";
    const page = positiveInt(searchParams.get("page"), 1);
    const pageSize = normalizePageSize(searchParams.get("pageSize"));
    const customFrom = searchParams.get("from") || "";
    const customTo = searchParams.get("to") || "";
    // 按用户汇总有自己的时间窗：默认跟随上方列表，也可以单独选择。
    const userRangeParam = searchParams.get("userRange");
    const userFrom = searchParams.get("userFrom") || "";
    const userTo = searchParams.get("userTo") || "";
    const debouncedKeyword = useDebouncedValue(keyword);

    const [tasks, setTasks] = useState<AdminTaskMonitorRow[]>([]);
    const [total, setTotal] = useState(0);
    const [counts, setCounts] = useState<AdminTaskMonitorCounts | null>(null);
    const [userStats, setUserStats] = useState<AdminTaskUserStat[]>([]);
    const [analyticsUsers, setAnalyticsUsers] = useState<AnalyticsUserRow[]>([]);
    const [loading, setLoading] = useState(true);
    const [loadError, setLoadError] = useState("");
    const [retry, setRetry] = useState(0);
    const [autoRefresh, setAutoRefresh] = useState(true);
    const [lastLoadedAt, setLastLoadedAt] = useState<Date | null>(null);
    const [playing, setPlaying] = useState<{ task: AdminTaskMonitorRow; url: string } | null>(null);
    const requestSequence = useRef(0);

    const range = useMemo(() => resolveRange(preset, customFrom, customTo), [preset, customFrom, customTo]);
    const summaryPreset = userRangeParam ? normalizePreset(userRangeParam) : "page";
    const summaryRange = useMemo(
        () => (summaryPreset === "page" ? range : resolveRange(summaryPreset, userFrom, userTo)),
        [summaryPreset, userFrom, userTo, range],
    );
    const hasFilters = Boolean(keyword || status !== "all" || capability !== "all" || userId);

    const updateUrl = (patch: Record<string, string | number>, replace = false) => {
        const next = new URLSearchParams(searchParams);
        Object.entries(patch).forEach(([key, value]) => {
            const isDefault =
                (key === "filter" && value === "") ||
                (key === "status" && value === "all") ||
                (key === "capability" && value === "all") ||
                (key === "userId" && value === "") ||
                (key === "page" && value === 1) ||
                (key === "pageSize" && value === 20) ||
                (key === "range" && value === "24h");
            if (isDefault) next.delete(key);
            else next.set(key, String(value));
        });
        setSearchParams(next, { replace });
    };

    // 汇总时间窗独立保存；第一次切到自定义时用当前窗口填入起止，避免输入框为空。
    const changeSummaryRange = (value: string) => {
        const next = new URLSearchParams(searchParams);
        if (value === "page") {
            next.delete("userRange");
            next.delete("userFrom");
            next.delete("userTo");
        } else {
            next.set("userRange", value);
            if (value !== "custom") {
                next.delete("userFrom");
                next.delete("userTo");
            } else {
                if (!next.get("userFrom")) next.set("userFrom", summaryRange.from);
                if (!next.get("userTo")) next.set("userTo", summaryRange.to);
            }
        }
        setSearchParams(next, { replace: false });
    };

    const changeSummaryBoundary = (key: "userFrom" | "userTo", value: string) => {
        const next = new URLSearchParams(searchParams);
        next.set("userRange", "custom");
        next.set(key, value);
        setSearchParams(next, { replace: false });
    };

    const listParams = useMemo(
        () => ({
            from: range.from,
            to: range.to,
            userId: userId || undefined,
            status: status === "all" ? undefined : status,
            capability: capability === "all" ? undefined : capability,
            keyword: debouncedKeyword || undefined,
            page,
            pageSize,
        }),
        [range.from, range.to, userId, status, capability, debouncedKeyword, page, pageSize],
    );

    // 列表与状态计数：只取窄投影，不触碰对象存储。
    const loadTasks = useCallback(
        async (background: boolean) => {
            const sequence = ++requestSequence.current;
            if (!background) setLoading(true);
            setLoadError("");
            try {
                const result = await listAdminTasks(listParams);
                if (sequence !== requestSequence.current) return;
                setTasks(result.tasks);
                setTotal(result.total);
                setCounts(result.counts);
                setLastLoadedAt(new Date());
                if (result.total > 0 && result.tasks.length === 0 && page > 1) updateUrl({ page: 1 }, true);
            } catch (error) {
                if (sequence !== requestSequence.current) return;
                setLoadError(error instanceof Error ? error.message : "读取任务列表失败");
            } finally {
                if (sequence === requestSequence.current) setLoading(false);
            }
        },
        // updateUrl 依赖 searchParams，这里用它的稳定行为即可。
        // eslint-disable-next-line react-hooks/exhaustive-deps
        [listParams, page],
    );

    // 按用户汇总：既有维度走 analytics，成功/失败走任务指标，两边按 userId 合并。
    const loadUserStats = useCallback(async () => {
        try {
            const [analytics, metrics] = await Promise.all([
                listAdminAnalyticsUsers({ from: summaryRange.from, to: summaryRange.to }),
                listAdminTaskUserStats({ from: summaryRange.from, to: summaryRange.to }),
            ]);
            setAnalyticsUsers(analytics.users);
            setUserStats(metrics.users);
        } catch {
            // 汇总失败不阻断主列表，页面仍可排查单个任务。
            setAnalyticsUsers([]);
            setUserStats([]);
        }
    }, [summaryRange.from, summaryRange.to]);

    useEffect(() => {
        void loadTasks(false);
    }, [loadTasks, retry]);

    useEffect(() => {
        void loadUserStats();
    }, [loadUserStats]);

    // 只有存在排队中/运行中任务时才轮询；页面在后台时暂停，避免无谓请求。
    const activeCount = (counts?.queued || 0) + (counts?.running || 0);
    const activeCountRef = useRef(activeCount);
    const autoRefreshRef = useRef(autoRefresh);
    useEffect(() => {
        activeCountRef.current = activeCount;
        autoRefreshRef.current = autoRefresh;
    }, [activeCount, autoRefresh]);

    useEffect(() => {
        if (!autoRefresh || activeCount === 0) return;
        let timer = 0;
        let cancelled = false;
        const scheduleNext = () => {
            timer = window.setTimeout(async () => {
                if (!cancelled && document.visibilityState === "visible" && autoRefreshRef.current && activeCountRef.current > 0) {
                    await loadTasks(true);
                }
                if (!cancelled && autoRefreshRef.current && activeCountRef.current > 0) scheduleNext();
            }, REFRESH_INTERVAL_MS);
        };
        scheduleNext();
        return () => {
            cancelled = true;
            window.clearTimeout(timer);
        };
    }, [autoRefresh, activeCount, loadTasks]);

    const mergedUsers = useMemo(() => mergeUserRows(analyticsUsers, userStats), [analyticsUsers, userStats]);
    const statusFiltered = status !== "all";

    const openPreview = (task: AdminTaskMonitorRow) => {
        if (!task.hasMedia) return;
        // 只有在管理员明确点击时才构造媒体地址并发起一次读取。
        setPlaying({ task, url: `${apiBaseURL.replace(/\/$/, "")}/admin/tasks/${encodeURIComponent(task.id)}/media` });
    };

    const columns: ColumnsType<AdminTaskMonitorRow> = [
        {
            title: "时间",
            key: "createdAt",
            width: 132,
            render: (_, task) => <span className="tabular-nums" title={formatTime(task.createdAt)}>{formatCompactTime(task.createdAt)}</span>,
        },
        {
            title: "用户",
            key: "user",
            width: 132,
            ellipsis: true,
            render: (_, task) => (
                <button type="button" className="admin-table-primary-link" title={`只看 ${task.displayName || task.username} 的任务`} onClick={() => updateUrl({ userId: task.userId, page: 1 })}>
                    {task.displayName || task.username || task.userId}
                </button>
            ),
        },
        { title: "类型", key: "type", width: 96, ellipsis: true, render: (_, task) => capabilityText(task.type) },
        { title: "模型", key: "model", width: 150, ellipsis: true, render: (_, task) => <span title={task.model}>{task.model || "--"}</span> },
        {
            title: "状态",
            key: "status",
            width: 108,
            render: (_, task) => (
                <div className="flex flex-col gap-1">
                    <AdminStatusBadge {...statusBadge(task.status)} />
                    {task.status === "running" || task.status === "queued" ? (
                        <span className="text-xs tabular-nums text-foreground/50">{task.stage || "等待中"}{task.progress > 0 ? ` · ${task.progress}%` : ""}</span>
                    ) : null}
                </div>
            ),
        },
        {
            title: "耗时",
            key: "duration",
            width: 88,
            align: "right",
            render: (_, task) => <span className="tabular-nums text-foreground/70">{formatDuration(task)}</span>,
        },
        {
            title: "结果",
            key: "media",
            width: 96,
            align: "center",
            render: (_, task) =>
                task.hasMedia && task.status === "succeeded" ? (
                    <Button size="small" icon={<Play className="size-3.5" />} onClick={() => openPreview(task)} title="点击后才会从对象存储读取该媒体">
                        播放
                    </Button>
                ) : (
                    <span className="text-xs text-foreground/40">--</span>
                ),
        },
        {
            title: "失败原因",
            key: "error",
            ellipsis: true,
            render: (_, task) => (task.error ? <span className="admin-log-error" title={task.error}>{task.error}</span> : <span className="text-foreground/35">--</span>),
        },
    ];

    const userColumns: ColumnsType<MergedUserRow> = [
        {
            title: "用户",
            key: "user",
            width: 150,
            ellipsis: true,
            render: (_, row) => (
                <button type="button" className="admin-table-primary-link" title={`只看 ${row.displayName || row.username} 的任务`} onClick={() => updateUrl({ userId: row.userId, page: 1 })}>
                    {row.displayName || row.username || row.userId}
                </button>
            ),
        },
        { title: "任务数", key: "tasks", width: 88, align: "right", render: (_, row) => <span className="tabular-nums">{row.tasks}</span> },
        { title: "成功", key: "succeeded", width: 80, align: "right", render: (_, row) => <span className="tabular-nums text-foreground/75">{row.succeeded}</span> },
        { title: "失败", key: "failed", width: 80, align: "right", render: (_, row) => <span className={`tabular-nums ${row.failed > 0 ? "admin-log-error" : "text-foreground/75"}`}>{row.failed}</span> },
        { title: "成功率", key: "rate", width: 92, align: "right", render: (_, row) => <span className="tabular-nums text-foreground/75">{row.succeeded + row.failed > 0 ? `${Math.round((row.succeeded / (row.succeeded + row.failed)) * 100)}%` : "--"}</span> },
        { title: "活跃天数", key: "activeDays", width: 96, align: "right", render: (_, row) => <span className="tabular-nums text-foreground/70">{row.activeDays}</span> },
        { title: "Agent 消息", key: "agentMessages", width: 104, align: "right", render: (_, row) => <span className="tabular-nums text-foreground/70">{row.agentMessages}</span> },
        { title: "画布天数", key: "canvasDays", width: 96, align: "right", render: (_, row) => <span className="tabular-nums text-foreground/70">{row.canvasDays}</span> },
        { title: "素材", key: "assets", width: 76, align: "right", render: (_, row) => <span className="tabular-nums text-foreground/70">{row.assets}</span> },
        { title: "资源", key: "resources", width: 76, align: "right", render: (_, row) => <span className="tabular-nums text-foreground/70">{row.resources}</span> },
        { title: "常用模型", key: "commonModel", width: 150, ellipsis: true, render: (_, row) => <span title={row.commonModel}>{row.commonModel || "--"}</span> },
    ];

    return (
        <AdminPageFrame
            title="任务看板"
            description="跨用户任务运行状态与按用户汇总；列表不读取对象存储，点击播放才加载媒体"
            scroll
            actions={
                <div className="flex items-center gap-2">
                    <span className="text-xs text-foreground/45">{lastLoadedAt ? `更新于 ${lastLoadedAt.toLocaleTimeString("zh-CN", { hour12: false })}` : ""}</span>
                    <Button size="small" icon={autoRefresh ? <Pause className="size-4" /> : <Play className="size-4" />} onClick={() => setAutoRefresh((value) => !value)} aria-pressed={autoRefresh}>
                        {autoRefresh ? "暂停刷新" : "自动刷新"}
                    </Button>
                    {!autoRefresh ? <span className="text-xs text-foreground/45">自动刷新已暂停</span> : null}
                    <Button size="small" icon={<RefreshCw className="size-4" />} onClick={() => { void loadTasks(true); void loadUserStats(); }}>
                        刷新
                    </Button>
                </div>
            }
        >
            <div className="mb-3 flex flex-wrap items-center gap-2">
                <Select aria-label="时间范围" className="w-32" value={preset} options={RANGE_OPTIONS.map((item) => ({ label: item.label, value: item.value }))} onChange={(value) => updateUrl({ range: String(value), page: 1 })} />
                {preset === "custom" ? (
                    <>
                        <input type="datetime-local" className="admin-datetime-input" aria-label="开始时间" value={toLocalInput(customFrom)} onChange={(event) => updateUrl({ from: fromLocalInput(event.target.value), page: 1 })} />
                        <span className="text-xs text-foreground/45">至</span>
                        <input type="datetime-local" className="admin-datetime-input" aria-label="结束时间" value={toLocalInput(customTo)} onChange={(event) => updateUrl({ to: fromLocalInput(event.target.value), page: 1 })} />
                    </>
                ) : null}
                <Select aria-label="任务状态" className="w-40" value={status} options={STATUS_OPTIONS} onChange={(value) => updateUrl({ status: String(value), page: 1 })} />
                <Select aria-label="任务类型" className="w-32" value={capability} options={CAPABILITY_OPTIONS} onChange={(value) => updateUrl({ capability: String(value), page: 1 })} />
                <AdminSearchInput className="app-list-search" placeholder="搜索用户、模型、任务 ID 或提示词" value={keyword} onValueChange={(value) => updateUrl({ filter: value, page: 1 }, true)} />
                {userId ? <AdminFilterChip label={`用户：${userLabel(userId, mergedUsers)}`} onRemove={() => updateUrl({ userId: "", page: 1 })} /> : null}
                {statusFiltered ? <AdminFilterChip label={`状态：${STATUS_OPTIONS.find((item) => item.value === status)?.label || status}`} onRemove={() => updateUrl({ status: "all", page: 1 })} /> : null}
                {capability !== "all" ? <AdminFilterChip label={`类型：${CAPABILITY_OPTIONS.find((item) => item.value === capability)?.label || capability}`} onRemove={() => updateUrl({ capability: "all", page: 1 })} /> : null}
                {keyword ? <AdminFilterChip label={`搜索：${keyword}`} onRemove={() => updateUrl({ filter: "", page: 1 })} /> : null}
            </div>

            <StatusCards counts={counts} active={status} onSelect={(value) => updateUrl({ status: value, page: 1 })} />

            <div className="mt-4">
                <AdminDataTable
                    skeletonColumns={8}
                    empty={
                        loadError ? (
                            <Alert type="error" showIcon title="任务列表读取失败" description={loadError} action={<Button size="small" onClick={() => setRetry((value) => value + 1)}>重试</Button>} />
                        ) : (
                            <AdminTableEmpty filtered={hasFilters} />
                        )
                    }
                    footer={<PaginationBar alwaysShow current={page} pageSize={pageSize} total={total} onChange={(nextPage, nextSize) => updateUrl({ page: nextSize !== pageSize ? 1 : nextPage, pageSize: nextSize })} />}
                    table={{
                        size: "small",
                        rowKey: "id",
                        loading,
                        columns,
                        dataSource: tasks,
                        pagination: false,
                        tableLayout: "fixed",
                        scroll: { x: 1000 },
                    }}
                />
            </div>

            <div className="mt-6">
                <div className="mb-2 flex flex-wrap items-center gap-2">
                    <h2 className="text-sm font-medium">按用户汇总</h2>
                    <Select aria-label="按用户汇总的时间范围" className="w-32" value={summaryPreset} options={[{ label: "跟随列表", value: "page" }, ...RANGE_OPTIONS.map((item) => ({ label: item.label, value: item.value }))]} onChange={(value) => changeSummaryRange(String(value))} />
                    {summaryPreset === "custom" ? (
                        <>
                            <input type="datetime-local" className="admin-datetime-input" aria-label="汇总开始时间" value={toLocalInput(userFrom)} onChange={(event) => changeSummaryBoundary("userFrom", fromLocalInput(event.target.value))} />
                            <span className="text-xs text-foreground/45">至</span>
                            <input type="datetime-local" className="admin-datetime-input" aria-label="汇总结束时间" value={toLocalInput(userTo)} onChange={(event) => changeSummaryBoundary("userTo", fromLocalInput(event.target.value))} />
                        </>
                    ) : null}
                    <span className="ml-auto text-xs text-foreground/45">任务数与成功/失败按此时间窗统计；其余维度沿用既有用户统计口径</span>
                </div>
                <AdminDataTable
                    skeletonColumns={6}
                    empty={<AdminTableEmpty title="该时间范围内没有生成任务" />}
                    table={{
                        size: "small",
                        rowKey: "userId",
                        columns: userColumns,
                        dataSource: mergedUsers,
                        pagination: false,
                        tableLayout: "fixed",
                        scroll: { x: 1180 },
                    }}
                />
            </div>

            <Modal
                open={Boolean(playing)}
                title={playing ? `任务 ${playing.task.id.slice(0, 8)} · ${capabilityText(playing.task.type)}` : "媒体预览"}
                width={880}
                footer={
                    playing ? (
                        <Button icon={<Download className="size-4" />} onClick={() => saveAs(playing.url, `${playing.task.id}.${playing.task.mediaKind === "video" ? "mp4" : "png"}`)}>
                            下载原文件
                        </Button>
                    ) : null
                }
                onCancel={() => setPlaying(null)}
                destroyOnHidden
            >
                {playing ? <TaskMedia src={playing.url} kind={playing.task.mediaKind === "video" ? "video" : "image"} /> : null}
            </Modal>
        </AdminPageFrame>
    );
}

// TaskMedia 与通用 MediaPreview 的差别只在视频 preload：这里必须是 none，
// 否则浏览器在打开弹窗时就会向对象存储预取数据。
function TaskMedia({ src, kind }: { src: string; kind: "image" | "video" }) {
    const [failed, setFailed] = useState(false);
    useEffect(() => setFailed(false), [src]);
    if (failed) return <div className="py-10 text-center text-sm text-foreground/50">媒体不可用，文件可能已删除</div>;
    if (kind === "video") {
        return <video key={src} src={src} controls playsInline preload="none" className="max-h-[72vh] w-full bg-black object-contain" onError={() => setFailed(true)} />;
    }
    return <img key={src} src={src} alt="任务结果" className="max-h-[72vh] w-full object-contain" onError={() => setFailed(true)} />;
}

function StatusCards({ counts, active, onSelect }: { counts: AdminTaskMonitorCounts | null; active: string; onSelect: (value: string) => void }) {
    const items = [
        { key: "queued", label: "排队中", value: counts?.queued ?? 0, tone: "warning" as const },
        { key: "running", label: "运行中", value: counts?.running ?? 0, tone: "info" as const },
        { key: "succeeded", label: "成功", value: counts?.succeeded ?? 0, tone: "success" as const },
        { key: "failed", label: "失败", value: counts?.failed ?? 0, tone: "error" as const },
        { key: "cancelled", label: "已取消", value: counts?.cancelled ?? 0, tone: "neutral" as const },
    ];
    return (
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
            {items.map((item) => (
                <button
                    key={item.key}
                    type="button"
                    className="rounded-md border bg-background p-3 text-left transition-colors"
                    style={{ borderColor: active === item.key ? "var(--admin-accent, #2563eb)" : "var(--admin-border)" }}
                    aria-pressed={active === item.key}
                    onClick={() => onSelect(active === item.key ? "all" : item.key)}
                >
                    <div className="text-xs text-foreground/50">{item.label}</div>
                    <div className="mt-1 text-xl font-semibold tabular-nums">{item.value}</div>
                </button>
            ))}
        </div>
    );
}

type AnalyticsUserRow = {
    userId: string;
    name: string;
    activeDays: number;
    tasks: number;
    agentMessages: number;
    canvasDays: number;
    assets: number;
    resources: number;
    commonModel?: string;
};

type MergedUserRow = {
    userId: string;
    username: string;
    displayName: string;
    tasks: number;
    succeeded: number;
    failed: number;
    activeDays: number;
    agentMessages: number;
    canvasDays: number;
    assets: number;
    resources: number;
    commonModel: string;
};

function mergeUserRows(analytics: AnalyticsUserRow[], metrics: AdminTaskUserStat[]): MergedUserRow[] {
    const rows = new Map<string, MergedUserRow>();
    for (const item of analytics) {
        rows.set(item.userId, {
            userId: item.userId,
            username: item.name,
            displayName: item.name,
            tasks: item.tasks,
            succeeded: 0,
            failed: 0,
            activeDays: item.activeDays,
            agentMessages: item.agentMessages,
            canvasDays: item.canvasDays,
            assets: item.assets,
            resources: item.resources,
            commonModel: item.commonModel || "",
        });
    }
    for (const item of metrics) {
        const existing = rows.get(item.userId);
        if (existing) {
            existing.succeeded = item.succeeded;
            existing.failed = item.failed;
            existing.username = item.username || existing.username;
            existing.displayName = item.displayName || existing.displayName;
            continue;
        }
        rows.set(item.userId, {
            userId: item.userId,
            username: item.username,
            displayName: item.displayName,
            tasks: item.total,
            succeeded: item.succeeded,
            failed: item.failed,
            activeDays: 0,
            agentMessages: 0,
            canvasDays: 0,
            assets: 0,
            resources: 0,
            commonModel: "",
        });
    }
    return Array.from(rows.values()).sort((a, b) => b.tasks - a.tasks);
}

function userLabel(userId: string, rows: MergedUserRow[]) {
    const row = rows.find((item) => item.userId === userId);
    return row?.displayName || row?.username || userId.slice(0, 8);
}

function resolveRange(preset: RangePreset, customFrom: string, customTo: string) {
    const now = new Date();
    if (preset === "custom") {
        const from = customFrom ? new Date(customFrom) : new Date(now.getTime() - 24 * 3600 * 1000);
        const to = customTo ? new Date(customTo) : now;
        return { from: validDate(from, new Date(now.getTime() - 24 * 3600 * 1000)).toISOString(), to: validDate(to, now).toISOString() };
    }
    if (preset === "today") {
        const start = new Date(now);
        start.setHours(0, 0, 0, 0);
        return { from: start.toISOString(), to: now.toISOString() };
    }
    const hours = preset === "1h" ? 1 : preset === "7d" ? 24 * 7 : 24;
    return { from: new Date(now.getTime() - hours * 3600 * 1000).toISOString(), to: now.toISOString() };
}

function validDate(value: Date, fallback: Date) {
    return Number.isNaN(value.getTime()) ? fallback : value;
}

function normalizePreset(value: string | null): RangePreset {
    return value === "1h" || value === "7d" || value === "today" || value === "custom" ? value : "24h";
}

function positiveInt(value: string | null, fallback: number) {
    const parsed = Number.parseInt(value || "", 10);
    return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback;
}

function normalizePageSize(value: string | null) {
    const parsed = positiveInt(value, 20);
    return [20, 50, 100].includes(parsed) ? parsed : 20;
}

function statusBadge(status: AdminTaskMonitorRow["status"]) {
    const map: Record<AdminTaskMonitorRow["status"], { label: string; tone: "success" | "error" | "warning" | "info" | "neutral" }> = {
        queued: { label: "排队中", tone: "neutral" },
        running: { label: "运行中", tone: "info" },
        succeeded: { label: "成功", tone: "success" },
        failed: { label: "失败", tone: "error" },
        cancelled: { label: "已取消", tone: "warning" },
    };
    return map[status] || { label: status, tone: "neutral" as const };
}

function capabilityText(type: string) {
    const lower = (type || "").toLowerCase();
    if (lower.includes("video")) return "视频";
    if (lower.includes("image")) return "图片";
    if (lower.includes("audio") || lower.includes("voice") || lower.includes("speech")) return "音频";
    return "文本";
}

function formatDuration(task: AdminTaskMonitorRow) {
    if (!task.startedAt) return "--";
    const end = task.completedAt ? new Date(task.completedAt) : new Date(task.updatedAt);
    const start = new Date(task.startedAt);
    const diff = end.getTime() - start.getTime();
    if (!Number.isFinite(diff) || diff < 0) return "--";
    if (diff < 1000) return "<1s";
    const seconds = Math.round(diff / 1000);
    if (seconds < 60) return `${seconds}s`;
    const minutes = Math.floor(seconds / 60);
    return `${minutes}m${String(seconds % 60).padStart(2, "0")}s`;
}

function formatTime(value?: string) {
    return value ? new Date(value).toLocaleString("zh-CN", { hour12: false }) : "--";
}

function formatCompactTime(value?: string) {
    if (!value) return "--";
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return "--";
    return date.toLocaleString("zh-CN", { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hour12: false });
}

function toLocalInput(iso: string) {
    if (!iso) return "";
    const date = new Date(iso);
    if (Number.isNaN(date.getTime())) return "";
    const pad = (value: number) => String(value).padStart(2, "0");
    return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

function fromLocalInput(value: string) {
    if (!value) return "";
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? "" : date.toISOString();
}
