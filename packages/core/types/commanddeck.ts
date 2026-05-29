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
