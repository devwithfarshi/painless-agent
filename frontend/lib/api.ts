import type { Task, TaskStep, Skill, WorkspaceFile, GeneratedFile } from "@/types/agent";

const BASE = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

async function requestText(path: string): Promise<string> {
  const apiKey = process.env.NEXT_PUBLIC_API_KEY ?? "";
  const headers: HeadersInit = apiKey ? { "X-API-Key": apiKey } : {};
  const res = await fetch(`${BASE}${path}`, { headers });
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return res.text();
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const apiKey = process.env.NEXT_PUBLIC_API_KEY ?? "";
  const headers: HeadersInit = {
    "Content-Type": "application/json",
    ...(apiKey ? { "X-API-Key": apiKey } : {}),
    ...init?.headers,
  };
  const res = await fetch(`${BASE}${path}`, { ...init, headers });
  if (!res.ok) {
    const body = await res.text();
    throw new Error(`${res.status} ${res.statusText}: ${body}`);
  }
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

// Tasks
export const api = {
  tasks: {
    list: (): Promise<Task[]> => request("/api/tasks"),
    create: (goal: string): Promise<Task> =>
      request("/api/tasks", { method: "POST", body: JSON.stringify({ goal }) }),
    get: (id: string): Promise<{ task: Task; steps: TaskStep[] }> =>
      request(`/api/tasks/${id}`),
    cancel: (id: string): Promise<void> =>
      request(`/api/tasks/${id}`, { method: "DELETE" }),
    files: (id: string): Promise<GeneratedFile[]> =>
      request(`/api/tasks/${id}/files`),
  },
  memory: {
    list: (limit = 50): Promise<{ results: { content: string; created_at: string }[] }> =>
      request(`/api/memory?limit=${limit}`),
    search: (q: string, k = 10): Promise<{ results: { content: string; created_at: string }[] }> =>
      request(`/api/memory/search?q=${encodeURIComponent(q)}&k=${k}`),
  },
  skills: {
    list: (): Promise<Skill[]> => request("/api/skills"),
    get: (id: string): Promise<Skill> => request(`/api/skills/${id}`),
    delete: (id: string): Promise<void> =>
      request(`/api/skills/${id}`, { method: "DELETE" }),
  },
  health: (): Promise<{ status: string; db: string; redis: string }> =>
    request("/api/health"),
  workspace: {
    list: (since?: number): Promise<WorkspaceFile[]> =>
      request(`/api/workspace${since !== undefined ? `?since=${since}` : ""}`),
    content: (path: string): Promise<string> =>
      requestText(`/api/workspace/file?path=${encodeURIComponent(path)}`),
    fileUrl: (path: string): string =>
      `${BASE}/api/workspace/file?path=${encodeURIComponent(path)}&download=1`,
  },
};
