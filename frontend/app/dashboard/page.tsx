import { api } from "@/lib/api";
import { TaskCard } from "@/components/agent/TaskCard";
import { CheckCircle2, Clock, AlertCircle, ListTodo } from "lucide-react";
import Link from "next/link";

export const dynamic = "force-dynamic";

export default async function DashboardPage() {
  let tasks: Awaited<ReturnType<typeof api.tasks.list>> = [];
  try {
    tasks = await api.tasks.list();
  } catch {
    // API may not be reachable during development
  }

  const running = tasks.filter((t) => t.Status === "running");
  const completed = tasks.filter((t) => t.Status === "completed");
  const failed = tasks.filter((t) => t.Status === "failed");
  const recent = tasks.slice(0, 9);

  const stats = [
    { label: "Total Tasks", value: tasks.length, icon: ListTodo, color: "text-primary" },
    { label: "Running", value: running.length, icon: Clock, color: "text-blue-500" },
    { label: "Completed", value: completed.length, icon: CheckCircle2, color: "text-emerald-500" },
    { label: "Failed", value: failed.length, icon: AlertCircle, color: "text-red-500" },
  ];

  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Dashboard</h1>
        <p className="text-muted-foreground text-sm mt-1">
          Overview of your autonomous agent activity.
        </p>
      </div>

      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        {stats.map((s) => (
          <div key={s.label} className="rounded-lg border bg-card p-4">
            <div className="flex items-center justify-between mb-3">
              <span className="text-xs font-medium text-muted-foreground uppercase tracking-wide">
                {s.label}
              </span>
              <s.icon className={`w-4 h-4 ${s.color}`} />
            </div>
            <p className="text-2xl font-semibold">{s.value}</p>
          </div>
        ))}
      </div>

      {running.length > 0 && (
        <section>
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-base font-semibold">Running Now</h2>
            <span className="inline-flex items-center gap-1.5 text-xs text-blue-600 dark:text-blue-400">
              <span className="w-1.5 h-1.5 rounded-full bg-blue-500 animate-pulse" />
              {running.length} active
            </span>
          </div>
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {running.map((t) => (
              <TaskCard key={t.ID} task={t} />
            ))}
          </div>
        </section>
      )}

      <section>
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-base font-semibold">Recent Tasks</h2>
          <Link href="/tasks" className="text-xs text-primary hover:underline">
            View all
          </Link>
        </div>
        {recent.length === 0 ? (
          <div className="rounded-lg border border-dashed p-8 text-center">
            <ListTodo className="w-8 h-8 text-muted-foreground/50 mx-auto mb-2" />
            <p className="text-sm text-muted-foreground">No tasks yet.</p>
            <Link href="/tasks" className="text-sm text-primary hover:underline mt-1 inline-block">
              Create your first task
            </Link>
          </div>
        ) : (
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {recent.map((t) => (
              <TaskCard key={t.ID} task={t} />
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
