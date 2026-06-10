"use client";

import { useEffect, useRef, useState } from "react";
import { useTaskStream } from "@/lib/sse";
import { StatusBadge } from "@/components/agent/StatusBadge";
import { MarkdownContent } from "@/components/ui/markdown";
import type { AgentEvent } from "@/types/agent";
import {
  PlayIcon,
  WrenchIcon,
  CheckIcon,
  XIcon,
  ChevronDownIcon,
  ChevronRightIcon,
  ZapIcon,
} from "lucide-react";

interface EventConfig {
  icon: React.ElementType;
  iconColor: string;
  label: string;
}

const eventConfig: Record<string, EventConfig> = {
  task_started: { icon: PlayIcon, iconColor: "text-blue-500", label: "Task started" },
  step_started: { icon: ZapIcon, iconColor: "text-purple-500", label: "Step" },
  tool_called: { icon: WrenchIcon, iconColor: "text-amber-500", label: "Tool" },
  step_done: { icon: CheckIcon, iconColor: "text-emerald-500", label: "Step done" },
  step_failed: { icon: XIcon, iconColor: "text-red-500", label: "Step failed" },
  task_done: { icon: CheckIcon, iconColor: "text-emerald-600", label: "Task completed" },
  task_failed: { icon: XIcon, iconColor: "text-red-600", label: "Task failed" },
};

interface FeedEvent {
  id: number;
  ev: AgentEvent;
  expanded: boolean;
}

function EventRow({ fe, onToggle }: { fe: FeedEvent; onToggle: () => void }) {
  const { ev } = fe;
  const cfg = eventConfig[ev.type] ?? { icon: ZapIcon, iconColor: "text-muted-foreground", label: ev.type };
  const Icon = cfg.icon;
  const p = ev.payload ?? {};

  let title = "";
  let body: string | null = null;
  let isMarkdown = false;
  let isError = false;

  switch (ev.type) {
    case "task_started":
      title = `Goal: ${p.goal ?? ""}`;
      break;
    case "step_started":
      title = p.description ? String(p.description) : `Step ${p.step_index ?? ""}`;
      break;
    case "tool_called":
      title = `${p.tool ?? "unknown"}`;
      body = p.duration ? `${p.duration}ms` : null;
      break;
    case "step_done": {
      const out = String(p.output ?? "");
      title = out.length > 80 ? out.slice(0, 80) + "…" : out;
      body = out.length > 80 ? out : null;
      isMarkdown = true;
      break;
    }
    case "step_failed":
      title = String(p.error ?? "Step failed");
      isError = true;
      break;
    case "task_done":
      title = "Task finished successfully.";
      break;
    case "task_failed":
      title = String(p.error ?? "Task failed.");
      isError = true;
      break;
    default:
      title = p ? JSON.stringify(p) : ev.type;
  }

  const hasBody = body !== null;

  return (
    <div className={`flex items-start gap-3 py-2.5 px-3 rounded-md ${isError ? "bg-red-50 dark:bg-red-950/20" : "hover:bg-muted/50"} transition-colors`}>
      <div className={`w-6 h-6 rounded-full flex items-center justify-center shrink-0 mt-0.5 ${isError ? "bg-red-100 dark:bg-red-900/30" : "bg-muted"}`}>
        <Icon className={`w-3.5 h-3.5 ${isError ? "text-red-500" : cfg.iconColor}`} />
      </div>

      <div className="flex-1 min-w-0">
        <div className="flex items-start gap-2">
          <span className="text-xs font-medium text-muted-foreground w-20 shrink-0 pt-0.5">
            {cfg.label}
          </span>
          <div className="flex-1 min-w-0">
            <p className={`text-sm leading-snug ${isError ? "text-red-700 dark:text-red-400 font-medium" : ""}`}>
              {title}
            </p>
            {hasBody && (
              <button
                onClick={onToggle}
                className="mt-1 flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors"
              >
                {fe.expanded ? (
                  <ChevronDownIcon className="w-3 h-3" />
                ) : (
                  <ChevronRightIcon className="w-3 h-3" />
                )}
                {fe.expanded ? "Collapse" : "Expand output"}
              </button>
            )}
            {hasBody && fe.expanded && body && (
              <div className="mt-2 border rounded-md overflow-hidden">
                {isMarkdown ? (
                  <div className="p-3">
                    <MarkdownContent content={body} compact />
                  </div>
                ) : (
                  <pre className="p-3 text-xs font-mono overflow-x-auto bg-muted/50 text-foreground whitespace-pre-wrap">
                    {body}
                  </pre>
                )}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

interface Props {
  taskId: string;
  onError?: (msg: string) => void;
}

export function AgentFeed({ taskId, onError }: Props) {
  const { events, connected, done } = useTaskStream(taskId);
  const bottomRef = useRef<HTMLDivElement>(null);
  const reportedError = useRef(false);
  const [expandedMap, setExpandedMap] = useState<Record<number, boolean>>({});

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [events]);

  useEffect(() => {
    if (reportedError.current || !onError) return;
    const failEv = events.find((e) => e.type === "task_failed");
    if (failEv?.payload?.error) {
      reportedError.current = true;
      onError(String(failEv.payload.error));
    }
  }, [events, onError]);

  const feedEvents: FeedEvent[] = events.map((ev, i) => ({
    id: i,
    ev,
    expanded: expandedMap[i] ?? false,
  }));

  const toggleExpand = (id: number) => {
    setExpandedMap((prev) => ({ ...prev, [id]: !prev[id] }));
  };

  const connectionStatus = done ? "completed" : connected ? "running" : "pending";

  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center justify-between pb-3 border-b">
        <span className="text-sm font-medium">Agent Activity</span>
        <StatusBadge status={connectionStatus} size="sm" />
      </div>

      <div className="space-y-0.5 max-h-[600px] overflow-y-auto">
        {feedEvents.length === 0 && (
          <div className="py-10 text-center">
            <p className="text-sm text-muted-foreground">
              {connected ? "Waiting for the agent to start…" : "No events yet."}
            </p>
          </div>
        )}

        {feedEvents.map((fe) => (
          <EventRow key={fe.id} fe={fe} onToggle={() => toggleExpand(fe.id)} />
        ))}
        <div ref={bottomRef} />
      </div>
    </div>
  );
}
