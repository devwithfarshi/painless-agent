"use client";

import { useEffect, useRef } from "react";
import { Badge } from "@/components/ui/badge";
import { Separator } from "@/components/ui/separator";
import { useTaskStream } from "@/lib/sse";
import type { AgentEvent } from "@/types/agent";

const eventDot: Record<string, string> = {
  task_started: "bg-blue-500",
  step_started: "bg-purple-500",
  tool_called: "bg-yellow-500",
  step_done: "bg-green-500",
  step_failed: "bg-red-500",
  task_done: "bg-emerald-600",
  task_failed: "bg-red-600",
};

function renderEvent(ev: AgentEvent): { label: string; message: string; isError: boolean } {
  const p = ev.payload ?? {};
  switch (ev.type) {
    case "task_started":
      return { label: "started", message: `Goal: ${p.goal ?? ""}`, isError: false };
    case "step_started":
      return {
        label: `step ${p.step_index ?? ""}`,
        message: String(p.description ?? ""),
        isError: false,
      };
    case "tool_called":
      return {
        label: "tool",
        message: `${p.tool ?? "unknown"} (${p.duration ?? 0}ms)`,
        isError: false,
      };
    case "step_done": {
      const out = String(p.output ?? "");
      return {
        label: "done",
        message: out.length > 150 ? out.slice(0, 150) + "…" : out,
        isError: false,
      };
    }
    case "step_failed":
      return { label: "step failed", message: String(p.error ?? "step failed"), isError: true };
    case "task_done":
      return { label: "completed", message: "Task finished successfully.", isError: false };
    case "task_failed":
      return { label: "failed", message: String(p.error ?? "Task failed."), isError: true };
    default:
      return { label: ev.type, message: p ? JSON.stringify(p) : "", isError: false };
  }
}

interface Props {
  taskId: string;
  onError?: (msg: string) => void;
}

export function AgentFeed({ taskId, onError }: Props) {
  const { events, connected, done } = useTaskStream(taskId);
  const bottomRef = useRef<HTMLDivElement>(null);
  const reportedError = useRef(false);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [events]);

  // Bubble the error up to the parent so the error banner can show it
  // even for tasks that have no ErrorMsg in the DB (e.g. old failures).
  useEffect(() => {
    if (reportedError.current || !onError) return;
    const failEv = events.find((e) => e.type === "task_failed");
    if (failEv?.payload?.error) {
      reportedError.current = true;
      onError(String(failEv.payload.error));
    }
  }, [events, onError]);

  return (
    <div className="flex flex-col gap-1">
      <div className="flex items-center gap-2 mb-2">
        <span className="text-sm font-medium">Live Feed</span>
        <Badge variant={connected ? "default" : "secondary"}>
          {done ? "finished" : connected ? "live" : "disconnected"}
        </Badge>
      </div>
      <Separator />
      <div className="space-y-1.5 mt-2 text-sm max-h-[600px] overflow-y-auto">
        {events.length === 0 && (
          <p className="text-muted-foreground text-center py-4">
            {connected ? "Waiting for events…" : "No events yet"}
          </p>
        )}
        {events.map((ev, i) => {
          const { label, message, isError } = renderEvent(ev);
          return (
            <div key={i} className="flex items-start gap-2 py-0.5">
              <span
                className={`mt-1.5 w-2 h-2 rounded-full shrink-0 ${eventDot[ev.type] ?? "bg-gray-400"}`}
              />
              <span className="text-muted-foreground w-24 shrink-0 text-xs pt-0.5">{label}</span>
              <span className={`break-words flex-1 ${isError ? "text-destructive font-medium" : ""}`}>
                {message}
              </span>
            </div>
          );
        })}
        <div ref={bottomRef} />
      </div>
    </div>
  );
}
