import { App, Button, Input, Modal, Tabs, Tag } from "antd";
import { ArrowLeft, Clapperboard, RefreshCw, Settings2, Sparkles, Upload, Zap } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router";

import { PageHeader, WorkspacePage } from "@/components/layout/workspace-page";
import { WorkspaceErrorState, WorkspaceLoadingState } from "@/components/layout/workspace-state";
import {
    batchVideoApi,
    batchVideoStageLabel,
    type BatchVideoAdminSettings,
    type BatchVideoCreative,
    type BatchVideoDocumentInput,
    type BatchVideoGenerationJob,
    type BatchVideoMaterialInput,
    type BatchVideoModelOption,
    type BatchVideoPlanningModelOption,
    type BatchVideoProjectDetail,
    type BatchVideoProjectSettings,
    type BatchVideoReferenceModelOption,
} from "@/services/api/batch-video";
import { CreativePanel } from "./components/creative-panel";
import { CreativeDetailDrawer } from "./components/creative-detail-drawer";
import { AddBatchModal, PlanDialog, ProjectSettingsDrawer, SubmitDialog } from "./components/dialogs";
import { JobsPanel } from "./components/jobs-panel";
import { MaterialsPanel } from "./components/materials-panel";
import { OverviewPanel } from "./components/overview-panel";

function hasActiveWork(detail: BatchVideoProjectDetail): boolean {
    if (detail.project.analysisStatus === "queued" || detail.project.analysisStatus === "running") return true;
    if (detail.rounds.some((round) => round.status === "queued" || round.status === "generating" || round.status === "running")) return true;
    return detail.jobs.some((job) => job.status === "waiting" || job.status === "queued" || job.status === "running");
}

