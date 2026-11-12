import { expect, test } from "bun:test";

import { autoRecoverCreationMessages } from "../src/pages/create/creation-conversations";
import type { CreationConversation, CreationMessage } from "../src/pages/create/creation-types";
import type { GenerationTask } from "../src/services/api/task-center";

function message(id: string, role: CreationMessage["role"], extra: Partial<CreationMessage> = {}): CreationMessage {
    return { id, role, content: id, createdAt: "2026-10-12T00:00:00.000Z", ...extra };
}

function conversation(messages: CreationMessage[]): CreationConversation {
    return { id: "c1", title: "会话", createdAt: "2026-10-12T00:00:00.000Z", updatedAt: "2026-10-12T00:00:00.000Z", messages };
}

function task(id: string, extra: Partial<GenerationTask> = {}): GenerationTask {
    return {
        id,
        type: "video",
        status: "succeeded",
        prompt: "p",
        attempts: 1,
        createdAt: "2026-10-12T00:00:00.000Z",
        updatedAt: "2026-10-12T00:00:00.000Z",
        ...extra,
    } as GenerationTask;
}

test("生成中的消息在后端已成功时自动补回结果", () => {
    const messages = [message("u1", "user", { mode: "video" }), message("a1", "assistant", { mode: "video", status: "pending", taskIds: ["t1"] })];
    const recovered = autoRecoverCreationMessages([conversation(messages)], [task("t1", { resultJson: JSON.stringify({ video: { url: "https://example.com/v.mp4" } }) })]);
    expect(recovered.recoveredCount).toBe(1);
    const updated = recovered.conversations[0].messages[1];
    expect(updated.status).toBe("done");
    expect(updated.resultUrls).toEqual(["https://example.com/v.mp4"]);
});

test("失败的消息在后端已成功时也恢复，并清掉失败原因", () => {
    const messages = [message("u1", "user", { mode: "image" }), message("a1", "assistant", { mode: "image", status: "error", error: "旧失败", errorKind: "task", taskIds: ["t2"] })];
    const recovered = autoRecoverCreationMessages([conversation(messages)], [task("t2", { type: "image", resultJson: JSON.stringify({ images: [{ url: "https://example.com/i.png" }] }) })]);
    expect(recovered.recoveredCount).toBe(1);
    const updated = recovered.conversations[0].messages[1];
    expect(updated.status).toBe("done");
    expect(updated.error).toBeUndefined();
    expect(updated.errorKind).toBeUndefined();
});

test("通过 retryOf 与 attemptGroupId 也能匹配到任务", () => {
    const messages = [message("u1", "user", { mode: "video" }), message("a1", "assistant", { mode: "video", status: "pending", taskIds: ["orig"] })];
    const recovered = autoRecoverCreationMessages([conversation(messages)], [task("retry-1", { retryOf: "orig", resultJson: JSON.stringify({ video: { storageKey: "k1" } }) })]);
    expect(recovered.recoveredCount).toBe(1);
    expect(recovered.conversations[0].messages[1].resultStorageKeys).toEqual(["k1"]);
});

test("后端任务没有结果时不误判为已完成", () => {
    const messages = [message("u1", "user", { mode: "video" }), message("a1", "assistant", { mode: "video", status: "pending", taskIds: ["t3"] })];
    const recovered = autoRecoverCreationMessages([conversation(messages)], [task("t3")]);
    expect(recovered.recoveredCount).toBe(0);
    expect(recovered.conversations[0].messages[1].status).toBe("pending");
});

test("没有成功的任务时不做任何改动", () => {
    const messages = [message("u1", "user"), message("a1", "assistant", { status: "pending", taskIds: ["t4"] })];
    const recovered = autoRecoverCreationMessages([conversation(messages)], [task("t4", { status: "failed" })]);
    expect(recovered.recoveredCount).toBe(0);
});

test("已完成的消息不会被重复处理", () => {
    const messages = [message("u1", "user", { mode: "video" }), message("a1", "assistant", { mode: "video", status: "done", resultUrls: ["https://example.com/old.mp4"], taskIds: ["t5"] })];
    const recovered = autoRecoverCreationMessages([conversation(messages)], [task("t5", { resultJson: JSON.stringify({ video: { url: "https://example.com/new.mp4" } }) })]);
    expect(recovered.recoveredCount).toBe(0);
    expect(recovered.conversations[0].messages[1].resultUrls).toEqual(["https://example.com/old.mp4"]);
});
