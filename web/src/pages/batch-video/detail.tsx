import { App, Button, Input, Modal } from "antd";
import { Activity, ArrowLeft, Clapperboard, RefreshCw, Settings2, Sparkles, Upload, Zap } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router";

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
    type BatchVideoProjectDetail,
    type BatchVideoProjectSettings,
} from "@/services/api/batch-video";
import { CreativePanel } from "./components/creative-panel";
import { CreativeDetailDrawer } from "./components/creative-detail-drawer";
import { AnalysisProgressDialog } from "./components/analysis-progress-dialog";
import { AddBatchModal, PlanDialog, ProjectSettingsDrawer, SubmitDialog } from "./components/dialogs";
import { JobsPanel } from "./components/jobs-panel";
import { MassPanel } from "./components/mass-panel";
import { MaterialsPanel } from "./components/materials-panel";
import { ProductProfilePanel, ValidationPanel } from "./components/overview-panel";
import { TaskProgressDialog } from "./components/task-progress-dialog";
import { WorkflowNav } from "./components/workflow-nav";
import { isBatchVideoWorkflowStepKey, workflowSteps, type BatchVideoWorkflowStepKey } from "./shared";

type AnalysisFailurePolicy = "ask" | "continue" | "abort";
const ANALYSIS_FAILURE_POLICY_KEY = "batchVideo.analysisFailurePolicy";

function readAnalysisFailurePolicy(): AnalysisFailurePolicy {
    try {
        const value = window.localStorage.getItem(ANALYSIS_FAILURE_POLICY_KEY);
        return value === "continue" || value === "abort" ? value : "ask";
    } catch {
        return "ask";
    }
}

function writeAnalysisFailurePolicy(policy: AnalysisFailurePolicy) {
    try {
        window.localStorage.setItem(ANALYSIS_FAILURE_POLICY_KEY, policy);
    } catch {
        // 本地偏好写入失败不影响本次继续/中止。
    }
}

// 图片全部成功分析后按钮才变成“重新分析”；否则“分析素材”只补跑未完成的图片。纯文档项目按项目分析状态判断。
function isAnalysisRerun(detail: BatchVideoProjectDetail): boolean {
    const uploads = detail.referenceAssets.filter((asset) => asset.source !== "generated" && asset.resourceId);
    if (uploads.length === 0) return detail.project.analysisStatus === "succeeded";
    return uploads.every((asset) => asset.analysisStatus === "succeeded");
}

function hasActiveWork(detail: BatchVideoProjectDetail): boolean {
    if (detail.project.analysisStatus === "queued" || detail.project.analysisStatus === "running" || detail.project.analysisStatus === "awaiting_decision") return true;
    if (detail.rounds.some((round) => round.status === "queued" || round.status === "generating" || round.status === "running")) return true;
    if (detail.referenceAssets.some((asset) => asset.status === "generating")) return true;
    // 定时到点附近开始轮询，避免一次性 timer 撞上调度器 3 秒间隔后停在旧状态。
    if (detail.project.scheduledStartAt && new Date(detail.project.scheduledStartAt).getTime() - Date.now() <= 30_000) return true;
    return detail.jobs.some((job) => job.status === "waiting" || job.status === "dispatching" || job.status === "queued" || job.status === "running");
}

