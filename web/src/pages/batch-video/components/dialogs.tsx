import { Button, Checkbox, Drawer, Input, InputNumber, Modal, Radio, Tag } from "antd";
import { useEffect, useMemo, useRef, useState } from "react";

import { Select } from "@/components/ui/base/select";
import { formatCredits } from "@/constant/credits";
import { batchVideoApi, type BatchVideoSubmissionInput, type BatchVideoSubmissionPreflight } from "@/services/api/batch-video";
import type {
    BatchVideoAdminSettings,
    BatchVideoDocumentInput,
    BatchVideoMaterialInput,
    BatchVideoModelOption,
    BatchVideoProjectDetail,
    BatchVideoProjectSettings,
} from "@/services/api/batch-video";
import { MaterialUploader } from "./material-uploader";

const DURATION_OPTIONS = [5, 8, 10, 15].map((value) => ({ value, label: `${value} 秒` }));
const ASPECT_OPTIONS = [
    { value: "9:16", label: "9:16 竖屏" },
    { value: "16:9", label: "16:9 横屏" },
    { value: "1:1", label: "1:1 方形" },
    { value: "3:4", label: "3:4 竖版" },
    { value: "4:3", label: "4:3 横版" },
];

export function ProjectSettingsDrawer({
    open,
    detail,
    saving,
    onClose,
    onSave,
}: {
    open: boolean;
    detail: BatchVideoProjectDetail;
    saving: boolean;
    onClose: () => void;
    onSave: (settings: BatchVideoProjectSettings) => void;
}) {
    const [draft, setDraft] = useState<BatchVideoProjectSettings>(detail.settings);
    const update = (patch: Partial<BatchVideoProjectSettings>) => setDraft((current) => ({ ...current, ...patch }));
    return (
        <Drawer
            open={open}
            onClose={onClose}
            title="项目设置"
            width={440}
            extra={
                <Button type="primary" loading={saving} onClick={() => onSave(draft)}>
                    保存
                </Button>
            }
        >
            <div className="grid gap-4">
                <div className="grid grid-cols-2 gap-3">
                    <label className="grid gap-1.5 text-sm">
                        <span className="font-medium">视频时长</span>
                        <Select value={draft.durationSeconds} onChange={(value) => update({ durationSeconds: Number(value) })} options={DURATION_OPTIONS} className="w-full" />
                    </label>
                    <label className="grid gap-1.5 text-sm">
                        <span className="font-medium">画面比例</span>
                        <Select value={draft.aspectRatio} onChange={(value) => update({ aspectRatio: String(value) })} options={ASPECT_OPTIONS} className="w-full" />
                    </label>
                </div>
                <label className="grid gap-1.5 text-sm">
                    <span className="font-medium">标准轮创意数量</span>
                    <InputNumber min={5} max={20} value={draft.standardCreativeCount} onChange={(value) => update({ standardCreativeCount: Number(value) || 5 })} className="w-full" />
                    <span className="text-xs text-foreground/45">标准验收可配置 5–20 条，默认 10 条。</span>
                </label>
                <div className="rounded-lg bg-amber-500/10 p-3 text-xs leading-5 text-amber-700 dark:text-amber-300">
                    修改参数后，已有分析会标记为过期；需要重新分析素材后再生成创意。
                </div>
            </div>
        </Drawer>
    );
}

