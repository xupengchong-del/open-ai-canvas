import { http } from "@/services/api/request";

export type BatchVideoProjectSettings = {
    durationSeconds: number;
    aspectRatio: string;
    standardCreativeCount: number;
    templates?: string[];
};

export type BatchVideoProject = {
    id: string;
    userId: string;
    name: string;
    productName: string;
    stage: string;
    status: string;
    settingsJson: string;
    productProfileJson: string;
    currentProfileVersion: number;
    referencePoolVersion: number;
    analysisTaskId?: string;
    analysisStatus?: string;
    analysisError?: string;
    analysisCurrentAssetId?: string;
    scheduledStartAt?: string | null;
    massUnlockedAt?: string | null;
    createdAt: string;
    updatedAt: string;
};

export type BatchVideoMaterialInput = {
    resourceId: string;
    kind: string;
    title: string;
    description: string;
    tags: string[];
};

export type BatchVideoDocumentInput = {
    name: string;
    text: string;
};

export type BatchVideoSellingPoint = {
    id: string;
    projectId: string;
    canonicalText: string;
    evidenceJson: string;
    confidence: number;
    source: string;
    status: string;
    usageCount: number;
};

export type BatchVideoMaterialBatch = {
    id: string;
    projectId: string;
    batchNo: number;
    label: string;
    status: string;
    visualVersion: number;
    createdAt: string;
    materials: BatchVideoMaterialInput[];
    documents: BatchVideoDocumentInput[];
};

export type BatchVideoReferenceAsset = {
    id: string;
    projectId: string;
    resourceId: string;
    contentHash: string;
    title: string;
    description: string;
    imagePrompt: string;
    tagsJson: string;
    source: string;
    status: string;
    version: number;
    analysisStatus?: string;
    analysisTaskId?: string;
    analysisError?: string;
    analysisResultJson?: string;
    analyzedAt?: string | null;
    usageCount: number;
};

export type BatchVideoCreative = {
    id: string;
    projectId: string;
    roundId: string;
    seq: number;
    templateId: string;
    sellingPointId: string;
    title: string;
    hook: string;
    copywriting: string;
    videoPrompt: string;
    imagePrompt: string;
    imageRequirementJson: string;
    durationSeconds: number;
    aspectRatio: string;
    matchedReferenceAssetId: string;
    matchStatus: string;
    matchMethod: string;
    matchScore: number;
    reviewStatus: string;
    reviewNote: string;
    duplicateHash: string;
    createdAt: string;
    updatedAt: string;
};

export type BatchVideoCreativeRound = {
    id: string;
    projectId: string;
    roundNo: number;
    mode: string;
    requestedCount: number;
    profileVersion: number;
    taskId?: string;
    modelId?: string;
    error?: string;
    status: string;
    createdAt: string;
};

export type BatchVideoGenerationJob = {
    id: string;
    projectId: string;
    creativeId: string;
    modelId: string;
    channelModelId: string;
    mode: string;
    status: string;
    modelPriority: number;
    currentTaskId: string;
    attemptsUsed: number;
    failoverCount: number;
    retryRound: number;
    successResourceId: string;
    lastError: string;
    progress: number;
    stage: string;
    createdAt: string;
    updatedAt: string;
};

export type BatchVideoAttempt = {
    id: string;
    jobId: string;
    creativeId: string;
    attemptNo: number;
    retryRound: number;
    modelId: string;
    taskId: string;
    status: string;
    error: string;
    switchedFromModelId: string;
    startedAt?: string;
    finishedAt?: string;
    createdAt: string;
};

export type BatchVideoSubmissionInput = {
    creativeIds: string[];
    allApproved: boolean;
    generationMode: "standard" | "mass";
    mode: "single" | "multi";
    models: { channelModelId: string; modelId: string; model: string }[];
};

export type BatchVideoSubmissionPreflight = {
    jobCount: number;
    totalMicrocredits: number;
    availableMicrocredits: number;
    estimated: boolean;
    storedBytes: number;
    storageLimitBytes: number;
    generationMode: "standard" | "mass";
    submissionFingerprint: string;
};

