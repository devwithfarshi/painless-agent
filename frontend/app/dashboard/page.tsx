import { api } from "@/lib/api";
import { TaskCard } from "@/components/agent/TaskCard";

export const dynamic = "force-dynamic";

export default async function DashboardPage() {
  let tasks: Awaited<ReturnType<typeof api.tasks.list>> = [];
  try {
    tasks = await api.tasks.list();
  } catch {
    // API may not be reachable during development
  }

  const running = tasks.filter((t) => t.Status === "running");
  const recent = tasks.slice(0, 10);

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold">Dashboard</h1>
        <p className="text-muted-foreground text-sm">
          {tasks.length} total tasks &mdash; {running.length} running
        </p>
      </div>

      {running.length > 0 && (
        <section>
          <h2 className="text-lg font-semibold mb-3">Running</h2>
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {running.map((t) => (
              <TaskCard key={t.ID} task={t} />
            ))}
          </div>
        </section>
      )}

      <section>
        <h2 className="text-lg font-semibold mb-3">Recent Tasks</h2>
        {recent.length === 0 ? (
          <p className="text-muted-foreground text-sm">No tasks yet. Create one from the Tasks page.</p>
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