export function PlanDialog({
    open,
    detail,
    settings,
    submitting,
    onClose,
    onConfirm,
}: {
    open: boolean;
    detail: BatchVideoProjectDetail;
    settings: BatchVideoAdminSettings | null;
    submitting: boolean;
    onClose: () => void;
    onConfirm: (payload: { count: number; mode: string; templates: string[] }) => void;
}) {
    const massUnlocked = Boolean(detail.project.massUnlockedAt);
    const maxStandard = settings?.maxStandardCreatives ?? 20;
    const maxMass = settings?.maxMassCreatives ?? 1000;
    const [count, setCount] = useState(Math.max(5, Math.min(maxStandard, detail.settings.standardCreativeCount || 10)));
    const [mode, setMode] = useState<"standard" | "mass">(massUnlocked && count > maxStandard ? "mass" : "standard");
    const limit = mode === "mass" && massUnlocked ? maxMass : maxStandard;
    const minCount = mode === "mass" && massUnlocked ? 1 : 5;
    const templates = [
        { id: "pain_point", label: "痛点型" },
        { id: "comparison", label: "对比型" },
        { id: "unboxing", label: "开箱型" },
        { id: "scene", label: "场景型" },
        { id: "voiceover", label: "口播型" },
    ];
    const [selectedTemplates, setSelectedTemplates] = useState<string[]>(templates.map((item) => item.id));
    return (
        <Modal
            open={open}
            title="生成创意"
            onCancel={onClose}
            width={520}
            footer={
                <>
                    <Button onClick={onClose}>取消</Button>
                    <Button type="primary" loading={submitting} disabled={count < minCount || count > limit || selectedTemplates.length === 0} onClick={() => onConfirm({ count, mode, templates: selectedTemplates })}>
                        开始策划
                    </Button>
                </>
            }
        >
            <div className="grid gap-4 py-2">
                <div className="flex flex-wrap gap-2 text-xs text-foreground/55">
                    <span>{detail.sellingPoints.length} 条卖点</span>
                    <span>·</span>
                    <span>{detail.referenceAssets.length} 张参考图</span>
                    <span>·</span>
                    <span>{detail.settings.durationSeconds}s / {detail.settings.aspectRatio}</span>
                </div>
                {massUnlocked ? (
                    <label className="grid gap-1.5 text-sm">
                        <span className="font-medium">生成模式</span>
                        <Radio.Group value={mode} onChange={(event) => { const nextMode = event.target.value as "standard" | "mass"; setMode(nextMode); setCount((value) => Math.max(nextMode === "mass" ? 1 : 5, Math.min(nextMode === "mass" ? maxMass : maxStandard, value))); }}>
                            <Radio.Button value="standard">标准轮（最多 {maxStandard} 条）</Radio.Button>
                            <Radio.Button value="mass">量产（最多 {maxMass} 条）</Radio.Button>
                        </Radio.Group>
                    </label>
                ) : (
                    <div className="rounded-lg bg-surface-active/50 px-3 py-2 text-xs text-foreground/55">当前为标准阶段，单轮最多 {maxStandard} 条；通过标准验收后可解锁量产。</div>
                )}
                <label className="grid gap-1.5 text-sm">
                    <span className="font-medium">创意数量</span>
                    <InputNumber min={minCount} max={limit} value={count} onChange={(value) => setCount(Math.max(minCount, Math.min(limit, Number(value) || minCount)))} className="w-full" />
                    <span className="text-xs text-foreground/45">{mode === "mass" ? "模型会按卖点和模板生成不重复的钩子与文案，数量越多耗时越长。" : "标准验收为 5–20 条，全部通过验证后才能解锁量产。"}</span>
                </label>
                <div className="grid gap-2 text-sm">
                    <span className="font-medium">模板范围</span>
                    <Checkbox.Group
                        value={selectedTemplates}
                        onChange={(values) => setSelectedTemplates(values.map(String))}
                        options={templates.map((item) => ({ label: item.label, value: item.id }))}
                    />
                </div>
                <div className="rounded-lg bg-primary/5 px-3 py-2 text-xs leading-5 text-foreground/60">
                    策划任务由服务端队列执行，可关闭页面；完成后创意会出现在“创意与审核”。
                </div>
            </div>
        </Modal>
    );
}