export type BatchVideoProjectCounts = {
    creativeTotal: number;
    approved: number;
    pendingReview: number;
    rejected: number;
    jobsTotal: number;
    submitted: number;
    waiting: number;
    running: number;
    succeeded: number;
    failed: number;
    cancelled: number;
    attempts: number;
    retrySucceeded: number;
    retryFailed: number;
};

export type BatchVideoValidationCheck = {
    key: string;
    label: string;
    passed: boolean;
    current: unknown;
    required: unknown;
};

export type BatchVideoValidationState = {
    passed: boolean;
    standardRoundId?: string;
    standardCreativeCount: number;
    approvedCount: number;
    successCount: number;
    successRate: number;
    templateIds: string[];
    modelIds: string[];
    checks: BatchVideoValidationCheck[];
    validation?: {
        id: string;
        status: string;
        successRate: number;
        unlockedAt?: string;
    };
};

export type BatchVideoAnalysisTask = {
    id: string;
    status: string;
    stage: string;
    progress: number;
    error: string;
    assetId?: string;
    assetTitle?: string;
    total: number;
    done: number;
    failed: number;
    running: number;
    createdAt: string;
    startedAt?: string | null;
    completedAt?: string | null;
};

export type BatchVideoProjectDetail = {
    project: BatchVideoProject;
    settings: BatchVideoProjectSettings;
    analysisTask?: BatchVideoAnalysisTask;
    materialBatches: BatchVideoMaterialBatch[];
    sellingPoints: BatchVideoSellingPoint[];
    referenceAssets: BatchVideoReferenceAsset[];
    rounds: BatchVideoCreativeRound[];
    creatives: BatchVideoCreative[];
    jobs: BatchVideoGenerationJob[];
    counts: BatchVideoProjectCounts;
    validation: BatchVideoValidationState;
};

export type BatchVideoProjectSummary = {
    project: BatchVideoProject;
    settings: BatchVideoProjectSettings;
    counts: BatchVideoProjectCounts;
    validation: BatchVideoValidationState;
};

export type BatchVideoAdminSettings = {
    standardCreativeCount: number;
    standardMinSuccess: number;
    standardSuccessRate: number;
    maxStandardCreatives: number;
    maxMassCreatives: number;
    referencePoolStandard: number;
    referencePoolMass: number;
    maxNewReferencesPerRound: number;
    maxAttemptsPerModel: number;
    defaultSubmissionMode: "single" | "multi";
    samplingRate: number;
    analysisConcurrency: number;
};

export type BatchVideoModelOption = {
    channelModelId: string;
    channelId: string;
    model: string;
    displayName: string;
    maxItems: number;
    priority: number;
    sortOrder: number;
};

export type CreateBatchVideoProjectInput = {
    name: string;
    productName?: string;
    settings?: BatchVideoProjectSettings;
    materials?: BatchVideoMaterialInput[];
    documents?: BatchVideoDocumentInput[];
    sellingPoints?: { text: string; evidence?: string; source?: string }[];
};

