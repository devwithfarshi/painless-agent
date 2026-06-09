export type TaskStatus = "pending" | "running" | "completed" | "failed" | "cancelled";
export type StepStatus = "pending" | "running" | "completed" | "failed";

export interface PlanStep {
  description: string;
  tool_hint?: string;
}

export interface Task {
  ID: string;
  Goal: string;
  Status: TaskStatus;
  Plan: PlanStep[] | null;
  ErrorMsg?: string;
  CreatedAt: string;
  UpdatedAt: string;
}

export interface TaskStep {
  ID: string;
  TaskID: string;
  Description: string;
  Status: StepStatus;
  Output: string;
  ToolUsed: string;
  CreatedAt: string;
  UpdatedAt: string;
}

export interface SkillStep {
  description: string;
  tool_hint?: string;
}

export interface Skill {
  ID: string;
  Name: string;
  Description: string;
  Steps: SkillStep[] | null;
  UsageCount: number;
  CreatedAt: string;
}

export interface WorkspaceFile {
  name: string;
  path: string;
  size: number;
  modified_at: string;
}

export interface AgentEvent {
  task_id: string;
  type: string;
  payload?: Record<string, unknown>;
}

export const EVENT_TYPES = {
  TASK_STARTED: "task_started",
  STEP_STARTED: "step_started",
  TOOL_CALLED: "tool_called",
  TOOL_RESULT: "tool_result",
  STEP_DONE: "step_done",
  STEP_FAILED: "step_failed",
  TASK_DONE: "task_done",
  TASK_FAILED: "task_failed",
} as const;
