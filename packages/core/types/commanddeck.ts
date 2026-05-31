export interface CommandTemplate {
  id: string;
  name: string;
  command: string;
  description?: string;
  category: string;
  risk_level: string;
  is_builtin: boolean;
  created_at: string;
}

export interface CommandRun {
  id: string;
  status: string;
  command: string;
  working_directory: string;
  exit_code?: number;
  stdout?: string;
  stderr?: string;
  stdout_truncated: boolean;
  stderr_truncated: boolean;
  cancellation_requested_at?: string;
  cancellation_requested_by_type?: string;
  cancellation_requested_by_id?: string;
  duration_ms?: number;
  started_at?: string;
  finished_at?: string;
  created_at: string;
}

export interface CommandRunExecuteRequest {
  runtime_id: string;
  template_id?: string;
  issue_id?: string;
}

export interface CommandRunListResponse {
  command_runs: CommandRun[];
  total: number;
}

export interface CommandTemplatesResponse {
  templates: CommandTemplate[];
}

export type PreviewHealthStatus = "healthy" | "unhealthy" | "unavailable" | "unknown";
export type PreviewLifecycleStatus = "registered" | "healthy" | "stale" | "offline" | "runtime_disconnected" | "retired";

export interface PreviewRegistryEntry {
  id: string;
  workspace_id: string;
  workspace_name: string;
  workspace_slug: string;
  project_id?: string | null;
  project_name?: string | null;
  runtime_id?: string | null;
  runtime_name?: string | null;
  runtime_status?: string | null;
  machine_identity?: string | null;
  preview_url: string;
  port: number;
  health_status: PreviewHealthStatus;
  lifecycle_status?: PreviewLifecycleStatus;
  health_status_code?: number | null;
  health_message?: string | null;
  health_error?: string | null;
  last_checked_at: string;
  last_success_at?: string | null;
  registered_at: string;
  updated_at: string;
  command_run_id?: string | null;
  command?: string | null;
  source: string;
}

export interface PreviewRegistryResponse {
  previews: PreviewRegistryEntry[];
  last_checked_at: string;
}

export type CommandWorkflowExecutionStatus =
  | "planned"
  | "running"
  | "needs_review"
  | "completed"
  | "failed"
  | "cancelled";

export interface CommandWorkflowExecution {
  id: string;
  workspace_id: string;
  project_id?: string | null;
  project_title?: string | null;
  command_run_id?: string | null;
  command_run_status?: string | null;
  command_run?: string | null;
  title: string;
  objective: string;
  status: CommandWorkflowExecutionStatus;
  created_by_type: string;
  created_by_id: string;
  created_at: string;
  updated_at: string;
}

export interface CommandWorkflowExecutionsResponse {
  workflow_executions: CommandWorkflowExecution[];
  total: number;
}

export interface CreateCommandWorkflowExecutionRequest {
  title: string;
  objective: string;
  project_id?: string;
  command_run_id?: string;
  status?: CommandWorkflowExecutionStatus;
}
