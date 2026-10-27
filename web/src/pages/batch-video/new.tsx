import { App, Button, Input, Steps, Tag } from "antd";
import { ArrowLeft, ArrowRight, CheckCircle2, Sparkles } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router";

import { PageHeader, WorkspacePage } from "@/components/layout/workspace-page";
import { Select } from "@/components/ui/base/select";
import {
    batchVideoApi,
    type BatchVideoDocumentInput,
    type BatchVideoMaterialInput,
    type BatchVideoPlanningModelOption,
    type BatchVideoReferenceModelOption,
} from "@/services/api/batch-video";
import { MaterialUploader } from "./components/material-uploader";

const DURATION_OPTIONS = [5, 8, 10, 15].map((value) => ({ value, label: `${value} 秒` }));
const ASPECT_OPTIONS = [
    { value: "9:16", label: "9:16 竖屏" },
    { value: "16:9", label: "16:9 横屏" },
    { value: "1:1", label: "1:1 方形" },
    { value: "3:4", label: "3:4 竖版" },
    { value: "4:3", label: "4:3 横版" },
];

export default function BatchVideoNewPage() {
    const navigate = useNavigate();
    const { message } = App.useApp();
    const [step, setStep] = useState(0);
    const [creating, setCreating] = useState(false);
    const [planningModels, setPlanningModels] = useState<BatchVideoPlanningModelOption[]>([]);
    const [referenceModels, setReferenceModels] = useState<BatchVideoReferenceModelOption[]>([]);
    const [name, setName] = useState("");
    const [productName, setProductName] = useState("");
    const [planningChannelModelId, setPlanningChannelModelId] = useState("");
    const [referenceModelId, setReferenceModelId] = useState("");
    const [durationSeconds, setDurationSeconds] = useState(8);
    const [aspectRatio, setAspectRatio] = useState("9:16");
    const [standardCreativeCount, setStandardCreativeCount] = useState(10);
    const [materials, setMaterials] = useState<BatchVideoMaterialInput[]>([]);
    const [documents, setDocuments] = useState<BatchVideoDocumentInput[]>([]);
    const [sellingPointText, setSellingPointText] = useState("");

    useEffect(() => {
        void Promise.all([
            batchVideoApi.listPlanningModels(),
            batchVideoApi.listReferenceModels(),
            batchVideoApi.settings().catch(() => null),
        ]).then(([models, references, settings]) => {
            setPlanningModels(models);
            setReferenceModels(references);
            const visionModel = models.find((item) => item.maxImages > 0);
            const defaultModel = visionModel || models[0];
            if (defaultModel) setPlanningChannelModelId((current) => current || defaultModel.channelModelId);
            if (settings) setStandardCreativeCount(Math.max(5, Math.min(20, settings.standardCreativeCount)));
        }).catch((error) => message.error(error instanceof Error ? error.message : "加载模型配置失败"));
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    const selectedPlanningModel = useMemo(() => planningModels.find((model) => model.channelModelId === planningChannelModelId), [planningChannelModelId, planningModels]);
    const validDocuments = documents.filter((document) => document.text.trim());
    const hasSource = materials.length > 0 || validDocuments.length > 0;
    const modelSupportsSource = Boolean(selectedPlanningModel) && (materials.length === 0 || Boolean(selectedPlanningModel?.maxImages));
    const canCreate = Boolean(name.trim() && productName.trim() && hasSource && modelSupportsSource);
    const canNext = step === 0 ? Boolean(name.trim() && productName.trim() && hasSource) : step === 1 ? modelSupportsSource : true;

    const createProject = async () => {
        setCreating(true);
        try {
            const project = await batchVideoApi.createProject({
                name: name.trim(),
                productName: productName.trim(),
                settings: { durationSeconds, aspectRatio, standardCreativeCount, planningChannelModelId, referenceModelId },
                materials,
                documents: validDocuments,
                sellingPoints: sellingPointText
                    .split("\n")
                    .map((line) => line.trim())
                    .filter(Boolean)
                    .map((text) => ({ text, source: "manual" })),
            });
            await batchVideoApi.analyze(project.project.id);
            message.success("项目已创建，素材分析已开始");
            navigate(`/batch-video/${project.project.id}`, { replace: true });
        } catch (error) {
            message.error(error instanceof Error ? error.message : "创建项目失败");
        } finally {
            setCreating(false);
        }
    };

    return (
        <WorkspacePage>
            <PageHeader
                title="新建批量视频项目"
                description="一个项目对应一个产品。创建后先做素材分析，再生成标准创意并人工审核。"
                actions={
                    <Button icon={<ArrowLeft className="size-4" />} onClick={() => navigate("/batch-video")}>
                        返回列表
                    </Button>
                }
            />

            <div className="mt-4 rounded-xl border border-border/70 bg-surface p-4 sm:p-6">
                <Steps
                    current={step}
                    items={[{ title: "产品与素材" }, { title: "参数与模型" }, { title: "确认创建" }]}
                    size="small"
                />

                <div className="mt-6">
                    {step === 0 ? (
                        <div className="grid gap-5">
                            <div className="grid max-w-3xl gap-4 sm:grid-cols-2">
                                <label className="grid gap-1.5 text-sm">
                                    <span className="font-medium">项目名称</span>
                                    <Input value={name} onChange={(event) => setName(event.target.value)} placeholder="例如：XX 智能手表 9 月铺量" maxLength={160} />
                                    <span className="text-xs text-foreground/45">一个产品一个项目；不同 SKU 请新建项目。</span>
                                </label>
                                <label className="grid gap-1.5 text-sm">
                                    <span className="font-medium">产品名称</span>
                                    <Input value={productName} onChange={(event) => setProductName(event.target.value)} placeholder="用于策划文案的产品名" maxLength={160} />
                                </label>
                            </div>
                            <MaterialUploader materials={materials} onMaterialsChange={setMaterials} documents={documents} onDocumentsChange={setDocuments} />
                            {materials.length === 0 ? (
                                <Tag color="warning" bordered={false} className="w-fit">可以只凭文档创建，但缺少产品图会导致外观、包装和场景描述偏差，也无法直接匹配产品参考图。</Tag>
                            ) : null}
                        </div>
                    ) : step === 1 ? (
                        <div className="grid max-w-3xl gap-4">
                            <label className="grid gap-1.5 text-sm">
                                <span className="font-medium">策划模型</span>
                                <Select
                                    value={planningChannelModelId}
                                    onChange={(value) => setPlanningChannelModelId(String(value))}
                                    options={planningModels.map((model) => ({
                                        value: model.channelModelId,
                                        label: `${model.displayName}${model.maxImages > 0 ? ` · 视觉 ${model.maxImages} 图` : " · 纯文本"}`,
                                        disabled: materials.length > 0 && model.maxImages <= 0,
                                    }))}
                                    placeholder={materials.length > 0 ? "选择具备视觉能力的文本模型" : "选择文本或视觉模型"}
                                    className="w-full"
                                />
                                <span className="text-xs text-foreground/45">{materials.length > 0 ? "当前包含产品图片，策划模型必须支持读图。" : "当前只有文档，可使用纯文本模型；后续追加图片时需改用视觉模型。"}</span>
                            </label>
                            <label className="grid gap-1.5 text-sm">
                                <span className="font-medium">参考图生成模型（可选）</span>
                                <Select
                                    value={referenceModelId}
                                    onChange={(value) => setReferenceModelId(String(value || ""))}
                                    options={[{ value: "", label: "不生成，只使用上传素材" }, ...referenceModels.map((model) => ({ value: model.channelModelId, label: model.displayName }))]}
                                    className="w-full"
                                />
                                <span className="text-xs text-foreground/45">创意缺少可用参考图时按图片提示词生成，生成图只保存在服务器本地。</span>
                            </label>
                            <div className="grid gap-4 sm:grid-cols-3">
                                <label className="grid gap-1.5 text-sm">
                                    <span className="font-medium">视频时长</span>
                                    <Select value={durationSeconds} onChange={(value) => setDurationSeconds(Number(value))} options={DURATION_OPTIONS} className="w-full" />
                                </label>
                                <label className="grid gap-1.5 text-sm">
                                    <span className="font-medium">画面比例</span>
                                    <Select value={aspectRatio} onChange={(value) => setAspectRatio(String(value))} options={ASPECT_OPTIONS} className="w-full" />
                                </label>
                                <label className="grid gap-1.5 text-sm">
                                    <span className="font-medium">标准验证创意数</span>
                                    <Input
                                        type="number"
                                        min={5}
                                        max={20}
                                        value={standardCreativeCount}
                                        onChange={(event) => setStandardCreativeCount(Math.max(5, Math.min(20, Number(event.target.value) || 5)))}
                                    />
                                    <span className="text-xs text-foreground/45">标准验收可配置 5–20 条。</span>
                                </label>
                            </div>
                            {materials.length > 0 && !planningModels.some((model) => model.maxImages > 0) ? (
                                <Tag color="warning" bordered={false} className="w-fit">当前没有支持读图的文本模型。请先在后台系统渠道中启用视觉文本模型并配置图片引用上限。</Tag>
                            ) : null}
                        </div>
                    ) : (
                        <div className="grid gap-5 lg:grid-cols-[1.2fr_0.8fr]">
                            <div className="space-y-4">
                                <div className="rounded-xl border border-border/70 p-4">
                                    <h3 className="text-sm font-semibold">项目信息</h3>
                                    <dl className="mt-3 grid grid-cols-2 gap-x-4 gap-y-2 text-xs">
                                        <dt className="text-foreground/50">项目名称</dt>
                                        <dd className="text-right font-medium">{name}</dd>
                                        <dt className="text-foreground/50">产品名称</dt>
                                        <dd className="text-right font-medium">{productName || "未填写"}</dd>
                                        <dt className="text-foreground/50">策划模型</dt>
                                        <dd className="text-right font-medium">{selectedPlanningModel?.displayName || "未选择"}</dd>
                                        <dt className="text-foreground/50">参考图模型</dt>
                                        <dd className="text-right font-medium">{referenceModels.find((model) => model.channelModelId === referenceModelId)?.displayName || "不生成"}</dd>
                                        <dt className="text-foreground/50">视频参数</dt>
                                        <dd className="text-right font-medium">{durationSeconds}s · {aspectRatio}</dd>
                                        <dt className="text-foreground/50">标准创意数</dt>
                                        <dd className="text-right font-medium">{standardCreativeCount} 条</dd>
                                        <dt className="text-foreground/50">素材 / 文档</dt>
                                        <dd className="text-right font-medium">{materials.length} 图 / {documents.length} 文档</dd>
                                    </dl>
                                </div>
                                <label className="grid gap-1.5 text-sm">
                                    <span className="font-medium">手动补充卖点（可选，每行一条）</span>
                                    <Input.TextArea
                                        value={sellingPointText}
                                        onChange={(event) => setSellingPointText(event.target.value)}
                                        autoSize={{ minRows: 4, maxRows: 10 }}
                                        placeholder={"例如：\n续航 14 天\n血氧监测\nIP68 防水"}
                                    />
                                    <span className="text-xs text-foreground/45">不填写时由策划模型从素材和文档中提取。</span>
                                </label>
                            </div>
                            <div className="rounded-xl border border-primary/25 bg-primary/5 p-4">
                                <div className="flex items-center gap-2 text-sm font-semibold">
                                    <Sparkles className="size-4" /> 创建后自动开始分析
                                </div>
                                <ul className="mt-3 space-y-2 text-xs leading-5 text-foreground/65">
                                    <li className="flex gap-2"><CheckCircle2 className="mt-0.5 size-3.5 shrink-0" /> 分析模型读取产品图片和卖点文档，生成产品档案与卖点。</li>
                                    <li className="flex gap-2"><CheckCircle2 className="mt-0.5 size-3.5 shrink-0" /> 参考图库按内容哈希去重，同一素材不会重复上传。</li>
                                    <li className="flex gap-2"><CheckCircle2 className="mt-0.5 size-3.5 shrink-0" /> 标准轮创意需要逐条或批量审核，通过后才能提交生成。</li>
                                </ul>
                            </div>
                        </div>
                    )}
                </div>

                <div className="mt-6 flex items-center justify-between border-t border-border/60 pt-4">
                    <Button disabled={step === 0 || creating} onClick={() => setStep((value) => Math.max(0, value - 1))}>
                        上一步
                    </Button>
                    <div className="flex items-center gap-2">
                        {step < 2 ? (
                            <Button type="primary" disabled={!canNext} onClick={() => setStep((value) => Math.min(2, value + 1))}>
                                下一步 <ArrowRight className="size-4" />
                            </Button>
                        ) : (
                            <Button type="primary" loading={creating} disabled={!canCreate} onClick={() => void createProject()}>
                                创建并开始分析
                            </Button>
                        )}
                    </div>
                </div>
            </div>
        </WorkspacePage>
    );
}
