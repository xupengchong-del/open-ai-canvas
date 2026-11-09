import { translate } from "@/i18n";
import { createClientId } from "@/lib/client-id";
import { generationErrorMessage } from "@/lib/generation-error";
import type { BackendGenerationResult } from "@/services/api/generation-task";
import type { GenerationTask } from "@/services/api/task-center";
import { creationAttachmentKind, type CreationAttachment } from "./creation-assets";
import type { CreationConversation, CreationMessage, CreationShotRailEntry } from "./creation-types";

type CreationRuntime = typeof import("./creation-runtime");
export type PersistedCreationTask = GenerationTask & { creationResultUrls?: string[]; creationResultStorageKeys?: string[]; creationError?: string };

export function newConversation(): CreationConversation {
    return { id: createClientId(), title: translate("create.conversation.untitled"), updatedAt: new Date().toISOString(), messages: [] };
}

export function newMessage(role: CreationMessage["role"], content: string, extra: Partial<CreationMessage> = {}): CreationMessage {
    return { id: createClientId(), role, content, createdAt: new Date().toISOString(), ...extra };
}

export function creationShotRail(messages: CreationMessage[]): CreationShotRailEntry[] {
    const rail: CreationShotRailEntry[] = [];
    let ordinal = 0;
    let lastIndex = -1;
    for (const message of messages) {
        const isVideo = (message.mode || "text") === "video";
        if (message.role === "user") {
            if (!isVideo) continue;
            ordinal += 1;
            rail.push({ key: message.id, ordinal, user: message });
            lastIndex = rail.length - 1;
        } else if (isVideo && lastIndex >= 0 && !rail[lastIndex].result) {
            rail[lastIndex].result = message;
        }
    }
    return rail;
}

export function creationVideoShotOrdinal(shots: CreationShotRailEntry[], item: CreationMessage): number {
    const own = shots.find((entry) => item.role === "user" ? entry.user.id === item.id : entry.result?.id === item.id);
    return own?.ordinal || 0;
}

export function completedCreationGenerationTask(runtime: CreationRuntime, input: { taskId: string; task?: GenerationTask; mode: "image" | "video"; prompt: string; result: BackendGenerationResult; conversationId: string; messageId: string; batchIndex?: number; batchCount?: number }): GenerationTask {
    const now = new Date().toISOString();
    const task = input.task ?? { id: input.taskId, type: input.mode, status: "succeeded" as const, prompt: input.prompt, attempts: 1, createdAt: now, updatedAt: now };
    return runtime.projectGenerationTaskResult({ ...task, status: "succeeded", prompt: input.prompt, clientContext: { conversationId: input.conversationId, messageId: input.messageId, ...(typeof input.batchIndex === "number" ? { batchIndex: input.batchIndex } : {}), ...(typeof input.batchCount === "number" ? { batchCount: input.batchCount } : {}) } }, input.result);
}

export function isVideoAttachment(attachment: CreationAttachment): attachment is CreationAttachment & { url: string } {
    return creationAttachmentKind(attachment) === "video";
}

export function isImageAttachment(attachment: CreationAttachment): attachment is CreationAttachment & { dataUrl: string; width?: number; height?: number } {
    return creationAttachmentKind(attachment) === "image";
}


export function attachCreationTaskContexts(tasks: GenerationTask[], conversations: CreationConversation[]) {
    const contexts = new Map<string, { prompt: string; clientContext: NonNullable<GenerationTask["clientContext"]> }>();
    for (const conversation of conversations) {
        for (const [messageIndex, message] of conversation.messages.entries()) {
            if (message.role !== "assistant" || !message.taskIds?.length) continue;
            const prompt = conversation.messages[messageIndex - 1]?.role === "user" ? conversation.messages[messageIndex - 1].content : "";
            for (const [batchIndex, taskId] of message.taskIds.entries()) contexts.set(taskId, { prompt, clientContext: { conversationId: conversation.id, messageId: message.id, batchIndex, batchCount: message.taskIds.length } });
        }
    }
    return tasks.map((task) => {
        const context = contexts.get(task.id);
        return context ? { ...task, prompt: context.prompt, clientContext: context.clientContext } : task;
    });
}

