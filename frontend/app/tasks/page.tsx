"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { TaskCard } from "@/components/agent/TaskCard";
import { api } from "@/lib/api";
import type { Task } from "@/types/agent";

export default function TasksPage() {
  const [tasks, setTasks] = useState<Task[]>([]);
  const [goal, setGoal] = useState("");
  const [loading, setLoading] = useState(false);
  const router = useRouter();

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

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold">Tasks</h1>

      <form onSubmit={submit} className="flex gap-2">
        <input
          value={goal}
          onChange={(e) => setGoal(e.target.value)}
          placeholder="Describe a goal for the agent…"
          className="flex-1 rounded-md border bg-background px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-ring"
        />
        <Button type="submit" disabled={loading || !goal.trim()}>
          {loading ? "Creating…" : "Run"}
        </Button>
      </form>

      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {tasks.map((t) => (
          <TaskCard key={t.ID} task={t} />
        ))}
      </div>

      {tasks.length === 0 && (
        <p className="text-muted-foreground text-sm text-center py-8">
          No tasks yet. Enter a goal above and press Run.
        </p>
      )}
    </div>
  );
}
