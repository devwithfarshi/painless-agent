"use client";

interface Props {
  status: string;
  size?: "sm" | "default";
}

const config: Record<string, { label: string; dot: string; bg: string; text: string }> = {
  pending: {
    label: "Pending",
    dot: "bg-amber-400",
    bg: "bg-amber-50 border-amber-200 dark:bg-amber-950/30 dark:border-amber-800/50",
    text: "text-amber-700 dark:text-amber-400",
  },
  running: {
    label: "Running",
    dot: "bg-blue-500 animate-pulse",
    bg: "bg-blue-50 border-blue-200 dark:bg-blue-950/30 dark:border-blue-800/50",
    text: "text-blue-700 dark:text-blue-400",
  },
  completed: {
    label: "Completed",
    dot: "bg-emerald-500",
    bg: "bg-emerald-50 border-emerald-200 dark:bg-emerald-950/30 dark:border-emerald-800/50",
    text: "text-emerald-700 dark:text-emerald-400",
  },
  failed: {
    label: "Failed",
    dot: "bg-red-500",
    bg: "bg-red-50 border-red-200 dark:bg-red-950/30 dark:border-red-800/50",
    text: "text-red-700 dark:text-red-400",
  },
  cancelled: {
    label: "Cancelled",
    dot: "bg-slate-400",
    bg: "bg-slate-100 border-slate-200 dark:bg-slate-800/30 dark:border-slate-700/50",
    text: "text-slate-600 dark:text-slate-400",
  },
};

export function StatusBadge({ status, size = "default" }: Props) {
  const c = config[status] ?? {
    label: status,
    dot: "bg-slate-400",
    bg: "bg-slate-100 border-slate-200",
    text: "text-slate-600",
  };

  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 font-medium ${
        size === "sm" ? "text-xs" : "text-xs"
      } ${c.bg} ${c.text}`}
    >
      <span className={`w-1.5 h-1.5 rounded-full ${c.dot}`} />
      {c.label}
    </span>
  );
}
