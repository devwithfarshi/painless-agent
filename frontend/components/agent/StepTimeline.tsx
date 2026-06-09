"use client";

import { Badge } from "@/components/ui/badge";
import type { TaskStep } from "@/types/agent";

const statusColor: Record<string, string> = {
  pending: "secondary",
  running: "default",
  completed: "outline",
  failed: "destructive",
};

interface Props {
  steps: TaskStep[];
}

export function StepTimeline({ steps }: Props) {
  if (steps.length === 0) {
    return <p className="text-muted-foreground text-sm">No steps yet.</p>;
  }
  return (
    <ol className="relative border-l border-muted ml-3 space-y-4">
      {steps.map((step, i) => (
        <li key={step.ID} className="ml-6">
          <span className="absolute -left-3 flex h-6 w-6 items-center justify-center rounded-full bg-muted border text-xs font-bold">
            {i + 1}
          </span>
          <div className="flex items-start gap-2">
            <div className="flex-1">
              <p className="text-sm font-medium">{step.Description}</p>
              {step.ToolUsed && (
                <p className="text-xs text-muted-foreground">tool: {step.ToolUsed}</p>
              )}
              {step.Output && (
                <pre className="mt-1 text-xs bg-muted/50 rounded p-2 overflow-x-auto whitespace-pre-wrap max-h-40">
                  {step.Output}
                </pre>
              )}
            </div>
            <Badge variant={statusColor[step.Status] as "default"}>{step.Status}</Badge>
          </div>
        </li>
      ))}
    </ol>
  );
}
