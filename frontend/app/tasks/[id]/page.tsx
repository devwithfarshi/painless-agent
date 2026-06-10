"use client";

import { useEffect, useState } from "react";
import { useParams } from "next/navigation";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { AgentFeed } from "@/components/agent/AgentFeed";
import { StepTimeline } from "@/components/agent/StepTimeline";
import { TaskFiles } from "@/components/agent/TaskFiles";
import { StatusBadge } from "@/components/agent/StatusBadge";
import { api } from "@/lib/api";
import type { Task, TaskStep } from "@/types/agent";
import { AlertTriangleIcon, ClockIcon, CalendarIcon } from "lucide-react";

function formatDuration(createdAt: string, updatedAt: string): string {
  const ms = new Date(updatedAt).getTime() - new Date(createdAt).getTime();
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60000) return `${(ms / 1000).toFixed(0)}s`;
  return `${Math.floor(ms / 60000)}m ${Math.floor((ms % 60000) / 1000)}s`;
}

export default function TaskDetailPage() {
  const { id } = useParams<{ id: string }>();
  const [task, setTask] = useState<Task | null>(null);
  const [steps, setSteps] = useState<TaskStep[]>([]);
  const [fetchError, setFetchError] = useState<string | null>(null);
  const [streamError, setStreamError] = useState<string | null>(null);

  const fetchTask = async () => {
    try {
      const data = await api.tasks.get(id);
      setTask(data.task);
      setSteps(data.steps);
    } catch (e) {
      setFetchError(String(e));
    }
  };

  useEffect(() => {
    fetchTask();
    const t = setInterval(fetchTask, 3000);
    return () => clearInterval(t);
  }, [id]);

  if (fetchError) {
    return (
      <div className="rounded-lg border border-destructive/40 bg-destructive/5 p-4 text-sm text-destructive">
        {fetchError}
      </div>
    );
  }

  if (!task) {
    return (
      <div className="space-y-3 animate-pulse">
        <div className="h-7 w-2/3 rounded bg-muted" />
        <div className="h-4 w-1/3 rounded bg-muted" />
        <div className="h-40 rounded bg-muted mt-6" />
      </div>
    );
  }

  const errorMsg = task.ErrorMsg || (task.Status === "failed" ? streamError : null);

  return (
    <div className="space-y-6 max-w-4xl">
      <div>
        <div className="flex items-start gap-3">
          <div className="flex-1 min-w-0">
            <h1 className="text-xl font-semibold leading-snug">{task.Goal}</h1>
            <div className="flex flex-wrap items-center gap-3 mt-2 text-xs text-muted-foreground">
              <span className="font-mono">{task.ID}</span>
              <span className="flex items-center gap-1">
                <CalendarIcon className="w-3 h-3" />
                {new Date(task.CreatedAt).toLocaleString()}
              </span>
              <span className="flex items-center gap-1">
                <ClockIcon className="w-3 h-3" />
                {formatDuration(task.CreatedAt, task.UpdatedAt)}
              </span>
            </div>
          </div>
          <StatusBadge status={task.Status} />
        </div>
      </div>

      {task.Status === "failed" && errorMsg && (
        <div className="rounded-lg border border-destructive/40 bg-destructive/5 p-4">
          <div className="flex items-start gap-2.5">
            <AlertTriangleIcon className="w-4 h-4 text-destructive shrink-0 mt-0.5" />
            <div>
              <p className="text-sm font-semibold text-destructive mb-1">Task failed</p>
              <p className="font-mono text-xs text-destructive/80 break-words">{errorMsg}</p>
            </div>
          </div>
        </div>
      )}

      <Tabs defaultValue="feed">
        <TabsList className="w-full justify-start">
          <TabsTrigger value="feed">Live Feed</TabsTrigger>
          <TabsTrigger value="steps">Steps ({steps.length})</TabsTrigger>
          <TabsTrigger value="files">Files</TabsTrigger>
        </TabsList>
        <TabsContent value="feed" className="mt-4">
          <AgentFeed taskId={id} onError={setStreamError} />
        </TabsContent>
        <TabsContent value="steps" className="mt-4">
          <StepTimeline steps={steps} />
        </TabsContent>
        <TabsContent value="files" className="mt-4">
          <TaskFiles taskId={id} />
        </TabsContent>
      </Tabs>
    </div>
  );
}
