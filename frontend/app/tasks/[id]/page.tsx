"use client";

import { useEffect, useState } from "react";
import { useParams } from "next/navigation";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Badge } from "@/components/ui/badge";
import { AgentFeed } from "@/components/agent/AgentFeed";
import { StepTimeline } from "@/components/agent/StepTimeline";
import { WorkspaceFiles } from "@/components/agent/WorkspaceFiles";
import { api } from "@/lib/api";
import type { Task, TaskStep } from "@/types/agent";

const statusColor: Record<string, string> = {
  pending: "secondary",
  running: "default",
  completed: "outline",
  failed: "destructive",
  cancelled: "secondary",
};

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

  if (fetchError) return <p className="text-destructive">{fetchError}</p>;
  if (!task) return <p className="text-muted-foreground">Loading…</p>;

  // Prefer DB-persisted error, fall back to what the SSE stream reported.
  const errorMsg = task.ErrorMsg || (task.Status === "failed" ? streamError : null);

  return (
    <div className="space-y-4 max-w-4xl">
      <div className="flex items-start gap-3">
        <div className="flex-1">
          <h1 className="text-xl font-bold">{task.Goal}</h1>
          <p className="text-xs text-muted-foreground font-mono mt-0.5">{task.ID}</p>
        </div>
        <Badge variant={statusColor[task.Status] as "default"}>{task.Status}</Badge>
      </div>

      {task.Status === "failed" && errorMsg && (
        <div className="rounded-md border border-destructive/50 bg-destructive/10 px-4 py-3 text-sm text-destructive">
          <p className="font-semibold mb-1">Task failed</p>
          <p className="font-mono text-xs break-words">{errorMsg}</p>
        </div>
      )}

      <Tabs defaultValue="feed">
        <TabsList>
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
          <WorkspaceFiles since={task.CreatedAt} />
        </TabsContent>
      </Tabs>
    </div>
  );
}
