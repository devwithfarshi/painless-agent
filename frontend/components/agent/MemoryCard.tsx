"use client";

import { MarkdownContent } from "@/components/ui/markdown";

interface Props {
  content: string;
  index: number;
}

export function MemoryCard({ content, index }: Props) {
  return (
    <div className="rounded-lg border bg-card overflow-hidden">
      <div className="flex items-center gap-2 px-4 py-2 border-b bg-muted/30">
        <span className="text-xs font-mono text-muted-foreground">#{index + 1}</span>
      </div>
      <div className="p-4">
        <MarkdownContent content={content} compact />
      </div>
    </div>
  );
}