export async function materializeCreationTaskResults(runtime: CreationRuntime, tasks: GenerationTask[], signal?: AbortSignal): Promise<PersistedCreationTask[]> {
    return Promise.all(tasks.map(async (task): Promise<PersistedCreationTask> => {
        // 文本正文保存在 resultJson，不进入媒体资源化链路。
        if (task.status !== "succeeded" || !task.clientContext || task.type === "canvas_text") return task;
        try {
            const materialized = await runtime.runGenerationConsumer(signal, (managedSignal: AbortSignal) => runtime.materializeGenerationTaskAssets(task, managedSignal));
            const creationResultUrls = runtime.generationTaskMaterializedUrls(materialized);
            const creationResultStorageKeys = runtime.generationTaskMaterializedStorageKeys(materialized);
            return creationResultUrls.length || creationResultStorageKeys.length ? { ...materialized, ...(creationResultUrls.length ? { creationResultUrls } : {}), ...(creationResultStorageKeys.length ? { creationResultStorageKeys } : {}) } : materialized;
        } catch (error) {
            return { ...task, creationError: error instanceof Error ? error.message : translate("create.conversation.materializeFailed") };
        }
    }));
}

/** 失败但后端没有确认任务失败（含旧版本未分类的失败）：作品可能已生成，应提供“重新获取结果”。 */
export function creationMessageNeedsResultSync(message: Pick<CreationMessage, "status" | "errorKind" | "taskIds">) {
    return message.status === "error" && message.errorKind !== "task" && Boolean(message.taskIds?.length);
}

// 仍需用任务终态来收敛的媒体消息：生成中，或失败但后端没有确认失败。
function creationMediaMessageAwaitsTask(message: CreationMessage, revive: boolean) {
    if (message.role !== "assistant") return false;
    return revive || message.status === "pending" || creationMessageNeedsResultSync(message);
}

const clearedCreationSyncState = { syncInterrupted: undefined, errorKind: undefined, mediaRecoverable: undefined } satisfies Partial<CreationMessage>;

/** 任务仍在进行：消息回到生成中，并刷新阶段与真实进度；没有变化时返回原对象，避免无谓重渲染与持久化。 */
function creationMessageWithObservedTask(message: CreationMessage, task: GenerationTask): CreationMessage {
    const progress = typeof task.progress === "number" && Number.isFinite(task.progress) ? Math.round(task.progress) : undefined;
    const unchanged =
        message.status === "pending" &&
        !message.syncInterrupted &&
        !message.error &&
        !message.errorKind &&
        message.generationStatus === task.status &&
        message.generationStage === task.stage &&
        message.generationMediaStage === task.mediaStage &&
        message.generationProgress === progress;
    if (unchanged) return message;
    return {
        ...message,
        ...clearedCreationSyncState,
        status: "pending",
        content: message.status === "pending" ? message.content : "",
        error: undefined,
        generationStatus: task.status,
        generationStage: task.stage,
        generationMediaStage: task.mediaStage,
        generationProgress: progress,
    };
}