export default function BatchVideoDetailPage() {
    const { id = "" } = useParams();
    const navigate = useNavigate();
    const [searchParams, setSearchParams] = useSearchParams();
    const { message, modal } = App.useApp();
    const [detail, setDetail] = useState<BatchVideoProjectDetail | null>(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState("");
    const requestedStep = searchParams.get("step");
    const [step, setStep] = useState<BatchVideoWorkflowStepKey>(() => (isBatchVideoWorkflowStepKey(requestedStep) ? requestedStep : "upload"));
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
    const [analysisProgressOpen, setAnalysisProgressOpen] = useState(false);
    const [continuingAnalysis, setContinuingAnalysis] = useState(false);
    const autoAnalysisDecisionRef = useRef("");
    const [planningProgressOpen, setPlanningProgressOpen] = useState(false);
    const [planningRoundIds, setPlanningRoundIds] = useState<string[]>([]);
    const [abortingTask, setAbortingTask] = useState("");
    const activeRef = useRef(false);
    const stepInitializedRef = useRef("");

    const selectStep = (key: BatchVideoWorkflowStepKey) => {
        setStep(key);
        const next = new URLSearchParams(searchParams);
        next.set("step", key);
        setSearchParams(next, { replace: true });
    };

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
                return next;
            } catch (err) {
                if (!silent) setError(err instanceof Error ? err.message : "加载项目失败");
                return null;
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
        void Promise.all([batchVideoApi.listModels(), batchVideoApi.settings().catch(() => null)])
            .then(([videoModels, adminSettings]) => {
                setModels(videoModels);
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

    useEffect(() => {
        const scheduledAt = detail?.project.scheduledStartAt;
        if (!scheduledAt) return;
        const delay = Math.max(2000, new Date(scheduledAt).getTime() - Date.now() + 2000);
        if (delay > 2_147_483_647) return;
        const timer = window.setTimeout(() => { void refresh(true); }, delay);
        return () => window.clearTimeout(timer);
    }, [detail?.project.scheduledStartAt, refresh]);

    const analysisStatus = detail?.project.analysisStatus;
    useEffect(() => {
        if (analysisStatus === "queued" || analysisStatus === "running" || analysisStatus === "awaiting_decision") {
            setAnalysisProgressOpen(true);
        } else if (analysisStatus === "succeeded" || analysisStatus === "failed" || analysisStatus === "cancelled") {
            setAnalysisProgressOpen(false);
        }
    }, [analysisStatus]);

    const roundSignature = detail ? detail.rounds.map((round) => `${round.id}:${round.status}`).join("|") : "";
    useEffect(() => {
        if (!planningProgressOpen || planningRoundIds.length === 0) return;
        const scoped = (detail?.rounds || []).filter((round) => planningRoundIds.includes(round.id));
        if (scoped.length > 0 && !scoped.some((round) => round.status === "queued" || round.status === "generating" || round.status === "running")) {
            setPlanningProgressOpen(false);
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [planningProgressOpen, planningRoundIds, roundSignature]);

    useEffect(() => {
        if (!detail || stepInitializedRef.current === detail.project.id) return;
        stepInitializedRef.current = detail.project.id;
        if (isBatchVideoWorkflowStepKey(requestedStep)) {
            setStep(requestedStep);
            return;
        }
        const steps = workflowSteps(detail);
        const current = steps.find((item) => item.inProgress) || steps.find((item) => item.active && !item.done) || steps.find((item) => !item.done) || steps[0];
        if (current) setStep(current.key);
    }, [detail, requestedStep]);

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

    const abortAnalysis = (askAgain = true, persist = true) => {
        if (!detail) return;
        if (persist) writeAnalysisFailurePolicy(askAgain ? "ask" : "abort");
        setAbortingTask("analysis");
        void batchVideoApi.cancelAnalysis(detail.project.id)
            .then((next) => {
                setDetail(next);
                setAnalysisProgressOpen(false);
                message.success("分析已中止，素材和已生成的卖点保留");
            })
            .catch((err) => message.error(err instanceof Error ? err.message : "中止分析失败"))
            .finally(() => setAbortingTask(""));
    };

    const continueAnalysis = (askAgain = true, persist = true) => {
        if (!detail) return;
        if (persist) writeAnalysisFailurePolicy(askAgain ? "ask" : "continue");
        setContinuingAnalysis(true);
        void batchVideoApi.continueAnalysis(detail.project.id)
            .then((next) => {
                setDetail(next);
                message.success("已跳过失败图片，继续分析剩余素材");
            })
            .catch((err) => message.error(err instanceof Error ? err.message : "继续分析失败"))
            .finally(() => setContinuingAnalysis(false));
    };

    // 记住“下次不再询问”后，失败决策在后台自动执行；每个失败点只处理一次。
    useEffect(() => {
        const project = detail?.project;
        if (!project || project.analysisStatus !== "awaiting_decision") return;
        const key = `${project.id}:${project.analysisCurrentAssetId}:${project.analysisError || ""}`;
        if (autoAnalysisDecisionRef.current === key) return;
        autoAnalysisDecisionRef.current = key;
        const policy = readAnalysisFailurePolicy();
        if (policy === "continue") {
            message.info("已按上次选择自动继续分析");
            continueAnalysis(false, false);
        } else if (policy === "abort") {
            message.info("已按上次选择自动中止分析");
            abortAnalysis(false, false);
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [detail?.project.analysisStatus, detail?.project.analysisCurrentAssetId, detail?.project.analysisError]);

    const abortPlanning = () => {
        if (!detail) return;
        setAbortingTask("planning");
        void batchVideoApi.cancelPlanning(detail.project.id)
            .then((next) => {
                setDetail(next);
                setPlanningProgressOpen(false);
                message.success("策划已中止，已完成的创意保留");
            })
            .catch((err) => message.error(err instanceof Error ? err.message : "中止策划失败"))
            .finally(() => setAbortingTask(""));
    };

    const openPlanningProgress = () => {
        if (!detail) return;
        const active = detail.rounds.filter((round) => round.status === "queued" || round.status === "generating" || round.status === "running");
        setPlanningRoundIds((active.length > 0 ? active : detail.rounds).map((round) => round.id));
        setPlanningProgressOpen(true);
    };

    const deleteMaterial = (batchId: string, resourceId: string) => {
        if (!detail) return;
        modal.confirm({
            title: "删除这张素材？",
            content: "会同时从项目参考图库移除；若素材仍被其他批次或创意引用，则保留参考图和文件。",
            okText: "删除",
            okButtonProps: { danger: true },
            onOk: () =>
                runAction(`delete-material-${resourceId}`, async () => {
                    const next = await batchVideoApi.deleteMaterial(detail.project.id, batchId, resourceId);
                    setDetail(next);
                    await refresh(true);
                    message.success("素材已删除");
                }),
        });
    };

    const deleteMaterialBatch = (batchId: string, label: string) => {
        if (!detail) return;
        modal.confirm({
            title: `删除批次「${label || "未命名批次"}」？`,
            content: "会删除该批次的图片和文档，并清理未被其他批次或创意使用的参考图与文件。",
            okText: "删除批次",
            okButtonProps: { danger: true },
            onOk: () =>
                runAction(`delete-batch-${batchId}`, async () => {
                    const next = await batchVideoApi.deleteMaterialBatch(detail.project.id, batchId);
                    setDetail(next);
                    await refresh(true);
                    message.success("素材批次已删除");
                }),
        });
    };

    const clearMaterials = () => {
        if (!detail) return;
        modal.confirm({
            title: "清空项目素材？",
            content: "会删除所有素材批次、卖点分析依据和未被创意使用的参考图文件，且不可恢复。已有创意引用素材时会拒绝清空。",
            okText: "清空素材",
            okButtonProps: { danger: true },
            onOk: () =>
                runAction("clear-materials", async () => {
                    const next = await batchVideoApi.clearMaterials(detail.project.id);
                    setDetail(next);
                    await refresh(true);
                    message.success("素材已清空");
                }),
        });
    };

    const analyze = () => {
        if (!detail) return;
        const rerun = isAnalysisRerun(detail);
        const perform = () =>
            void runAction("analyze", async () => {
                const next = await batchVideoApi.analyze(detail.project.id, rerun ? "all" : "pending");
                setDetail(next);
                activeRef.current = true;
                setAnalysisProgressOpen(true);
                message.success(rerun ? "已提交全部图片重新分析" : "已提交未分析图片的分析任务");
            });
        if (rerun) {
            modal.confirm({ title: "重新分析全部素材？", content: "会按后台并行数重新调用视觉模型读取每张图片，覆盖已有图片描述与卖点；已有创意不会被删除。若只想补跑新素材，请返回上一步取消。", okText: "重新分析全部", onOk: perform });
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
                setPlanningRoundIds(result.roundIds || []);
                setPlanningProgressOpen(true);
                selectStep("review");
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
                selectStep("validation");
                message.success(`已提交 ${result.jobs} 个生成任务，请点击“开始生成”或设置定时`);
                return refresh(true);
            })
            .catch((err) => message.error(err instanceof Error ? err.message : "提交生成失败"))
            .finally(() => setSubmitting(false));
    };

    const startGeneration = (scheduledAt?: string) => {
        if (!detail) return;
        void runAction("start-generation", async () => {
            const result = await batchVideoApi.start(detail.project.id, scheduledAt);
            await refresh(true);
            message.success(scheduledAt ? `已定时，将于 ${new Date(scheduledAt).toLocaleString("zh-CN")} 开始 ${result.jobs} 条任务` : `已开始 ${result.jobs} 条生成任务`);
        });
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
            content: "会取消已提交待开始、定时等待和排队中的任务；已提交给模型的任务和已完成产物不受影响。",
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
                const current = await loadDetail(true);
                void loadCreatives();
                void loadJobs();
                if (current && (current.project.analysisStatus === "queued" || current.project.analysisStatus === "running" || current.project.analysisStatus === "awaiting_decision")) {
                    message.info("分析任务正在运行，完成后可再点“分析素材”补跑本次新素材");
                    return;
                }
                try {
                    const next = await batchVideoApi.analyze(detail.project.id);
                    setDetail(next);
                    activeRef.current = true;
                    setAnalysisProgressOpen(true);
                    message.success("素材已入库，分析任务已自动开始");
                } catch (err) {
                    message.warning(`素材已入库，但分析未能启动：${err instanceof Error ? err.message : "未知错误"}。请检查后台文本模型优先级后重试分析。`);
                }
            })
            .catch((err) => message.error(err instanceof Error ? err.message : "素材上传失败"))
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
    const analysisRunning = detail.project.analysisStatus === "queued" || detail.project.analysisStatus === "running" || detail.project.analysisStatus === "awaiting_decision";
    const hasMaterials = detail.materialBatches.some((batch) => batch.materials.length > 0 || batch.documents.length > 0);
    const planningScoped = planningRoundIds.length > 0 ? detail.rounds.filter((round) => planningRoundIds.includes(round.id)) : [];
    const planningTotal = planningScoped.length || planningRoundIds.length;
    const planningCompleted = planningScoped.filter((round) => round.status === "reviewing" || round.status === "failed").length;
    const planningActive = planningScoped.some((round) => round.status === "queued" || round.status === "generating" || round.status === "running");
    const activeCreative = creatives.find((creative) => creative.id === activeCreativeId) || detail.creatives.find((creative) => creative.id === activeCreativeId);
    const workflow = workflowSteps(detail);
    const currentStep = workflow.find((item) => item.key === step) || workflow[0];

    const creativePanel = (mode: "plan" | "review") => (
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
            mode={mode}
            onReviewSelectChange={setReviewSelection}
            onSubmitSelectChange={setSubmitSelection}
            onPageChange={(page, pageSize) => { setCreativePage(page); setCreativePageSize(pageSize); void loadCreatives(page, pageSize); }}
            onFilterChange={(value) => { setReviewFilter(value); setCreativePage(1); setReviewSelection(new Set()); setSubmitSelection(new Set()); void loadCreatives(1, creativePageSize, value); }}
            onReview={requestReview}
            onBatchReview={batchReview}
            onGenerateReference={generateReference}
            onOpen={(creative) => setActiveCreativeId(creative.id)}
        />
    );

    const jobsPanel = (
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
            onCancelPending={cancelPending}
            onPause={togglePause}
            onResume={togglePause}
            onStart={startGeneration}
        />
    );

    const stepContent = (() => {
        switch (step) {
            case "upload":
                return (
                    <div className="space-y-4">
                        <ProductProfilePanel detail={detail} />
                        <MaterialsPanel
                            detail={detail}
                            actionLoading={actionLoading}
                            onDeleteMaterial={deleteMaterial}
                            onDeleteBatch={deleteMaterialBatch}
                            onClearAll={clearMaterials}
                        />
                    </div>
                );
            case "creative":
                return creativePanel("plan");
            case "review":
                return creativePanel("review");
            case "validation":
                return <div className="space-y-4"><ValidationPanel detail={detail} />{jobsPanel}</div>;
            case "mass":
                return <MassPanel detail={detail} actionLoading={actionLoading} onGenerate={generate} onUnlock={unlockMass} />;
            default:
                return null;
        }
    })();

    const stepAction = (() => {
        switch (step) {
            case "upload":
                return (
                    <>
                        <Button type="primary" icon={<Upload className="size-4" />} onClick={() => setAddBatchOpen(true)}>
                            {hasMaterials ? "追加素材" : "上传素材"}
                        </Button>
                        {hasMaterials ? (
                            analysisRunning ? (
                                <Button icon={<Activity className="size-4" />} onClick={() => setAnalysisProgressOpen(true)}>
                                    查看进度
                                </Button>
                            ) : (
                                <Button icon={<Sparkles className="size-4" />} loading={actionLoading === "analyze"} onClick={analyze}>
                                    {isAnalysisRerun(detail) ? "重新分析" : "分析素材"}
                                </Button>
                            )
                        ) : null}
                    </>
                );
            case "creative":
                return (
                    <>
                        <Button icon={<Clapperboard className="size-4" />} onClick={generate}>
                            生成创意
                        </Button>
                        {generatingRound ? (
                            <Button icon={<Activity className="size-4" />} onClick={openPlanningProgress}>
                                查看进度
                            </Button>
                        ) : null}
                    </>
                );
            case "review":
                return (
                    <Button type="primary" icon={<Zap className="size-4" />} disabled={detail.counts.approved === 0} onClick={() => { setSubmitOpen(true); void batchVideoApi.listModels().then(setModels).catch((err) => message.error(err instanceof Error ? err.message : "加载视频模型失败")); }}>
                        提交生成
                    </Button>
                );
            default:
                return null;
        }
    })();

    const stageSummary = [
        detail.project.productName || "批量视频项目",
        batchVideoStageLabel(detail.project.stage),
        detail.project.massUnlockedAt ? "量产已解锁" : "",
        detail.project.status === "paused" ? "已暂停" : "",
        detail.counts.submitted > 0 ? `待开始 ${detail.counts.submitted} 条` : "",
        detail.project.scheduledStartAt ? `定时 ${new Date(detail.project.scheduledStartAt).toLocaleString("zh-CN")}` : "",
    ].filter(Boolean).join(" · ");

    return (
        <WorkspacePage>
            <PageHeader
                title={detail.project.name}
                description={stageSummary}
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

            <div className="mt-4 grid gap-4 lg:grid-cols-[248px_minmax(0,1fr)]">
                <aside className="min-w-0 lg:sticky lg:top-4 lg:self-start">
                    <div className="rounded-xl border border-border/70 bg-surface p-3">
                        <WorkflowNav steps={workflow} activeKey={step} onSelect={selectStep} />
                        <div className="mt-3 grid gap-2 border-t border-border/60 pt-3">
                            <Button className="!justify-start" icon={<ArrowLeft className="size-4" />} onClick={() => navigate("/batch-video")}>返回列表</Button>
                            <Button className="!justify-start" icon={<RefreshCw className="size-4" />} loading={loading} onClick={() => void refresh(false)}>刷新项目</Button>
                            <Button className="!justify-start" icon={<Settings2 className="size-4" />} onClick={() => setSettingsOpen(true)}>项目设置</Button>
                        </div>
                    </div>
                </aside>

                <section className="min-w-0 space-y-4">
                    <div className="rounded-xl border border-border/70 bg-surface px-4 py-3">
                        <div className="flex flex-wrap items-center gap-2">
                            <h2 className="text-base font-semibold">{currentStep.title}</h2>
                            {currentStep.inProgress ? (
                                <span className="rounded-full bg-primary/10 px-2 py-0.5 text-[10px] font-medium text-primary">进行中</span>
                            ) : currentStep.done ? (
                                <span className="rounded-full bg-emerald-500/10 px-2 py-0.5 text-[10px] font-medium text-emerald-600 dark:text-emerald-400">已完成</span>
                            ) : currentStep.active ? (
                                <span className="rounded-full bg-primary/10 px-2 py-0.5 text-[10px] font-medium text-primary">当前步骤</span>
                            ) : null}
                        </div>
                        <p className="mt-1 text-xs text-foreground/50">{currentStep.description}</p>
                        {stepAction ? <div className="mt-3 flex flex-wrap items-center gap-2">{stepAction}</div> : null}
                    </div>
                    {stepContent}
                </section>
            </div>

            <ProjectSettingsDrawer key={settingsOpen ? "settings-open" : "settings-closed"} open={settingsOpen} detail={detail} saving={savingSettings} onClose={() => setSettingsOpen(false)} onSave={saveSettings} />
            <PlanDialog key={planOpen ? "plan-open" : "plan-closed"} open={planOpen} detail={detail} settings={settings} submitting={planning} onClose={() => setPlanOpen(false)} onConfirm={confirmPlan} />
            <SubmitDialog key={submitOpen ? "submit-open" : "submit-closed"} open={submitOpen} detail={detail} models={models} settings={settings} selectedIds={Array.from(submitSelection)} submitting={submitting} onClose={() => setSubmitOpen(false)} onConfirm={confirmSubmit} />
            <AddBatchModal key={addBatchOpen ? "batch-open" : "batch-closed"} open={addBatchOpen} hasMaterials={hasMaterials} submitting={addingBatch} onClose={() => setAddBatchOpen(false)} onConfirm={confirmAddBatch} />
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
            <AnalysisProgressDialog
                open={analysisProgressOpen && analysisRunning}
                task={detail.analysisTask}
                assets={detail.referenceAssets}
                analysisError={detail.project.analysisError}
                awaitingDecision={detail.project.analysisStatus === "awaiting_decision"}
                aborting={abortingTask === "analysis"}
                continuing={continuingAnalysis}
                onAbort={abortAnalysis}
                onContinue={continueAnalysis}
            />
            <TaskProgressDialog
                open={planningProgressOpen && planningRoundIds.length > 0}
                title="创意策划进度"
                description={`分 ${planningTotal || planningRoundIds.length} 轮生成创意，完成后进入人工审核。中止后已完成的创意会保留。`}
                state={{
                    status: planningActive ? "running" : planningCompleted >= planningTotal && planningTotal > 0 ? "succeeded" : "running",
                    stage: `已完成 ${planningCompleted}/${planningTotal || planningRoundIds.length} 轮`,
                    progress: planningTotal > 0 ? Math.round((planningCompleted / planningTotal) * 100) : 5,
                    startedAt: planningScoped[0]?.createdAt,
                }}
                aborting={abortingTask === "planning"}
                onAbort={abortPlanning}
                onClose={() => setPlanningProgressOpen(false)}
            />
        </WorkspacePage>
    );
}
