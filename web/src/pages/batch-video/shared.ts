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
        case "queued":
            return "已排队";
        case "waiting":
            return "等待调度";
        default:
            return value || "未知";
    }
}

export function jobStatusColor(value: string): string {
    if (value === "succeeded") return "success";
    if (value === "failed" || value === "cancelled") return "error";
    if (value === "running" || value === "queued") return "processing";
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

export function workflowSteps(detail: BatchVideoProjectDetail): { title: string; description: string; done: boolean; active: boolean; inProgress: boolean }[] {
    const { project, materialBatches, rounds, counts, validation } = detail;
    const hasMaterials = materialBatches.some((batch) => batch.materials.length > 0);
    const analysisReady = project.analysisStatus === "succeeded";
    const hasRound = rounds.length > 0;
    const analysisRunning = project.analysisStatus === "queued" || project.analysisStatus === "running";
    const planningRunning = rounds.some((round) => round.status === "running" || round.status === "generating" || round.status === "queued");
    const reviewing = rounds.some((round) => round.status === "reviewing") || counts.pendingReview > 0;
    const submitted = counts.jobsTotal > 0;
    const unlocked = Boolean(project.massUnlockedAt);
    return [
        { title: "上传素材", description: "产品图片与卖点文档", done: hasMaterials, active: !hasMaterials, inProgress: false },
        { title: "素材分析", description: analysisReady ? "产品档案与卖点已生成" : project.analysisStatus === "failed" ? "分析失败，请重试" : "由视觉模型提取卖点", done: analysisReady, active: hasMaterials && !analysisReady, inProgress: analysisRunning },
        { title: "创意策划", description: hasRound ? `已生成 ${counts.creativeTotal} 条创意` : "按卖点和参数生成创意", done: counts.creativeTotal > 0, active: analysisReady && !hasRound, inProgress: planningRunning },
        { title: "人工审核", description: reviewing ? "逐条或批量审核" : counts.approved > 0 ? `已通过 ${counts.approved} 条` : "审核通过后才能生成", done: counts.approved > 0 && !reviewing, active: hasRound && counts.pendingReview > 0, inProgress: false },
        { title: "标准验收", description: validation.passed ? "验收通过" : `成功率 ${Math.round(validation.successRate * 100)}%`, done: validation.passed || unlocked, active: submitted && !validation.passed && !unlocked, inProgress: counts.waiting + counts.running > 0 },
        { title: "量产放量", description: unlocked ? "已解锁量产" : "标准验收通过后解锁", done: unlocked, active: validation.passed && !unlocked, inProgress: false },
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
