"use client";

import { useEffect, useRef, useState } from "react";
import type { AgentEvent } from "@/types/agent";

const BASE = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

export function useTaskStream(taskId: string | null) {
  const [events, setEvents] = useState<AgentEvent[]>([]);
  const [connected, setConnected] = useState(false);
  const [done, setDone] = useState(false);
  const esRef = useRef<EventSource | null>(null);

  useEffect(() => {
    if (!taskId) return;

    const apiKey = process.env.NEXT_PUBLIC_API_KEY ?? "";
    // EventSource doesn't support custom headers; pass key as query param
    // only when auth is enabled. For production use a proxy or token exchange.
    const url = `${BASE}/api/tasks/${taskId}/stream${apiKey ? `?api_key=${apiKey}` : ""}`;
    const es = new EventSource(url);
    esRef.current = es;

    es.onopen = () => setConnected(true);

    es.onmessage = (e: MessageEvent) => {
      try {
        const ev: AgentEvent = JSON.parse(e.data as string);
        setEvents((prev) => [...prev, ev]);
        if (ev.type === "task_done" || ev.type === "task_failed") {
          setDone(true);
          es.close();
        }
      } catch {
        // ignore malformed events
      }
    };

    es.onerror = () => {
      setConnected(false);
      es.close();
    };

    return () => {
      es.close();
      esRef.current = null;
    };
  }, [taskId]);

  const reset = () => {
    setEvents([]);
    setDone(false);
    setConnected(false);
  };

  return { events, connected, done, reset };
}
