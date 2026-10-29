import { Check, Loader2 } from "lucide-react";

import { cn } from "@/lib/utils";
import type { BatchVideoWorkflowStep, BatchVideoWorkflowStepKey } from "../shared";

export function WorkflowNav({
    steps,
    activeKey,
    onSelect,
}: {
    steps: BatchVideoWorkflowStep[];
    activeKey: BatchVideoWorkflowStepKey;
    onSelect: (key: BatchVideoWorkflowStepKey) => void;
}) {
    const doneCount = steps.filter((step) => step.done).length;
    return (
        <div className="space-y-2">
            <div className="flex items-center justify-between px-1">
                <span className="text-sm font-semibold">项目流程</span>
                <span className="text-[11px] tabular-nums text-foreground/45">{doneCount}/{steps.length} 完成</span>
            </div>
            <nav className="flex gap-2 overflow-x-auto pb-1 lg:flex-col lg:overflow-visible lg:pb-0" aria-label="项目流程">
                {steps.map((step, index) => {
                    const selected = step.key === activeKey;
                    return (
                        <button
                            key={step.key}
                            type="button"
                            aria-current={selected ? "step" : undefined}
                            onClick={() => onSelect(step.key)}
                            className={cn(
                                "flex min-w-[190px] shrink-0 items-start gap-3 rounded-xl border px-3 py-3 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring lg:w-full lg:min-w-0",
                                selected ? "border-primary/40 bg-primary/10" : "border-border/60 bg-surface hover:border-border hover:bg-surface-active/40",
                            )}
                        >
                            <span
                                className={cn(
                                    "mt-0.5 grid size-7 shrink-0 place-items-center rounded-full border text-[11px] font-semibold tabular-nums",
                                    step.done
                                        ? "border-emerald-500/40 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400"
                                        : selected || step.inProgress
                                          ? "border-primary/50 bg-primary/10 text-primary"
                                          : "border-border/70 text-foreground/35",
                                )}
                            >
                                {step.done ? <Check className="size-4" /> : step.inProgress ? <Loader2 className="size-3.5 animate-spin" /> : index + 1}
                            </span>
                            <span className="min-w-0 flex-1">
                                <span className="flex items-center gap-2">
                                    <span className="truncate text-sm font-medium">{step.title}</span>
                                    {step.inProgress ? <span className="shrink-0 text-[10px] text-primary">进行中</span> : selected ? <span className="shrink-0 text-[10px] text-primary">当前</span> : null}
                                </span>
                                <span className="mt-0.5 block text-[11px] leading-4 text-foreground/50">{step.description}</span>
                            </span>
                        </button>
                    );
                })}
            </nav>
        </div>
    );
}
