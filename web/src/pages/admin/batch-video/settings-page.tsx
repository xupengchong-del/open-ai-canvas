import { App, Button, InputNumber, Select, Skeleton } from "antd";
import { RefreshCw, Save, Settings2 } from "lucide-react";
import { useCallback, useEffect, useState } from "react";

import { AdminPageFrame } from "../components/admin-shell";
import { AdminStatusBadge, SettingsSectionCard } from "../components/admin-ui";
import { batchVideoApi, type BatchVideoAdminSettings } from "@/services/api/batch-video";

type NumericKey = keyof Omit<BatchVideoAdminSettings, "defaultSubmissionMode">;

export default function BatchVideoSettingsPage() {
    const { message } = App.useApp();
    const [saved, setSaved] = useState<BatchVideoAdminSettings | null>(null);
    const [draft, setDraft] = useState<BatchVideoAdminSettings | null>(null);
    const [loading, setLoading] = useState(true);
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState("");

    const load = useCallback(async (announce = false) => {
        setLoading(true);
        setError("");
        try {
            const value = await batchVideoApi.adminSettings();
            setSaved(value);
            setDraft(value);
            if (announce) message.success("已重新读取批量视频配置");
        } catch (err) {
            setError(err instanceof Error ? err.message : "读取配置失败");
        } finally {
            setLoading(false);
        }
    }, [message]);

    useEffect(() => {
        void load(false);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    const updateNumber = (key: NumericKey, value: number | null) => {
        if (!draft) return;
        setDraft({ ...draft, [key]: value ?? 0 });
    };

    const save = async () => {
        if (!draft) return;
        setSaving(true);
        try {
            const value = await batchVideoApi.updateAdminSettings(draft);
            setSaved(value);
            setDraft(value);
            message.success("批量视频配置已保存");
        } catch (err) {
            message.error(err instanceof Error ? err.message : "保存失败");
        } finally {
            setSaving(false);
        }
    };

    const dirty = Boolean(draft && saved && JSON.stringify(draft) !== JSON.stringify(saved));

    return (
        <AdminPageFrame
            title="批量视频设置"
            description="集中管理标准验收、量产上限、参考图池和模型重试策略。"
            actions={
                <>
                    <Button icon={<RefreshCw className="size-4" />} onClick={() => void load(true)} loading={loading}>刷新</Button>
                    <Button type="primary" icon={<Save className="size-4" />} disabled={!dirty} loading={saving} onClick={() => void save()}>保存配置</Button>
                </>
            }
        >
            {error ? <div className="admin-empty-state">{error}</div> : null}
            {loading && !draft ? (
                <Skeleton active paragraph={{ rows: 8 }} />
            ) : draft ? (
                <div className="grid gap-4 xl:grid-cols-2">
                    <SettingsSectionCard
                        icon={<Settings2 className="size-4" />}
                        title="标准阶段"
                        description="所有新项目先走标准批量，通过验收后才能量产。"
                        status={<AdminStatusBadge label="服务端强制" tone="info" />}
                        layout="stacked"
                    >
                        <div className="grid gap-3 sm:grid-cols-2">
                            <Field label="标准轮默认创意数" hint="新建项目默认值，5–20 条" value={draft.standardCreativeCount} min={5} max={20} onChange={(value) => updateNumber("standardCreativeCount", value)} />
                            <Field label="标准轮最多创意数" hint="单轮上限，5–20 条" value={draft.maxStandardCreatives} min={5} max={20} onChange={(value) => updateNumber("maxStandardCreatives", value)} />
                            <Field label="最少成功条数" hint="标准验收要求" value={draft.standardMinSuccess} min={1} max={draft.maxStandardCreatives} onChange={(value) => updateNumber("standardMinSuccess", value)} />
                            <Field label="最低成功率" hint="0-1，例如 0.8" value={draft.standardSuccessRate} min={0} max={1} step={0.05} onChange={(value) => updateNumber("standardSuccessRate", value)} />
                        </div>
                    </SettingsSectionCard>

                    <SettingsSectionCard
                        icon={<Settings2 className="size-4" />}
                        title="量产阶段"
                        description="标准验收通过并人工确认后解锁，适合铺量场景。"
                        layout="stacked"
                    >
                        <div className="grid gap-3 sm:grid-cols-2">
                            <Field label="量产单轮最多创意数" hint="单项目和单轮均最多 1000" value={draft.maxMassCreatives} min={1} max={1000} onChange={(value) => updateNumber("maxMassCreatives", value)} />
                            <Field label="抽检比例" hint="0-1，例如 0.1" value={draft.samplingRate} min={0} max={1} step={0.05} onChange={(value) => updateNumber("samplingRate", value)} />
                        </div>
                    </SettingsSectionCard>

                    <SettingsSectionCard
                        icon={<Settings2 className="size-4" />}
                        title="参考图池"
                        description="参考图由程序按标签匹配；达到上限后不再自动扩容。"
                        layout="stacked"
                    >
                        <div className="grid gap-3 sm:grid-cols-3">
                            <Field label="标准阶段上限" value={draft.referencePoolStandard} min={1} max={500} onChange={(value) => updateNumber("referencePoolStandard", value)} />
                            <Field label="量产阶段上限" value={draft.referencePoolMass} min={1} max={500} onChange={(value) => updateNumber("referencePoolMass", value)} />
                            <Field label="单轮最多新增" value={draft.maxNewReferencesPerRound} min={1} max={500} onChange={(value) => updateNumber("maxNewReferencesPerRound", value)} />
                        </div>
                    </SettingsSectionCard>

                    <SettingsSectionCard
                        icon={<Settings2 className="size-4" />}
                        title="重试与提交"
                        description="同一模型失败达到次数后自动切换到下一个模型；总尝试次数 = 模型数 × 每模型上限。"
                        layout="stacked"
                    >
                        <div className="grid gap-3 sm:grid-cols-2">
                            <Field label="每模型最多尝试次数" value={draft.maxAttemptsPerModel} min={1} max={10} onChange={(value) => updateNumber("maxAttemptsPerModel", value)} />
                            <label className="grid gap-1.5 text-xs">
                                <span className="font-medium text-foreground/70">默认提交模式</span>
                                <Select
                                    value={draft.defaultSubmissionMode}
                                    onChange={(value) => setDraft({ ...draft, defaultSubmissionMode: value as "single" | "multi" })}
                                    options={[
                                        { value: "single", label: "模式 a：一次一个模型，失败切换" },
                                        { value: "multi", label: "模式 b：同时提交多个模型" },
                                    ]}
                                />
                            </label>
                        </div>
                    </SettingsSectionCard>

                    <div className="xl:col-span-2">
                        <SettingsSectionCard icon={<Settings2 className="size-4" />} title="模型批量授权" description="允许哪些视频模型用于批量生成，在“系统渠道 → 模型 → 能力与参数”里配置。" layout="stacked">
                            <p className="text-xs leading-5 text-foreground/60">
                                该开关默认关闭。只有显式开启“允许用于批量视频”的模型才会出现在提交弹窗；单次最多创意数也在模型能力里配置。建议只对单价低、并发额度充足的模型开启。
                            </p>
                        </SettingsSectionCard>
                    </div>
                </div>
            ) : null}
        </AdminPageFrame>
    );
}

function Field({
    label,
    hint,
    value,
    min,
    max,
    step = 1,
    onChange,
}: {
    label: string;
    hint?: string;
    value: number;
    min: number;
    max: number;
    step?: number;
    onChange: (value: number | null) => void;
}) {
    return (
        <label className="grid gap-1.5 text-xs">
            <span className="font-medium text-foreground/70">{label}</span>
            <InputNumber className="w-full" min={min} max={max} step={step} value={value} onChange={onChange} />
            {hint ? <span className="text-[var(--fs-tiny)] text-foreground/45">{hint}</span> : null}
        </label>
    );
}