export function reconcileCreationTaskMessages(runtime: CreationRuntime, conversations: CreationConversation[], tasks: PersistedCreationTask[], options: { reviveMessageIds?: ReadonlySet<string> } = {}) {
    let changed = false;
    const next = conversations.map((conversation) => {
        let conversationChanged = false;
        let completedAt = conversation.updatedAt;
        const messages = conversation.messages.map((message) => {
            const taskIds = new Set(message.taskIds || []);
            const matches = tasks
                .filter((task) => taskIds.has(task.id) || (task.clientContext?.conversationId === conversation.id && task.clientContext.messageId === message.id))
                .sort((left, right) => (left.clientContext?.batchIndex || 0) - (right.clientContext?.batchIndex || 0));
            const active = matches.filter((task) => task.status === "queued" || task.status === "running");
            if (message.role === "assistant" && message.mode === "text") {
                const recovery = runtime.recoverCreationTextTask(message, matches);
                if (!recovery) {
                    // 恢复监听已重新拿到任务状态，连接中断提示可以收起；正文仍等任务终态一次性回填。
                    if (!message.syncInterrupted || !active.length) return message;
                    conversationChanged = true;
                    changed = true;
                    return { ...message, syncInterrupted: undefined };
                }
                completedAt = matches.reduce((latest, task) => conversationTimestamp(task.updatedAt) > conversationTimestamp(latest) ? task.updatedAt : latest, completedAt);
                conversationChanged = true;
                changed = true;
                return { ...message, ...recovery, syncInterrupted: undefined };
            }
            if (!creationMediaMessageAwaitsTask(message, Boolean(options.reviveMessageIds?.has(message.id)))) return message;
            const expectedTaskCount = Math.max(0, ...matches.map((task) => task.clientContext?.batchCount || 0));
            if (!matches.length || (expectedTaskCount > 0 && matches.length < expectedTaskCount)) return message;
            if (active.length) {
                const observed = creationMessageWithObservedTask(message, active[0]);
                if (observed === message) return message;
                conversationChanged = true;
                changed = true;
                return observed;
            }

            const succeeded = matches.filter((task) => task.status === "succeeded");
            const resultUrls = Array.from(new Set(succeeded.flatMap(creationTaskResultUrls)));
            const resultStorageKeys = Array.from(new Set(succeeded.flatMap(creationTaskResultStorageKeys)));
            const failedCount = matches.filter((task) => task.status !== "succeeded" || Boolean(task.creationError)).length;
            const nextTaskIds = Array.from(new Set([...(message.taskIds || []), ...matches.map((task) => task.id)]));
            completedAt = matches.reduce((latest, task) => conversationTimestamp(task.updatedAt) > conversationTimestamp(latest) ? task.updatedAt : latest, completedAt);
            conversationChanged = true;
            changed = true;

            if (resultUrls.length || resultStorageKeys.length) {
                const content = message.mode === "video" ? translate("create.status.videoGenerated") : failedCount ? translate("create.status.imagesPartiallyGenerated", { done: resultStorageKeys.length || resultUrls.length, failed: failedCount }) : translate("create.status.imageGenerated");
                return { ...message, ...clearedCreationSyncState, status: "done" as const, content, ...(resultUrls.length ? { resultUrls } : {}), ...(resultStorageKeys.length ? { resultStorageKeys } : {}), error: undefined, generationStatus: "succeeded" as const, taskIds: nextTaskIds };
            }
            if (matches.every((task) => task.status === "cancelled")) {
                return { ...message, ...clearedCreationSyncState, status: "cancelled" as const, content: translate("create.status.stopped"), error: undefined, generationStatus: "cancelled" as const, taskIds: nextTaskIds };
            }
            // 任务成功但结果没能载入（资源化失败）不是生成失败：保留“重新获取结果”，不引导重新付费生成。
            const unavailable = succeeded.find((task) => task.creationError) || succeeded[0];
            const failed = matches.find((task) => task.status === "failed");
            if (unavailable) {
                return { ...message, ...clearedCreationSyncState, status: "error" as const, content: translate("create.status.failed"), errorKind: "sync" as const, error: generationErrorMessage(unavailable.creationError || translate("create.conversation.resultUnreadable")), generationStatus: "succeeded" as const, taskIds: nextTaskIds };
            }
            return {
                ...message,
                ...clearedCreationSyncState,
                status: "error" as const,
                content: translate("create.status.failed"),
                errorKind: "task" as const,
                mediaRecoverable: matches.some((task) => task.status === "failed" && task.canRecoverMedia) || undefined,
                error: generationErrorMessage(failed?.error || translate("create.conversation.noResult")),
                generationStatus: failed ? ("failed" as const) : message.generationStatus,
                generationMediaStage: failed?.mediaStage,
                taskIds: nextTaskIds,
            };
        });
        return conversationChanged ? { ...conversation, messages, updatedAt: completedAt } : conversation;
    });
    return changed ? next : conversations;
}

function creationTaskResultUrls(task: PersistedCreationTask) {
    if (task.creationResultUrls?.length) return task.creationResultUrls;
    return [];
}

function creationTaskResultStorageKeys(task: PersistedCreationTask) {
    if (task.creationResultStorageKeys?.length) return task.creationResultStorageKeys;
    return [];
}

export function conversationTimestamp(value: string) {
    const timestamp = new Date(value).getTime();
    return Number.isFinite(timestamp) ? timestamp : 0;
}

/**
 * 一条「生成记录」= 一条用户提示 + 其后到下一次用户提示之前的全部结果消息。
 * 传入记录内任意一条消息 id；结果消息前没有用户提示的异常数据只算它自己。
 */
export function creationRecordRange(messages: CreationMessage[], messageId: string): { start: number; end: number } | null {
    const index = messages.findIndex((message) => message.id === messageId);
    if (index < 0) return null;
    let start = index;
    while (start > 0 && messages[start].role !== "user") start -= 1;
    if (messages[start].role !== "user") return { start: index, end: index + 1 };
    let end = start + 1;
    while (end < messages.length && messages[end].role !== "user") end += 1;
    return { start, end };
}