export default function BatchVideoDetailPage() {
    const { id = "" } = useParams();
    const navigate = useNavigate();
    const { message, modal } = App.useApp();
    const [detail, setDetail] = useState<BatchVideoProjectDetail | null>(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState("");
    const [tab, setTab] = useState("overview");
    const [creatives, setCreatives] = useState<BatchVideoCreative[]>([]);
    const [creativeTotal, setCreativeTotal] = useState(0);
    const [creativePage, setCreativePage] = useState(1);
    const [creativePageSize, setCreativePageSize] = useState(20);
    const [reviewFilter, setReviewFilter] = useState("");
    const [creativeLoading, setCreativeLoading] = useState(false);
    const [reviewSelection, setReviewSelection] = useState<Set<string>>(new Set());
    const [submitSelection, setSubmitSelection] = useState<Set<string>>(new Set());
    const [rejectIds, setRejectIds] = useState<string[]>([]);
    const [rejectNote, setRejectNote] = useState("");
    const [jobs, setJobs] = useState<BatchVideoGenerationJob[]>([]);
    const [jobPage, setJobPage] = useState(1);
    const [jobPageSize, setJobPageSize] = useState(20);
    const [jobLoading, setJobLoading] = useState(false);
    const [actionLoading, setActionLoading] = useState("");
    const [planningModels, setPlanningModels] = useState<BatchVideoPlanningModelOption[]>([]);
    const [referenceModels, setReferenceModels] = useState<BatchVideoReferenceModelOption[]>([]);
    const [models, setModels] = useState<BatchVideoModelOption[]>([]);
    const [settings, setSettings] = useState<BatchVideoAdminSettings | null>(null);
    const [settingsOpen, setSettingsOpen] = useState(false);
    const [planOpen, setPlanOpen] = useState(false);
    const [submitOpen, setSubmitOpen] = useState(false);
    const [addBatchOpen, setAddBatchOpen] = useState(false);
    const [activeCreativeId, setActiveCreativeId] = useState("");
    const [savingSettings, setSavingSettings] = useState(false);
    const [planning, setPlanning] = useState(false);
    const [submitting, setSubmitting] = useState(false);
    const [addingBatch, setAddingBatch] = useState(false);
    const activeRef = useRef(false);

    const loadCreatives = useCallback(
        async (page = creativePage, pageSize = creativePageSize, reviewStatus = reviewFilter) => {
            if (!id) return;
            setCreativeLoading(true);
            try {
                const result = await batchVideoApi.listCreatives(id, { page, pageSize, reviewStatus: reviewStatus || undefined });
                setCreatives(result.creatives);
                setCreativeTotal(result.total);
            } catch (err) {
                message.error(err instanceof Error ? err.message : "加载创意失败");
            } finally {
                setCreativeLoading(false);
            }
        },
        [creativePage, creativePageSize, id, message, reviewFilter],
    );

    const loadJobs = useCallback(
        async (page = jobPage, pageSize = jobPageSize) => {
            if (!id) return;
            setJobLoading(true);
            try {
                setJobs(await batchVideoApi.listJobs(id, { page, pageSize }));
            } catch (err) {
                message.error(err instanceof Error ? err.message : "加载生成任务失败");
            } finally {
                setJobLoading(false);
            }
        },
        [id, jobPage, jobPageSize, message],
    );

    const loadDetail = useCallback(
        async (silent = false) => {
            if (!id) return;
            if (!silent) setLoading(true);
            setError("");
            try {
                const next = await batchVideoApi.getProject(id);
                setDetail(next);
                activeRef.current = hasActiveWork(next);
            } catch (err) {
                if (!silent) setError(err instanceof Error ? err.message : "加载项目失败");
            } finally {
                if (!silent) setLoading(false);
            }
        },
        [id],
    );

    const refresh = useCallback(
        async (silent = false) => {
            await Promise.all([loadDetail(silent), loadCreatives(), loadJobs()]);
        },
        [loadCreatives, loadDetail, loadJobs],
    );

    useEffect(() => {
        void loadDetail(false);
        void loadCreatives(1, creativePageSize, "");
        void loadJobs(1, jobPageSize);
        void Promise.all([batchVideoApi.listPlanningModels(), batchVideoApi.listModels(), batchVideoApi.listReferenceModels(), batchVideoApi.settings().catch(() => null)])
            .then(([planning, videoModels, reference, adminSettings]) => {
                setPlanningModels(planning);
                setModels(videoModels);
                setReferenceModels(reference);
                setSettings(adminSettings);
            })
            .catch((err) => message.error(err instanceof Error ? err.message : "加载模型列表失败"));
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [id]);

    useEffect(() => {
        const timer = window.setInterval(() => {
            if (!activeRef.current || document.hidden) return;
            void loadDetail(true);
            void loadCreatives();
            void loadJobs();
        }, 3000);
        return () => window.clearInterval(timer);
    }, [loadCreatives, loadDetail, loadJobs]);

    const runAction = async (key: string, action: () => Promise<void>) => {
        setActionLoading(key);
        try {
            await action();
        } catch (err) {
            message.error(err instanceof Error ? err.message : "操作失败");
        } finally {
            setActionLoading("");
        }
    };

    const analyze = () => {
        if (!detail) return;
        const rerun = detail.project.analysisStatus === "succeeded";
        const perform = () =>
            void runAction("analyze", async () => {
                const next = await batchVideoApi.analyze(detail.project.id);
                setDetail(next);
                activeRef.current = true;
                message.success("分析任务已提交，完成后自动更新卖点与产品档案");
            });
        if (rerun) {
            modal.confirm({ title: "重新分析素材？", content: "会重新调用策划模型读取素材和文档，更新卖点与参考图说明。已有创意不会被删除。", okText: "重新分析", onOk: perform });
            return;
        }
        perform();
    };

    const generate = () => {
        if (!detail) return;
        if (detail.sellingPoints.length === 0) {
            message.warning("请先分析素材或确认已有卖点");
            return;
        }
        setPlanOpen(true);
    };

    const confirmPlan = (payload: { count: number; mode: string; templates: string[] }) => {
        if (!detail) return;
        setPlanning(true);
        void batchVideoApi.generateCreatives(detail.project.id, payload)
            .then((result) => {
                setPlanOpen(false);
                setTab("creatives");
                message.success(`已提交 ${result.rounds || 1} 个策划轮次，完成后创意会出现在列表`);
                return refresh(true);
            })
            .catch((err) => message.error(err instanceof Error ? err.message : "提交策划失败"))
            .finally(() => setPlanning(false));
    };

    const generateReference = (creative: BatchVideoCreative) => {
        if (!detail) return;
        void runAction(`generate-ref-${creative.id}`, async () => {
            await batchVideoApi.generateReference(detail.project.id, creative.id);
            activeRef.current = true;
            await refresh(true);
            message.success("参考图生成任务已提交");
        });
    };

    const saveCreative = (creative: BatchVideoCreative, draft: Pick<BatchVideoCreative, "title" | "hook" | "copywriting" | "videoPrompt" | "imagePrompt">) => {
        if (!detail) return;
        void runAction(`save-${creative.id}`, async () => {
            await batchVideoApi.updateCreative(detail.project.id, creative.id, draft);
            setSubmitSelection((current) => {
                const next = new Set(current);
                next.delete(creative.id);
                return next;
            });
            await refresh(true);
            message.success("创意已保存并转为待审核");
        });
    };

    const rematchCreative = (creative: BatchVideoCreative) => {
        if (!detail) return;
        void runAction(`rematch-${creative.id}`, async () => {
            const updated = await batchVideoApi.rematchCreative(detail.project.id, creative.id);
            await refresh(true);
            message.success(updated.matchStatus === "matched" ? "已重新匹配参考图，请重新审核" : "没有符合要求的参考图，可单独生成");
        });
    };

    const reviewCreative = (creative: BatchVideoCreative, status: "approved" | "rejected", note = "") => {
        if (!detail) return;
        void runAction(`${status}-${creative.id}`, async () => {
            await batchVideoApi.reviewCreative(detail.project.id, creative.id, status, note);
            setReviewSelection((current) => {
                const next = new Set(current);
                next.delete(creative.id);
                return next;
            });
            if (status === "rejected") {
                setSubmitSelection((current) => {
                    const next = new Set(current);
                    next.delete(creative.id);
                    return next;
                });
            }
            await refresh(true);
            message.success(status === "approved" ? "已通过" : "已驳回");
        });
    };

    const requestReview = (creative: BatchVideoCreative, status: "approved" | "rejected") => {
        if (status === "rejected") {
            setRejectIds([creative.id]);
            setRejectNote("");
            return;
        }
        reviewCreative(creative, status);
    };

    const batchReview = (status: "approved" | "rejected") => {
        if (!detail) return;
        const ids = Array.from(reviewSelection);
        if (status === "rejected") {
            setRejectIds(ids);
            setRejectNote("");
            return;
        }
        void runAction("batch-approved", async () => {
            const result = await batchVideoApi.reviewCreatives(detail.project.id, ids, status);
            setReviewSelection(new Set());
            await refresh(true);
            message.success(`已处理 ${result.updated} 条创意`);
        });
    };

    const confirmReject = () => {
        if (!detail || !rejectNote.trim()) {
            message.warning("请填写驳回原因");
            return;
        }
        const ids = rejectIds;
        void runAction("reject-confirm", async () => {
            if (ids.length === 1) await batchVideoApi.reviewCreative(detail.project.id, ids[0], "rejected", rejectNote.trim());
            else await batchVideoApi.reviewCreatives(detail.project.id, ids, "rejected", rejectNote.trim());
            setRejectIds([]);
            setRejectNote("");
            setReviewSelection(new Set());
            setSubmitSelection((current) => {
                const next = new Set(current);
                ids.forEach((id) => next.delete(id));
                return next;
            });
            await refresh(true);
            message.success(`已驳回 ${ids.length} 条创意`);
        });
    };

    const confirmSubmit = (payload: { allApproved: boolean; generationMode: "standard" | "mass"; mode: "single" | "multi"; modelIds: string[] }) => {
        if (!detail) return;
        const selected = payload.allApproved ? [] : Array.from(submitSelection);
        setSubmitting(true);
        void batchVideoApi.submit(detail.project.id, {
            creativeIds: selected,
            allApproved: payload.allApproved,
            generationMode: payload.generationMode,
            mode: payload.mode,
            models: payload.modelIds.map((channelModelId) => {
                const model = models.find((item) => item.channelModelId === channelModelId);
                return { channelModelId, modelId: channelModelId, model: model?.model || "" };
            }),
        })
            .then((result) => {
                setSubmitOpen(false);
                setSubmitSelection(new Set());
                setTab("jobs");
                message.success(`已提交 ${result.jobs} 个生成任务，服务端队列开始运行`);
                return refresh(true);
            })
            .catch((err) => message.error(err instanceof Error ? err.message : "提交生成失败"))
            .finally(() => setSubmitting(false));
    };

    const retryJob = (job: BatchVideoGenerationJob) => {
        if (!detail) return;
        void runAction(`retry-${job.id}`, async () => {
            await batchVideoApi.retryJob(detail.project.id, job.id);
            await refresh(true);
            message.success("已重新排队");
        });
    };

    const retryFailed = () => {
        if (!detail) return;
        void runAction("retry-failed", async () => {
            const result = await batchVideoApi.retryFailed(detail.project.id);
            await refresh(true);
            message.success(`已重新提交 ${result.retried} 条失败任务`);
        });
    };

    const cancelPending = () => {
        if (!detail) return;
        modal.confirm({
            title: "取消全部未开始任务？",
            content: "只取消仍在等待的任务；已提交给模型的任务和已完成产物不受影响。",
            okText: "取消未开始项",
            okButtonProps: { danger: true },
            onOk: () => runAction("cancel-pending", async () => {
                const result = await batchVideoApi.cancelPending(detail.project.id);
                await refresh(true);
                message.success(`已取消 ${result.cancelled} 个未开始任务`);
            }),
        });
    };

    const togglePause = () => {
        if (!detail) return;
        const paused = detail.project.status === "paused";
        void runAction(paused ? "resume" : "pause", async () => {
            if (paused) await batchVideoApi.resume(detail.project.id);
            else await batchVideoApi.pause(detail.project.id);
            await refresh(true);
            message.success(paused ? "调度已继续" : "调度已暂停，运行中的任务不受影响");
        });
    };

    const unlockMass = () => {
        if (!detail) return;
        modal.confirm({
            title: "解锁量产？",
            content: "标准验收通过后解锁量产。解锁后单轮可生成最多 1000 条创意，仍受模型和预算约束。",
            okText: "解锁量产",
            onOk: () =>
                runAction("unlock", async () => {
                    await batchVideoApi.unlockMass(detail.project.id);
                    await refresh(true);
                    message.success("已解锁量产");
                }),
        });
    };

    const saveSettings = (next: BatchVideoProjectSettings) => {
        if (!detail) return;
        setSavingSettings(true);
        void batchVideoApi.updateSettings(detail.project.id, next)
            .then((updated) => {
                setDetail(updated);
                setSettingsOpen(false);
                message.success("项目设置已保存");
            })
            .catch((err) => message.error(err instanceof Error ? err.message : "保存失败"))
            .finally(() => setSavingSettings(false));
    };

    const confirmAddBatch = (payload: { label: string; materials: BatchVideoMaterialInput[]; documents: BatchVideoDocumentInput[] }) => {
        if (!detail) return;
        setAddingBatch(true);
        void batchVideoApi.addMaterialBatch(detail.project.id, payload)
            .then(async () => {
                setAddBatchOpen(false);
                await refresh(true);
                modal.confirm({
                    title: "素材批次已追加",
                    content: "是否立即重新分析，把新素材的卖点和参考图说明合并进产品档案？",
                    okText: "重新分析",
                    cancelText: "稍后",
                    onOk: analyze,
                });
            })
            .catch((err) => message.error(err instanceof Error ? err.message : "追加素材失败"))
            .finally(() => setAddingBatch(false));
    };

    if (loading && !detail) {
        return (
            <WorkspacePage>
                <WorkspaceLoadingState label="正在加载批量视频项目" rows={3} />
            </WorkspacePage>
        );
    }
    if (error || !detail) {
        return (
            <WorkspacePage>
                <WorkspaceErrorState description={error || "项目不存在"} onRetry={() => void loadDetail(false)} />
            </WorkspacePage>
        );
    }

    const generatingRound = detail.rounds.some((round) => round.status === "queued" || round.status === "generating" || round.status === "running");
    const analysisRunning = detail.project.analysisStatus === "queued" || detail.project.analysisStatus === "running";
    const activeCreative = creatives.find((creative) => creative.id === activeCreativeId) || detail.creatives.find((creative) => creative.id === activeCreativeId);

    return (
        <WorkspacePage>
            <PageHeader
                title={detail.project.name}
                description={detail.project.productName || "批量视频项目"}
                meta={
                    <>
                        <Tag color={detail.project.massUnlockedAt ? "success" : "processing"} bordered={false}>
                            {batchVideoStageLabel(detail.project.stage)}
                        </Tag>
                        {detail.project.massUnlockedAt ? <Tag color="warning" bordered={false}>量产已解锁</Tag> : null}
                        {detail.project.status === "paused" ? <Tag color="warning" bordered={false}>已暂停</Tag> : null}
                    </>
                }
                actions={
                    <>
                        <Button icon={<ArrowLeft className="size-4" />} onClick={() => navigate("/batch-video")}>返回</Button>
                        <Button icon={<RefreshCw className="size-4" />} onClick={() => void refresh(false)} loading={loading}>刷新</Button>
                        <Button icon={<Settings2 className="size-4" />} onClick={() => setSettingsOpen(true)}>项目设置</Button>
                        <Button icon={<Upload className="size-4" />} onClick={() => setAddBatchOpen(true)}>追加素材</Button>
                        <Button icon={<Sparkles className="size-4" />} loading={actionLoading === "analyze"} disabled={analysisRunning} onClick={analyze}>
                            {detail.project.analysisStatus === "succeeded" ? "重新分析" : "分析素材"}
                        </Button>
                        <Button icon={<Clapperboard className="size-4" />} onClick={generate}>生成创意</Button>
                        <Button type="primary" icon={<Zap className="size-4" />} disabled={detail.counts.approved === 0} onClick={() => setSubmitOpen(true)}>
                            提交生成
                        </Button>
                    </>
                }
            />

            {detail.validation.passed && !detail.project.massUnlockedAt ? (
                <div className="mt-3 flex flex-wrap items-center justify-between gap-3 rounded-xl border border-emerald-500/30 bg-emerald-500/10 px-4 py-3">
                    <div>
                        <div className="text-sm font-semibold text-emerald-700 dark:text-emerald-300">标准验收已通过</div>
                        <p className="text-xs text-emerald-700/70 dark:text-emerald-300/70">确认验收结果后可解锁量产，单轮最多 1000 条创意。</p>
                    </div>
                    <Button type="primary" loading={actionLoading === "unlock"} onClick={unlockMass}>解锁量产</Button>
                </div>
            ) : null}

            <Tabs
                className="mt-3"
                activeKey={tab}
                onChange={setTab}
                items={[
                    { key: "overview", label: "概览", children: <OverviewPanel detail={detail} onAnalyze={analyze} /> },
                    {
                        key: "creatives",
                        label: `创意与审核${detail.counts.pendingReview > 0 ? `（${detail.counts.pendingReview}）` : ""}`,
                        children: (
                            <CreativePanel
                                detail={detail}
                                creatives={creatives}
                                total={creativeTotal}
                                page={creativePage}
                                pageSize={creativePageSize}
                                reviewFilter={reviewFilter}
                                loading={creativeLoading}
                                reviewSelected={reviewSelection}
                                submitSelected={submitSelection}
                                generatingRound={generatingRound}
                                actionLoading={actionLoading}
                                onReviewSelectChange={setReviewSelection}
                                onSubmitSelectChange={setSubmitSelection}
                                onPageChange={(page, pageSize) => { setCreativePage(page); setCreativePageSize(pageSize); void loadCreatives(page, pageSize); }}
                                onFilterChange={(value) => { setReviewFilter(value); setCreativePage(1); setReviewSelection(new Set()); setSubmitSelection(new Set()); void loadCreatives(1, creativePageSize, value); }}
                                onReview={requestReview}
                                onBatchReview={batchReview}
                                onGenerate={generate}
                                onSubmit={() => setSubmitOpen(true)}
                                onGenerateReference={generateReference}
                                onOpen={(creative) => setActiveCreativeId(creative.id)}
                            />
                        ),
                    },
                    {
                        key: "jobs",
                        label: "生成任务",
                        children: (
                            <JobsPanel
                                detail={detail}
                                jobs={jobs}
                                page={jobPage}
                                pageSize={jobPageSize}
                                loading={jobLoading}
                                actionLoading={actionLoading}
                                onPageChange={(page, pageSize) => { setJobPage(page); setJobPageSize(pageSize); void loadJobs(page, pageSize); }}
                                onRetry={retryJob}
                                onRetryFailed={retryFailed}
                                onSubmit={() => setSubmitOpen(true)}
                                onCancelPending={cancelPending}
                                onPause={togglePause}
                                onResume={togglePause}
                            />
                        ),
                    },
                    { key: "materials", label: "素材与档案", children: <MaterialsPanel detail={detail} onAddBatch={() => setAddBatchOpen(true)} /> },
                ]}
            />

            <ProjectSettingsDrawer key={settingsOpen ? "settings-open" : "settings-closed"} open={settingsOpen} detail={detail} planningModels={planningModels} referenceModels={referenceModels} saving={savingSettings} onClose={() => setSettingsOpen(false)} onSave={saveSettings} />
            <PlanDialog key={planOpen ? "plan-open" : "plan-closed"} open={planOpen} detail={detail} settings={settings} submitting={planning} onClose={() => setPlanOpen(false)} onConfirm={confirmPlan} />
            <SubmitDialog key={submitOpen ? "submit-open" : "submit-closed"} open={submitOpen} detail={detail} models={models} settings={settings} selectedIds={Array.from(submitSelection)} submitting={submitting} onClose={() => setSubmitOpen(false)} onConfirm={confirmSubmit} />
            <AddBatchModal key={addBatchOpen ? "batch-open" : "batch-closed"} open={addBatchOpen} submitting={addingBatch} onClose={() => setAddBatchOpen(false)} onConfirm={confirmAddBatch} />
            {activeCreative ? (
                <CreativeDetailDrawer
                    key={`${activeCreative.id}-${activeCreative.updatedAt}`}
                    open={Boolean(activeCreativeId)}
                    creative={activeCreative}
                    detail={detail}
                    saving={actionLoading === `save-${activeCreative.id}`}
                    rematching={actionLoading === `rematch-${activeCreative.id}`}
                    onClose={() => setActiveCreativeId("")}
                    onSave={(draft) => saveCreative(activeCreative, draft)}
                    onRematch={() => rematchCreative(activeCreative)}
                />
            ) : null}
            <Modal
                open={rejectIds.length > 0}
                title={`驳回 ${rejectIds.length} 条创意`}
                okText="确认驳回"
                okButtonProps={{ danger: true, disabled: !rejectNote.trim(), loading: actionLoading === "reject-confirm" }}
                onOk={confirmReject}
                onCancel={() => { if (actionLoading !== "reject-confirm") setRejectIds([]); }}
            >
                <label className="grid gap-2 py-2 text-sm">
                    <span>驳回原因（必填）</span>
                    <Input.TextArea
                        value={rejectNote}
                        onChange={(event) => setRejectNote(event.target.value)}
                        placeholder="例如：文案不合格、图片不匹配、合规风险、产品不一致、完全重复"
                        autoSize={{ minRows: 3, maxRows: 6 }}
                        maxLength={500}
                    />
                </label>
            </Modal>
        </WorkspacePage>
    );
}
