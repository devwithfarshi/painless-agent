"use client";

import Link from "next/link";
import { Card, CardContent } from "@/components/ui/card";
import { StatusBadge } from "@/components/agent/StatusBadge";
import type { Task } from "@/types/agent";
import { ClockIcon } from "lucide-react";

function formatDuration(createdAt: string, updatedAt: string): string {
  const ms = new Date(updatedAt).getTime() - new Date(createdAt).getTime();
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60000) return `${(ms / 1000).toFixed(0)}s`;
  return `${Math.floor(ms / 60000)}m ${Math.floor((ms % 60000) / 1000)}s`;
}

interface Props {
  task: Task;
}

const statusBorder: Record<string, string> = {
  pending: "border-l-amber-400",
  running: "border-l-blue-500",
  completed: "border-l-emerald-500",
  failed: "border-l-red-500",
  cancelled: "border-l-slate-400",
};

export function TaskCard({ task }: Props) {
  return (
    <Link href={`/tasks/${task.ID}`} className="block group">
      <Card
        className={`border-l-2 hover:border-l-primary transition-colors h-full ${
          statusBorder[task.Status] ?? "border-l-border"
        }`}
      >
        <CardContent className="p-4 space-y-2.5">
          <div className="flex items-start justify-between gap-2">
            <p className="font-medium text-sm leading-snug line-clamp-2 flex-1 group-hover:text-primary transition-colors">
              {task.Goal}
            </p>
            <StatusBadge status={task.Status} size="sm" />
          </div>
          <div className="flex items-center gap-3 text-xs text-muted-foreground">
            <span className="font-mono truncate max-w-[140px]">{task.ID.slice(0, 8)}…</span>
            <span className="flex items-center gap-1 ml-auto shrink-0">
              <ClockIcon className="w-3 h-3" />
              {formatDuration(task.CreatedAt, task.UpdatedAt)}
            </span>
          </div>
        </CardContent>
      </Card>
    </Link>
  );
}
