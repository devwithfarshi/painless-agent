"use client";

import { MarkdownContent } from "@/components/ui/markdown";

interface Props {
  content: string;
  created_at?: string;
  index: number;
}

export function MemoryCard({ content, created_at, index }: Props) {
  return (
    <div className="rounded-lg border bg-card overflow-hidden">
      <div className="flex items-center justify-between px-4 py-2 border-b bg-muted/30">
        <span className="text-xs font-mono text-muted-foreground">#{index + 1}</span>
        {created_at && (
          <span className="text-xs text-muted-foreground">
            {new Date(created_at).toLocaleString()}
          </span>
        )}
      </div>
      <div className="p-4">
        <MarkdownContent content={content} compact />
      </div>
    </div>
  );
}
