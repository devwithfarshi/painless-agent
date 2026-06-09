"use client";

import Link from "next/link";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import type { Task } from "@/types/agent";

const statusColor: Record<string, string> = {
  pending: "secondary",
  running: "default",
  completed: "outline",
  failed: "destructive",
  cancelled: "secondary",
};

interface Props {
  task: Task;
}

export function TaskCard({ task }: Props) {
  const elapsed = Math.floor(
    (new Date(task.UpdatedAt).getTime() - new Date(task.CreatedAt).getTime()) / 1000
  );
  return (
    <Link href={`/tasks/${task.ID}`}>
      <Card className="hover:border-primary transition-colors cursor-pointer">
        <CardHeader className="pb-2 flex flex-row items-center justify-between">
          <span className="text-sm font-mono text-muted-foreground truncate max-w-xs">
            {task.ID}
          </span>
          <Badge variant={statusColor[task.Status] as "default"}>{task.Status}</Badge>
        </CardHeader>
        <CardContent>
          <p className="font-medium line-clamp-2">{task.Goal}</p>
          <p className="text-xs text-muted-foreground mt-1">
            {new Date(task.CreatedAt).toLocaleString()} &mdash; {elapsed}s
          </p>
        </CardContent>
      </Card>
    </Link>
  );
}
