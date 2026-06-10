"use client";

import { useState } from "react";
import { StatusBadge } from "@/components/agent/StatusBadge";
import { MarkdownContent } from "@/components/ui/markdown";
import type { TaskStep } from "@/types/agent";
import { ChevronDownIcon, ChevronRightIcon, WrenchIcon } from "lucide-react";

interface Props {
  steps: TaskStep[];
}

function StepItem({ step, index }: { step: TaskStep; index: number }) {
  const [expanded, setExpanded] = useState(step.Status === "completed" || step.Status === "failed");
  const hasOutput = Boolean(step.Output?.trim());

  return (
    <li className="relative pl-10">
      <span className="absolute left-0 flex h-7 w-7 items-center justify-center rounded-full border bg-background text-xs font-semibold z-10">
        {index + 1}
      </span>

      <div className="border rounded-lg overflow-hidden">
        <div
          className={`flex items-start gap-3 px-4 py-3 ${hasOutput ? "cursor-pointer hover:bg-muted/30 transition-colors" : ""}`}
          onClick={() => hasOutput && setExpanded((v) => !v)}
        >
          <div className="flex-1 min-w-0">
            <p className="text-sm font-medium">{step.Description}</p>
            {step.ToolUsed && (
              <div className="flex items-center gap-1.5 mt-1">
                <WrenchIcon className="w-3 h-3 text-muted-foreground" />
                <span className="text-xs text-muted-foreground font-mono">{step.ToolUsed}</span>
              </div>
            )}
          </div>

          <div className="flex items-center gap-2 shrink-0">
            <StatusBadge status={step.Status} size="sm" />
            {hasOutput && (
              expanded ? (
                <ChevronDownIcon className="w-4 h-4 text-muted-foreground" />
              ) : (
                <ChevronRightIcon className="w-4 h-4 text-muted-foreground" />
              )
            )}
          </div>
        </div>

        {hasOutput && expanded && (
          <div className="border-t bg-muted/20 px-4 py-3">
            <MarkdownContent content={step.Output!} compact />
          </div>
        )}
      </div>
    </li>
  );
}

export function StepTimeline({ steps }: Props) {
  if (steps.length === 0) {
    return (
      <p className="text-muted-foreground text-sm py-4 text-center">No steps recorded yet.</p>
    );
  }

  return (
    <ol className="space-y-3 relative before:absolute before:left-3.5 before:top-7 before:bottom-7 before:w-px before:bg-border">
      {steps.map((step, i) => (
        <StepItem key={step.ID} step={step} index={i} />
      ))}
    </ol>
  );
}