export const batchVideoApi = {
    listProjects: (page = 1, pageSize = 20, stage = "") => http.get<BatchVideoProjectSummary[]>("/batch-video/projects", { params: { page, pageSize, stage: stage || undefined } }),
    createProject: (input: CreateBatchVideoProjectInput) => http.post<BatchVideoProjectDetail>("/batch-video/projects", input),
    getProject: (id: string) => http.get<BatchVideoProjectDetail>(`/batch-video/projects/${encodeURIComponent(id)}`),
    deleteProject: (id: string) => http.delete<{ deleted: boolean }>(`/batch-video/projects/${encodeURIComponent(id)}`),
    updateSettings: (id: string, settings: BatchVideoProjectSettings) => http.patch<BatchVideoProjectDetail>(`/batch-video/projects/${encodeURIComponent(id)}/settings`, settings),
    addMaterialBatch: (id: string, payload: { label: string; materials: BatchVideoMaterialInput[]; documents: BatchVideoDocumentInput[] }) =>
        http.post<BatchVideoMaterialBatch>(`/batch-video/projects/${encodeURIComponent(id)}/material-batches`, payload),
    deleteMaterialBatch: (id: string, batchId: string) =>
        http.delete<BatchVideoProjectDetail>(`/batch-video/projects/${encodeURIComponent(id)}/material-batches/${encodeURIComponent(batchId)}`),
    deleteMaterial: (id: string, batchId: string, resourceId: string) =>
        http.delete<BatchVideoProjectDetail>(`/batch-video/projects/${encodeURIComponent(id)}/material-batches/${encodeURIComponent(batchId)}/materials/${encodeURIComponent(resourceId)}`),
    clearMaterials: (id: string) =>
        http.delete<BatchVideoProjectDetail>(`/batch-video/projects/${encodeURIComponent(id)}/material-batches`),
    analyze: (id: string, scope: "pending" | "all" = "pending") => http.post<BatchVideoProjectDetail>(`/batch-video/projects/${encodeURIComponent(id)}/analyze`, { scope }),
    continueAnalysis: (id: string) => http.post<BatchVideoProjectDetail>(`/batch-video/projects/${encodeURIComponent(id)}/analysis/continue`, {}),
    cancelAnalysis: (id: string) => http.post<BatchVideoProjectDetail>(`/batch-video/projects/${encodeURIComponent(id)}/analysis/cancel`, {}),
    cancelPlanning: (id: string) => http.post<BatchVideoProjectDetail>(`/batch-video/projects/${encodeURIComponent(id)}/planning/cancel`, {}),
    generateCreatives: (id: string, payload: { count: number; mode?: string; templates?: string[] }) =>
        http.post<{ round: BatchVideoCreativeRound; rounds: number; roundIds: string[]; created: number; mode: string; matchCount: number }>(`/batch-video/projects/${encodeURIComponent(id)}/creatives/generate`, payload),
    listCreatives: (id: string, params: { page?: number; pageSize?: number; roundId?: string; reviewStatus?: string; matchStatus?: string } = {}) =>
        http.get<{ creatives: BatchVideoCreative[]; total: number }>(`/batch-video/projects/${encodeURIComponent(id)}/creatives`, { params }),
    updateCreative: (id: string, creativeId: string, payload: Pick<BatchVideoCreative, "title" | "hook" | "copywriting" | "videoPrompt" | "imagePrompt">) =>
        http.patch<BatchVideoCreative>(`/batch-video/projects/${encodeURIComponent(id)}/creatives/${encodeURIComponent(creativeId)}`, payload),
    rematchCreative: (id: string, creativeId: string) =>
        http.post<BatchVideoCreative>(`/batch-video/projects/${encodeURIComponent(id)}/creatives/${encodeURIComponent(creativeId)}/rematch`, {}),
    reviewCreative: (id: string, creativeId: string, status: "approved" | "rejected", note = "") =>
        http.post<BatchVideoCreative>(`/batch-video/projects/${encodeURIComponent(id)}/creatives/${encodeURIComponent(creativeId)}/review`, { status, note }),
    reviewCreatives: (id: string, ids: string[], status: "approved" | "rejected", note = "") =>
        http.post<{ updated: number }>(`/batch-video/projects/${encodeURIComponent(id)}/creatives/review-batch`, { ids, status, note }),
    listReferenceAssets: (id: string) => http.get<BatchVideoReferenceAsset[]>(`/batch-video/projects/${encodeURIComponent(id)}/reference-assets`),
    addReferenceAssets: (id: string, assets: { resourceId: string; title: string; description: string; tags: string[] }[]) =>
        http.post<BatchVideoReferenceAsset[]>(`/batch-video/projects/${encodeURIComponent(id)}/reference-assets`, { assets }),
    validation: (id: string) => http.get<BatchVideoValidationState>(`/batch-video/projects/${encodeURIComponent(id)}/validation`),
    unlockMass: (id: string) => http.post<BatchVideoProjectDetail>(`/batch-video/projects/${encodeURIComponent(id)}/unlock-mass`, {}),
    preflight: (id: string, payload: BatchVideoSubmissionInput) =>
        http.post<BatchVideoSubmissionPreflight>(`/batch-video/projects/${encodeURIComponent(id)}/preflight`, payload),
    discardUploads: (resourceIds: string[]) =>
        http.post<{ discarded: number }>("/batch-video/uploads/discard", { resourceIds }),
    submit: (id: string, payload: BatchVideoSubmissionInput) =>
        http.post<{ jobs: number }>(`/batch-video/projects/${encodeURIComponent(id)}/submit`, payload),
    start: (id: string, scheduledAt?: string) =>
        http.post<{ jobs: number }>(`/batch-video/projects/${encodeURIComponent(id)}/start`, scheduledAt ? { scheduledAt } : {}),
    listJobs: (id: string, params: { page?: number; pageSize?: number; status?: string } = {}) =>
        http.get<BatchVideoGenerationJob[]>(`/batch-video/projects/${encodeURIComponent(id)}/jobs`, { params }),
    listAttempts: (id: string, jobId: string) =>
        http.get<BatchVideoAttempt[]>(`/batch-video/projects/${encodeURIComponent(id)}/jobs/${encodeURIComponent(jobId)}/attempts`),
    retryJob: (id: string, jobId: string) => http.post<BatchVideoGenerationJob>(`/batch-video/projects/${encodeURIComponent(id)}/jobs/${encodeURIComponent(jobId)}/retry`, {}),
    cancelPending: (id: string) => http.post<{ cancelled: number }>(`/batch-video/projects/${encodeURIComponent(id)}/cancel-pending`, {}),
    retryFailed: (id: string) => http.post<{ retried: number }>(`/batch-video/projects/${encodeURIComponent(id)}/retry-failed`, {}),
    pause: (id: string) => http.post<{ status: string }>(`/batch-video/projects/${encodeURIComponent(id)}/pause`, {}),
    resume: (id: string) => http.post<{ status: string }>(`/batch-video/projects/${encodeURIComponent(id)}/resume`, {}),
    stats: (id: string) => http.get<BatchVideoProjectCounts>(`/batch-video/projects/${encodeURIComponent(id)}/stats`),
    settings: () => http.get<BatchVideoAdminSettings>("/batch-video/settings"),
    listModels: () => http.get<BatchVideoModelOption[]>("/batch-video/models"),
    generateReference: (id: string, creativeId: string) =>
        http.post<BatchVideoCreative>(`/batch-video/projects/${encodeURIComponent(id)}/creatives/${encodeURIComponent(creativeId)}/generate-reference`, {}),
    adminSettings: () => http.get<BatchVideoAdminSettings>("/admin/batch-video/settings"),
    updateAdminSettings: (settings: BatchVideoAdminSettings) => http.put<BatchVideoAdminSettings>("/admin/batch-video/settings", settings),
};