/** 从会话中移除一整条生成记录（提示词与它的结果）；素材库中的原素材不受影响。 */
export function removeCreationRecord(messages: CreationMessage[], messageId: string): { messages: CreationMessage[]; removed: CreationMessage[] } | null {
    const range = creationRecordRange(messages, messageId);
    if (!range) return null;
    return {
        messages: [...messages.slice(0, range.start), ...messages.slice(range.end)],
        removed: messages.slice(range.start, range.end),
    };
}

// 自动恢复要处理的是后端原始任务，可能只有 resultJson、没有本地投影出的 creationResult* 字段，
// 因此这里用比 reconcile 更宽松的解析，避免把「其实已成功」的任务判成没有结果。
function autoRecoverTaskResultUrls(task: GenerationTask): string[] {
    const fromProps = (task as PersistedCreationTask).creationResultUrls;
    if (fromProps?.length) return fromProps;
    if (task.previewUrl) return [task.previewUrl];
    if (task.resultJson) {
        try {
            const parsed = JSON.parse(task.resultJson);
            if (parsed.video?.url || parsed.video?.dataUrl) return [parsed.video.url || parsed.video.dataUrl];
            if (Array.isArray(parsed.images) && parsed.images.length) {
                const urls = parsed.images
                    .map((img: { url?: string; dataUrl?: string }) => img.url || img.dataUrl)
                    .filter(Boolean);
                if (urls.length) return urls;
            }
        } catch {
            // 结果体不是合法 JSON 时按「没有结果」处理，交给任务终态收敛。
        }
    }
    return [];
}

function autoRecoverTaskResultStorageKeys(task: GenerationTask): string[] {
    const fromProps = (task as PersistedCreationTask).creationResultStorageKeys;
    if (fromProps?.length) return fromProps;
    if (task.resultJson) {
        try {
            const parsed = JSON.parse(task.resultJson);
            if (parsed.video?.storageKey) return [parsed.video.storageKey];
            if (Array.isArray(parsed.images) && parsed.images.length) {
                const keys = parsed.images.map((img: { storageKey?: string }) => img.storageKey).filter(Boolean);
                if (keys.length) return keys;
            }
        } catch {
            // 同上：解析失败时不做恢复判断。
        }
    }
    return [];
}

/**
 * 页面水合阶段的深度恢复：会话里有「生成中」或「失败」的助手消息时，
 * 直接拿它们的任务 ID 与后端最近的已完成任务比对；后端其实已经出片就静默挂回结果，
 * 用户不需要重新排队生成，也不会被误判成失败。
 */
export function autoRecoverCreationMessages(
    conversations: CreationConversation[],
    recentTasks: GenerationTask[],
): { conversations: CreationConversation[]; recoveredCount: number } {
    const succeededTasks = recentTasks.filter((task) => task.status === "succeeded");
    if (!succeededTasks.length) return { conversations, recoveredCount: 0 };
    let recoveredCount = 0;

    const next = conversations.map((conversation) => {
        let conversationChanged = false;
        const messages = conversation.messages.map((message) => {
            if (message.role !== "assistant" || (message.status !== "pending" && message.status !== "error")) {
                return message;
            }
            const targetTaskIds = new Set(message.taskIds || []);
            const matchedTask = succeededTasks.find(
                (task) =>
                    targetTaskIds.has(task.id) ||
                    Boolean(task.retryOf && targetTaskIds.has(task.retryOf)) ||
                    Boolean(task.attemptGroupId && targetTaskIds.has(task.attemptGroupId)) ||
                    Boolean(task.clientContext?.messageId && task.clientContext.messageId === message.id),
            );
            if (!matchedTask) return message;

            const resultUrls = autoRecoverTaskResultUrls(matchedTask);
            const resultStorageKeys = autoRecoverTaskResultStorageKeys(matchedTask);
            if (!resultUrls.length && !resultStorageKeys.length) return message;

            conversationChanged = true;
            recoveredCount += 1;
            return {
                ...message,
                status: "done" as const,
                content: message.mode === "video" ? "视频已生成" : "图片已生成",
                error: undefined,
                errorKind: undefined,
                syncInterrupted: undefined,
                resultUrls,
                resultStorageKeys,
                taskIds: Array.from(new Set([...(message.taskIds || []), matchedTask.id])),
            };
        });
        return conversationChanged ? { ...conversation, messages, updatedAt: new Date().toISOString() } : conversation;
    });

    return { conversations: recoveredCount > 0 ? next : conversations, recoveredCount };
}
