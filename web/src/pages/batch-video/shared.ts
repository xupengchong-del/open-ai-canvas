import type { BatchVideoProjectDetail } from "@/services/api/batch-video";

export function reviewStatusLabel(value: string): string {
    switch (value) {
        case "approved":
            return "已通过";
        case "rejected":
            return "已驳回";
        default:
            return "待审核";
    }
}

export function reviewStatusColor(value: string): string {
    if (value === "approved") return "success";
    if (value === "rejected") return "error";
    return "warning";
}

export function jobStatusLabel(value: string): string {
    switch (value) {
        case "succeeded":
            return "成功";
        case "failed":
            return "失败";
        case "cancelled":
            return "已取消";
        case "running":
            return "生成中";
        case "dispatching":
            return "提交中";
        case "queued":
            return "已排队";
        case "waiting":
            return "等待调度";
        case "submitted":
            return "待开始";
        default:
            return value || "未知";
    }
}

export function jobStatusColor(value: string): string {
    if (value === "succeeded") return "success";
    if (value === "failed" || value === "cancelled") return "error";
    if (value === "running" || value === "dispatching" || value === "queued") return "processing";
    if (value === "submitted") return "warning";
    return "default";
}

export function roundStatusLabel(value: string): string {
    switch (value) {
        case "queued":
            return "排队中";
        case "generating":
        case "running":
            return "策划中";
        case "reviewing":
            return "待审核";
        case "failed":
            return "策划失败";
        default:
            return value || "草稿";
    }
}

export type BatchVideoWorkflowStepKey = "upload" | "creative" | "review" | "validation" | "mass";

export interface BatchVideoWorkflowStep {
    key: BatchVideoWorkflowStepKey;
    title: string;
    description: string;
    done: boolean;
    active: boolean;
    inProgress: boolean;
}

export function isBatchVideoWorkflowStepKey(value: string | null): value is BatchVideoWorkflowStepKey {
    return value === "upload" || value === "creative" || value === "review" || value === "validation" || value === "mass";
}

export function workflowSteps(detail: BatchVideoProjectDetail): BatchVideoWorkflowStep[] {
    const { project, materialBatches, rounds, counts, validation } = detail;
    const hasMaterials = materialBatches.some((batch) => batch.materials.length > 0 || batch.documents.length > 0);
    const analysisReady = project.analysisStatus === "succeeded";
    const hasRound = rounds.length > 0;
    const analysisAwaiting = project.analysisStatus === "awaiting_decision";
    const analysisRunning = project.analysisStatus === "queued" || project.analysisStatus === "running" || analysisAwaiting;
    const planningRunning = rounds.some((round) => round.status === "running" || round.status === "generating" || round.status === "queued");
    const reviewing = rounds.some((round) => round.status === "reviewing") || counts.pendingReview > 0;
    const submitted = counts.jobsTotal > 0;
    const unlocked = Boolean(project.massUnlockedAt);
    const uploadDescription = !hasMaterials
        ? "上传产品图片与卖点文档，上传后自动分析"
        : analysisAwaiting
          ? "有图片分析失败，请在进度窗确认继续或中止"
          : analysisRunning
            ? "分析中，完成后自动更新卖点与产品档案"
          : analysisReady
            ? "产品档案与卖点已生成"
            : project.analysisStatus === "failed"
              ? "分析失败，请重新分析"
              : project.analysisStatus === "cancelled"
                ? "分析已中止，可重新分析"
                : project.analysisStatus === "stale"
                  ? "素材已变更，请重新分析"
                  : "已有素材，等待分析";
    return [
        { key: "upload", title: "上传素材并分析", description: uploadDescription, done: analysisReady, active: !analysisReady, inProgress: analysisRunning },
        { key: "creative", title: "创意策划", description: hasRound ? `已生成 ${counts.creativeTotal} 条创意` : "按卖点和参数生成创意", done: counts.creativeTotal > 0, active: analysisReady && !hasRound, inProgress: planningRunning },
        { key: "review", title: "人工审核", description: reviewing ? "逐条或批量审核" : counts.approved > 0 ? `已通过 ${counts.approved} 条` : "审核通过后才能生成", done: counts.approved > 0 && !reviewing, active: hasRound && counts.pendingReview > 0, inProgress: false },
        { key: "validation", title: "标准验收", description: validation.passed ? "验收通过" : counts.submitted > 0 && counts.waiting + counts.running === 0 ? `已提交 ${counts.submitted} 条，等待开始` : `成功率 ${Math.round(validation.successRate * 100)}%`, done: validation.passed || unlocked, active: submitted && !validation.passed && !unlocked, inProgress: counts.waiting + counts.running > 0 },
        { key: "mass", title: "量产放量", description: unlocked ? "已解锁量产" : "标准验收通过后解锁", done: unlocked, active: validation.passed && !unlocked, inProgress: false },
    ];
}

export function formatPercent(value: number): string {
    if (!Number.isFinite(value)) return "0%";
    return `${Math.round(value * 100)}%`;
}

export function parseTags(raw: string): string[] {
    try {
        const value = JSON.parse(raw || "[]");
        return Array.isArray(value) ? value.map(String).filter(Boolean) : [];
    } catch {
        return [];
    }
}

export function templateLabel(value: string): string {
    switch (value) {
        case "comparison":
            return "对比型";
        case "unboxing":
            return "开箱型";
        case "scene":
            return "场景型";
        case "voiceover":
            return "口播型";
        case "pain_point":
            return "痛点型";
        default:
            return value || "模板";
    }
}