export type BatchVideoUploadPurpose = "material_image" | "material_video" | "material_document";

export async function uploadBatchVideoFile(file: File, purpose: BatchVideoUploadPurpose, kind: string, width = 0, height = 0): Promise<{ id: string; kind: string; provider: string }> {
    const form = new FormData();
    form.append("file", file);
    form.append("purpose", purpose);
    form.append("kind", kind);
    if (width > 0) form.append("width", String(width));
    if (height > 0) form.append("height", String(height));
    return http.post<{ id: string; kind: string; provider: string }>("/batch-video/uploads", form);
}

export function parseBatchVideoSettings(project: BatchVideoProject): BatchVideoProjectSettings {
    try {
        const value = JSON.parse(project.settingsJson || "{}") as Partial<BatchVideoProjectSettings>;
        return {
            durationSeconds: value.durationSeconds || 8,
            aspectRatio: value.aspectRatio || "9:16",
            standardCreativeCount: value.standardCreativeCount || 10,
            templates: value.templates,
        };
    } catch {
        return { durationSeconds: 8, aspectRatio: "9:16", standardCreativeCount: 10 };
    }
}

export function parseBatchVideoProfile(project: BatchVideoProject): { productSummary?: string; complianceNotes?: string[] } {
    try {
        return JSON.parse(project.productProfileJson || "{}");
    } catch {
        return {};
    }
}

export function batchVideoStageLabel(stage: string): string {
    switch (stage) {
        case "draft":
            return "资料准备";
        case "analyzing":
            return "分析中";
        case "standard_planning":
            return "标准策划中";
        case "standard_reviewing":
            return "标准审核中";
        case "standard_validating":
            return "标准验收中";
        case "mass_planning":
            return "量产策划中";
        case "mass_reviewing":
            return "量产审核中";
        case "mass_generating":
            return "量产生成中";
        case "completed":
            return "已完成";
        default:
            return stage || "资料准备";
    }
}