export function SubmitDialog({
    open,
    detail,
    models,
    settings,
    selectedIds,
    submitting,
    onClose,
    onConfirm,
}: {
    open: boolean;
    detail: BatchVideoProjectDetail;
    models: BatchVideoModelOption[];
    settings: BatchVideoAdminSettings | null;
    selectedIds: string[];
    submitting: boolean;
    onClose: () => void;
    onConfirm: (payload: { allApproved: boolean; generationMode: "standard" | "mass"; mode: "single" | "multi"; modelIds: string[] }) => void;
}) {
    const massUnlocked = Boolean(detail.project.massUnlockedAt);
    const [scope, setScope] = useState<"all" | "selected">("all");
    const [generationMode, setGenerationMode] = useState<"standard" | "mass">(massUnlocked ? "mass" : "standard");
    const [mode, setMode] = useState<"single" | "multi">(settings?.defaultSubmissionMode || "single");
    const [modelIds, setModelIds] = useState<string[]>([]);
    const [preflight, setPreflight] = useState<BatchVideoSubmissionPreflight | null>(null);
    const [preflightLoading, setPreflightLoading] = useState(false);
    const [preflightError, setPreflightError] = useState("");
    const totalCreatives = scope === "all" ? detail.counts.approved : selectedIds.length;
    const estimated = useMemo(() => totalCreatives * (mode === "multi" ? Math.max(1, modelIds.length) : 1), [mode, modelIds.length, totalCreatives]);
    const payload = (): BatchVideoSubmissionInput => ({
        creativeIds: scope === "all" ? [] : selectedIds,
        allApproved: scope === "all",
        generationMode,
        mode,
        models: modelIds.map((channelModelId) => {
            const model = models.find((item) => item.channelModelId === channelModelId);
            return { channelModelId, modelId: channelModelId, model: model?.model || "" };
        }),
    });
    const change = <T,>(setter: (value: T) => void, value: T) => {
        setter(value);
        setPreflight(null);
        setPreflightError("");
    };
    const check = async () => {
        setPreflightLoading(true);
        setPreflightError("");
        try {
            setPreflight(await batchVideoApi.preflight(detail.project.id, payload()));
        } catch (error) {
            setPreflight(null);
            setPreflightError(error instanceof Error ? error.message : "提交预检失败");
        } finally {
            setPreflightLoading(false);
        }
    };
    return (
        <Modal
            open={open}
            title="提交视频生成"
            onCancel={onClose}
            width={640}
            footer={
                <>
                    <Button onClick={onClose}>取消</Button>
                    <Button loading={preflightLoading} disabled={totalCreatives === 0 || modelIds.length === 0 || submitting} onClick={() => void check()}>
                        {preflight ? "重新预检" : "预检任务与费用"}
                    </Button>
                    <Button type="primary" loading={submitting} disabled={!preflight || preflightLoading} onClick={() => onConfirm({ allApproved: scope === "all", generationMode, mode, modelIds })}>
                        确认提交 {preflight?.jobCount || estimated} 个任务
                    </Button>
                </>
            }
        >
            <div className="grid gap-4 py-2">
                <div className="grid gap-1.5 text-sm">
                    <span className="font-medium">提交范围</span>
                    <Radio.Group value={scope} onChange={(event) => change(setScope, event.target.value as "all" | "selected")}>
                        <Radio.Button value="all">全部已通过（{detail.counts.approved} 条）</Radio.Button>
                        <Radio.Button value="selected" disabled={selectedIds.length === 0}>当前勾选（{selectedIds.length} 条）</Radio.Button>
                    </Radio.Group>
                </div>
                {massUnlocked ? (
                    <div className="grid gap-1.5 text-sm">
                        <span className="font-medium">生成阶段</span>
                        <Radio.Group value={generationMode} onChange={(event) => change(setGenerationMode, event.target.value as "standard" | "mass")}>
                            <Radio.Button value="mass">量产（使用已验收模型和模板）</Radio.Button>
                            <Radio.Button value="standard">标准复验（验证新模型或模板）</Radio.Button>
                        </Radio.Group>
                    </div>
                ) : null}
                <div className="grid gap-1.5 text-sm">
                    <span className="font-medium">提交方式</span>
                    <Radio.Group value={mode} onChange={(event) => change(setMode, event.target.value as "single" | "multi")}>
                        <Radio.Button value="single">模式 a：一次一个模型，失败自动切换</Radio.Button>
                        <Radio.Button value="multi">模式 b：同时提交多个模型</Radio.Button>
                    </Radio.Group>
                    <span className="text-xs text-foreground/45">
                        {mode === "single" ? "每个创意先走优先级最高的模型，失败达到上限后按优先级切换；预检费用不含后续重试。" : "每个创意同时提交到所选模型，任务数与费用按模型数量计算；调度时优先调用优先级高的模型。"}
                    </span>
                </div>
                <label className="grid gap-1.5 text-sm">
                    <span className="font-medium">视频模型（只显示后台开启批量生成的模型）</span>
                    <Select
                        mode="multiple"
                        value={modelIds}
                        onChange={(values) => change(setModelIds, values.map(String))}
                        options={[...models].sort((left, right) => right.priority - left.priority).map((model) => ({ value: model.channelModelId, label: `${model.displayName} · 优先级 ${model.priority} · 单次≤${model.maxItems} 条` }))}
                        placeholder={models.length === 0 ? "暂无可用模型" : "选择一个或多个模型"}
                        className="w-full"
                    />
                    {models.length === 0 ? (
                        <span className="text-xs text-amber-600 dark:text-amber-400">还没有视频模型开启批量授权。请管理员在“系统渠道 → 模型 → 能力与参数”中打开“允许用于批量视频”。</span>
                    ) : null}
                </label>
                <div className="rounded-lg bg-surface-active/50 px-3 py-2 text-xs leading-6 text-foreground/60">
                    <div>提交后不会立即调用模型；需要在任务面板点击“开始生成”或设置定时。</div>
                    <div>预计任务数：<span className="font-semibold text-foreground">{preflight?.jobCount ?? estimated}</span></div>
                    {preflight ? (
                        <>
                            <div>{preflight.estimated ? "预估" : "本次"}费用：<span className="font-semibold text-foreground">{formatCredits(preflight.totalMicrocredits)} 积分</span>；可用 {formatCredits(preflight.availableMicrocredits)} 积分</div>
                            <div>本地存储：{(preflight.storedBytes / 1024 ** 3).toFixed(2)} / {(preflight.storageLimitBytes / 1024 ** 3).toFixed(2)} GB；实际耗时取决于渠道并发与单条生成耗时。</div>
                        </>
                    ) : (
                        <div>提交前由服务端校验审核状态、模型授权、验收快照、余额和本地存储。</div>
                    )}
                </div>
                {preflightError ? <div className="rounded-lg bg-red-500/10 px-3 py-2 text-xs text-red-600 dark:text-red-400">{preflightError}</div> : null}
            </div>
        </Modal>
    );
}

