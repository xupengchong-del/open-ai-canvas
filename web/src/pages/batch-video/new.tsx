import { App, Button, Input } from "antd";
import { ArrowLeft, CheckCircle2 } from "lucide-react";
import { useState } from "react";
import { useNavigate } from "react-router";

import { PageHeader, WorkspacePage } from "@/components/layout/workspace-page";
import { batchVideoApi } from "@/services/api/batch-video";

export default function BatchVideoNewPage() {
    const navigate = useNavigate();
    const { message } = App.useApp();
    const [creating, setCreating] = useState(false);
    const [name, setName] = useState("");
    const [productName, setProductName] = useState("");

    const createProject = async () => {
        if (!name.trim()) return;
        setCreating(true);
        try {
            const project = await batchVideoApi.createProject({ name: name.trim(), productName: productName.trim() });
            message.success("项目已创建，请先上传素材");
            navigate(`/batch-video/${project.project.id}?step=upload`, { replace: true });
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
                description="只填写项目名称即可创建，其余步骤在项目内按顺序完成。"
                actions={
                    <Button icon={<ArrowLeft className="size-4" />} onClick={() => navigate("/batch-video")}>
                        返回列表
                    </Button>
                }
            />

            <div className="mt-4 max-w-2xl rounded-xl border border-border/70 bg-surface p-4 sm:p-6">
                <div className="grid gap-4">
                    <label className="grid gap-1.5 text-sm">
                        <span className="font-medium">项目名称</span>
                        <Input value={name} onChange={(event) => setName(event.target.value)} placeholder="例如：XX 智能手表 9 月铺量" maxLength={160} />
                        <span className="text-xs text-foreground/45">一个产品一个项目；不同 SKU 请新建项目。</span>
                    </label>
                    <label className="grid gap-1.5 text-sm">
                        <span className="font-medium">产品名称（可选）</span>
                        <Input value={productName} onChange={(event) => setProductName(event.target.value)} placeholder="用于策划文案的产品名" maxLength={160} />
                        <span className="text-xs text-foreground/45">留空时使用项目名称。</span>
                    </label>
                </div>

                <div className="mt-6 rounded-lg bg-primary/5 p-4 text-xs leading-5 text-foreground/60">
                    <div className="mb-2 text-sm font-semibold text-foreground/75">创建后按左侧流程继续</div>
                    <ul className="space-y-1.5">
                        <li className="flex gap-2"><CheckCircle2 className="mt-0.5 size-3.5 shrink-0" /> 1. 上传素材并分析：上传后自动提取卖点与参考图说明</li>
                        <li className="flex gap-2"><CheckCircle2 className="mt-0.5 size-3.5 shrink-0" /> 2. 创意策划：按卖点和参数生成创意</li>
                        <li className="flex gap-2"><CheckCircle2 className="mt-0.5 size-3.5 shrink-0" /> 3. 人工审核：审核通过后才能生成</li>
                        <li className="flex gap-2"><CheckCircle2 className="mt-0.5 size-3.5 shrink-0" /> 4. 标准验收：验证成功率与参考图</li>
                        <li className="flex gap-2"><CheckCircle2 className="mt-0.5 size-3.5 shrink-0" /> 5. 量产放量：标准验收通过后解锁</li>
                    </ul>
                </div>

                <div className="mt-6 flex justify-end border-t border-border/60 pt-4">
                    <Button type="primary" loading={creating} disabled={!name.trim()} onClick={() => void createProject()}>
                        创建项目
                    </Button>
                </div>
            </div>
        </WorkspacePage>
    );
}
