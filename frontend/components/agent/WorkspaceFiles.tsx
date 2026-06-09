"use client";

import { useEffect, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { api } from "@/lib/api";
import type { WorkspaceFile } from "@/types/agent";

const TEXT_EXTS = new Set([
  ".txt", ".md", ".py", ".js", ".ts", ".tsx", ".jsx", ".go", ".sh", ".bash",
  ".json", ".yaml", ".yml", ".toml", ".env", ".html", ".css", ".rs", ".rb",
  ".java", ".c", ".cpp", ".h", ".csv", ".xml", ".sql",
]);

function isText(filename: string): boolean {
  const dot = filename.lastIndexOf(".");
  return dot !== -1 && TEXT_EXTS.has(filename.slice(dot).toLowerCase());
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

interface Props {
  since?: string; // ISO date — only show files modified after this timestamp
}

export function WorkspaceFiles({ since }: Props) {
  const [files, setFiles] = useState<WorkspaceFile[]>([]);
  const [loading, setLoading] = useState(true);
  const [expanded, setExpanded] = useState<Record<string, string | null>>({});

  useEffect(() => {
    const sinceTs = since ? Math.floor(new Date(since).getTime() / 1000) : undefined;
    api.workspace.list(sinceTs).then((f) => {
      setFiles(f);
      setLoading(false);
    });
  }, [since]);

  const toggleView = async (path: string) => {
    if (expanded[path] !== undefined) {
      setExpanded((prev) => { const n = { ...prev }; delete n[path]; return n; });
      return;
    }
    setExpanded((prev) => ({ ...prev, [path]: null })); // null = loading
    try {
      const content = await api.workspace.content(path);
      setExpanded((prev) => ({ ...prev, [path]: content }));
    } catch {
      setExpanded((prev) => ({ ...prev, [path]: "(failed to load file content)" }));
    }
  };

  if (loading) return <p className="text-muted-foreground text-sm py-4 text-center">Loading…</p>;
  if (files.length === 0) {
    return (
      <p className="text-muted-foreground text-sm py-4 text-center">
        No files were created in the workspace during this task.
      </p>
    );
  }

  return (
    <div className="space-y-2">
      {files.map((f) => (
        <div key={f.path} className="border rounded-md overflow-hidden">
          <div className="flex items-center gap-3 px-3 py-2.5 bg-muted/30">
            <span className="font-mono text-sm flex-1 truncate" title={f.path}>
              {f.path}
            </span>
            <span className="text-xs text-muted-foreground shrink-0">{formatSize(f.size)}</span>
            <div className="flex gap-1.5 shrink-0">
              {isText(f.name) && (
                <Badge
                  variant="outline"
                  className="cursor-pointer hover:bg-accent text-xs px-2 py-0.5"
                  onClick={() => toggleView(f.path)}
                >
                  {expanded[f.path] !== undefined ? "Hide" : "View"}
                </Badge>
              )}
              <a
                href={api.workspace.fileUrl(f.path)}
                download={f.name}
                className="inline-flex items-center rounded-md border px-2 py-0.5 text-xs font-semibold transition-colors hover:bg-accent"
              >
                Download
              </a>
            </div>
          </div>
          {expanded[f.path] !== undefined && (
            <pre className="px-3 py-2.5 text-xs font-mono overflow-auto max-h-72 border-t bg-background text-foreground">
              {expanded[f.path] ?? "Loading…"}
            </pre>
          )}
        </div>
      ))}
    </div>
  );
}
