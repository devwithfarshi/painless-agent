"use client";

import { useEffect, useState, useRef } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { TaskCard } from "@/components/agent/TaskCard";
import { api } from "@/lib/api";
import type { Task } from "@/types/agent";
import { PlusIcon, SendIcon } from "lucide-react";

export default function TasksPage() {
  const [tasks, setTasks] = useState<Task[]>([]);
  const [goal, setGoal] = useState("");
  const [loading, setLoading] = useState(false);
  const router = useRouter();
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const fetchTasks = async () => {
    try {
      const data = await api.tasks.list();
      setTasks(data ?? []);
    } catch {
      // ignore
    }
  };

  useEffect(() => {
    fetchTasks();
    const t = setInterval(fetchTasks, 5000);
    return () => clearInterval(t);
  }, []);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!goal.trim()) return;
    setLoading(true);
    try {
      const task = await api.tasks.create(goal.trim());
      setGoal("");
      router.push(`/tasks/${task.ID}`);
    } catch {
      alert("Failed to create task");
    } finally {
      setLoading(false);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      submit(e as unknown as React.FormEvent);
    }
  };

  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Tasks</h1>
        <p className="text-muted-foreground text-sm mt-1">
          Create a task and let the agent work autonomously.
        </p>
      </div>

      <div className="rounded-lg border bg-card p-5">
        <div className="flex items-center gap-2 mb-3">
          <PlusIcon className="w-4 h-4 text-primary" />
          <h2 className="text-sm font-semibold">New Task</h2>
        </div>
        <form onSubmit={submit} className="space-y-3">
          <textarea
            ref={textareaRef}
            value={goal}
            onChange={(e) => setGoal(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Describe a goal for the agent… (Ctrl+Enter to submit)"
            rows={3}
            className="w-full rounded-md border bg-background px-3 py-2.5 text-sm placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring resize-none"
          />
          <div className="flex items-center justify-between">
            <p className="text-xs text-muted-foreground">
              The agent will plan, execute tools, and report progress in real time.
            </p>
            <Button type="submit" disabled={loading || !goal.trim()} size="sm" className="gap-2">
              <SendIcon className="w-3.5 h-3.5" />
              {loading ? "Creating…" : "Run Task"}
            </Button>
          </div>
        </form>
      </div>

      <section>
        <h2 className="text-base font-semibold mb-4">
          All Tasks
          {tasks.length > 0 && (
            <span className="ml-2 text-xs font-normal text-muted-foreground">
              ({tasks.length})
            </span>
          )}
        </h2>

        {tasks.length === 0 ? (
          <div className="rounded-lg border border-dashed p-10 text-center">
            <p className="text-sm text-muted-foreground">
              No tasks yet. Enter a goal above and click Run Task.
            </p>
          </div>
        ) : (
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {tasks.map((t) => (
              <TaskCard key={t.ID} task={t} />
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