export function AddBatchModal({
    open,
    hasMaterials,
    submitting,
    onClose,
    onConfirm,
}: {
    open: boolean;
    hasMaterials: boolean;
    submitting: boolean;
    onClose: () => void;
    onConfirm: (payload: { label: string; materials: BatchVideoMaterialInput[]; documents: BatchVideoDocumentInput[] }) => void;
}) {
    const [label, setLabel] = useState("");
    const [materials, setMaterials] = useState<BatchVideoMaterialInput[]>([]);
    const [documents, setDocuments] = useState<BatchVideoDocumentInput[]>([]);
    const uploadedRef = useRef<Set<string>>(new Set());
    const removedRef = useRef<Set<string>>(new Set());
    const openRef = useRef(open);
    useEffect(() => {
        openRef.current = open;
        if (!open) {
            setLabel("");
            setMaterials([]);
            setDocuments([]);
            uploadedRef.current.clear();
            removedRef.current.clear();
        }
    }, [open]);
    const discardUploads = (ids: Iterable<string>) => {
        const unique = Array.from(new Set(Array.from(ids).filter(Boolean)));
        if (unique.length === 0) return;
        void batchVideoApi.discardUploads(unique).catch(() => {});
    };
    const changeMaterials = (next: BatchVideoMaterialInput[]) => {
        const currentIDs = new Set(materials.map((item) => item.resourceId));
        const nextIDs = new Set(next.map((item) => item.resourceId));
        next.forEach((item) => {
            if (!currentIDs.has(item.resourceId)) {
                uploadedRef.current.add(item.resourceId);
                removedRef.current.delete(item.resourceId);
            }
        });
        materials.forEach((item) => {
            if (!nextIDs.has(item.resourceId)) removedRef.current.add(item.resourceId);
        });
        if (!openRef.current) {
            // 关闭后才完成的上传没有进入批次，直接回收，避免留下死文件。
            discardUploads(Array.from(uploadedRef.current));
            uploadedRef.current.clear();
            removedRef.current.clear();
            return;
        }
        setMaterials(next);
    };
    const cancel = () => {
        discardUploads(Array.from(uploadedRef.current));
        uploadedRef.current.clear();
        removedRef.current.clear();
        onClose();
    };
    const confirm = () => {
        // 被移除的已上传文件不属于本次提交，确认时立即回收。
        discardUploads(Array.from(removedRef.current));
        removedRef.current.clear();
        onConfirm({ label: label.trim(), materials, documents });
    };
    return (
        <Modal
            open={open}
            title={hasMaterials ? "追加素材批次" : "上传素材"}
            onCancel={cancel}
            width={760}
            footer={
                <>
                    <Button onClick={cancel}>取消</Button>
                    <Button type="primary" loading={submitting} disabled={materials.length === 0 && documents.length === 0} onClick={confirm}>
                        {hasMaterials ? "追加并分析" : "上传并分析"}
                    </Button>
                </>
            }
        >
            <div className="grid gap-4 py-2">
                <label className="grid gap-1.5 text-sm">
                    <span className="font-medium">批次名称（可选）</span>
                    <Input value={label} onChange={(event) => setLabel(event.target.value)} placeholder="留空自动使用当天日期" maxLength={160} />
                </label>
                <MaterialUploader materials={materials} onMaterialsChange={changeMaterials} documents={documents} onDocumentsChange={setDocuments} compact />
                <div className="rounded-lg bg-surface-active/50 px-3 py-2 text-xs text-foreground/55">
                    {hasMaterials ? "追加成功后自动开始分析，把新素材的卖点和参考图说明合并进产品档案；数字命名的图片会按数值升序排列。" : "上传成功后自动开始分析，生成产品档案、卖点和参考图说明；数字命名的图片会按数值升序排列。"}
                </div>
            </div>
        </Modal>
    );
}

export function CreativeTagSummary({ detail }: { detail: BatchVideoProjectDetail }) {
    return (
        <div className="flex flex-wrap gap-1.5">
            <Tag bordered={false}>{detail.settings.durationSeconds}s</Tag>
            <Tag bordered={false}>{detail.settings.aspectRatio}</Tag>
            <Tag bordered={false}>{detail.sellingPoints.length} 卖点</Tag>
        </div>
    );
}
