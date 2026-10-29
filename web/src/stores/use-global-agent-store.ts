import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { scopedLocalStorage } from "@/lib/user-scope";
import type { CanvasResourceReference } from "@/lib/canvas/canvas-resource-references";
import type { CloudAgentMessagePresentation } from "@/services/cloud-agent-conversations";
import type { CanvasNodeData } from "@/types/canvas";

export type AgentPrefillRequest = CloudAgentMessagePresentation & { id: number; text: string; autoSubmit?: boolean; requiresVision?: boolean };

/**
 * 当前页面提供给平台级 Agent 的画布上下文。只有画布编辑器会发布它；
 * 离开画布时清空，此后的对话轮次不再绑定该画布。
 */
export type AgentCanvasContext = {
    canvasId: string;
    domainProjectId?: string;
    nodeCount: number;
    selectedNodeIds: string[];
    references: CanvasResourceReference[];
    canvasNodes?: readonly CanvasNodeData[];
    runningNodeId?: string | null;
    prefillRequest?: AgentPrefillRequest | null;
    onFocusNode?: (nodeId: string) => void;
};

type GlobalAgentStore = {
    /** 平台唯一 Agent 面板的开合状态，跨路由保持。 */
    open: boolean;
    /** 本次会话是否需要挂载面板；关闭悬浮入口后，画布内的 Agent 入口仍可临时拉起。 */
    mounted: boolean;
    /** 悬浮入口开关，默认关闭：不挂载面板，也就不加载助手形象与事件流。 */
    launcherEnabled: boolean;
    canvas: AgentCanvasContext | null;
    openAgent: () => void;
    closeAgent: () => void;
    setLauncherEnabled: (enabled: boolean) => void;
    setCanvas: (canvas: AgentCanvasContext | null) => void;
    reset: () => void;
};

export const AGENT_LAUNCHER_STORE_KEY = "open_ai_canvas:agent_launcher";

export const useGlobalAgentStore = create<GlobalAgentStore>()(
    persist(
        (set) => ({
            open: false,
            mounted: false,
            launcherEnabled: false,
            canvas: null,
            openAgent: () => set({ open: true, mounted: true }),
            closeAgent: () => set({ open: false }),
            setLauncherEnabled: (enabled) => set((state) => ({ launcherEnabled: enabled, mounted: state.mounted || enabled })),
            setCanvas: (canvas) => set({ canvas }),
            reset: () => set({ open: false, mounted: false, canvas: null }),
        }),
        {
            name: AGENT_LAUNCHER_STORE_KEY,
            storage: createJSONStorage(() => scopedLocalStorage),
            partialize: (state) => ({ launcherEnabled: state.launcherEnabled }),
            merge: (persisted, current) => {
                const stored = (persisted || {}) as Partial<GlobalAgentStore>;
                const launcherEnabled = stored.launcherEnabled === true;
                return { ...current, launcherEnabled, mounted: current.mounted || launcherEnabled };
            },
        },
    ),
);
