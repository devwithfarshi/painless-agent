"use client";

import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import type { WorkspaceFile } from "@/types/agent";
import { FileIcon, DownloadIcon, EyeIcon, EyeOffIcon } from "lucide-react";

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
  since?: string;
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
    setExpanded((prev) => ({ ...prev, [path]: null }));
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
      <div className="py-8 text-center">
        <FileIcon className="w-7 h-7 text-muted-foreground/40 mx-auto mb-2" />
        <p className="text-sm text-muted-foreground">
          No files were created in the workspace during this task.
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-2">
      {files.map((f) => (
        <div key={f.path} className="border rounded-lg overflow-hidden">
          <div className="flex items-center gap-3 px-4 py-3">
            <FileIcon className="w-4 h-4 text-muted-foreground shrink-0" />
            <span className="font-mono text-sm flex-1 truncate" title={f.path}>
              {f.path}
            </span>
            <span className="text-xs text-muted-foreground shrink-0">{formatSize(f.size)}</span>
            <div className="flex items-center gap-2 shrink-0">
              {isText(f.name) && (
                <button
                  onClick={() => toggleView(f.path)}
                  className="inline-flex items-center gap-1.5 rounded-md border px-2.5 py-1 text-xs font-medium hover:bg-accent transition-colors"
                >
                  {expanded[f.path] !== undefined ? (
                    <><EyeOffIcon className="w-3.5 h-3.5" />Hide</>
                  ) : (
                    <><EyeIcon className="w-3.5 h-3.5" />View</>
                  )}
                </button>
              )}
              <a
                href={api.workspace.fileUrl(f.path)}
                download={f.name}
                className="inline-flex items-center gap-1.5 rounded-md border px-2.5 py-1 text-xs font-medium hover:bg-accent transition-colors"
              >
                <DownloadIcon className="w-3.5 h-3.5" />
                Download
              </a>
            </div>
          </div>
          {expanded[f.path] !== undefined && (
            <pre className="px-4 py-3 text-xs font-mono overflow-auto max-h-72 border-t bg-muted/30 text-foreground whitespace-pre-wrap leading-relaxed">
              {expanded[f.path] ?? "Loading…"}
            </pre>
          )}
        </div>
      ))}
    </div>
  );
}
