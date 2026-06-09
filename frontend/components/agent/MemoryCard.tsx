"use client";

interface Props {
  content: string;
  index: number;
}

export function MemoryCard({ content, index }: Props) {
  return (
    <div className="rounded-lg border bg-card p-3 text-sm space-y-1">
      <span className="text-xs text-muted-foreground font-mono">#{index + 1}</span>
      <p className="line-clamp-4 text-foreground">{content}</p>
    </div>
  );
}
