export interface EventAnalytics {
  passed_count: number;
  failed_count: number;
  tasks_closed: number;
  initial_ready: number;
  current_ready: number;
  avg_duration_ms: number;
  total_duration_ms: number;
}

export interface EventContext {
  session_id: string;
  session_start: string;
  max_iterations: number;
  current_iteration: number;
  status: string;
  current_phase: string;
  analytics?: EventAnalytics;
}

export interface EventData {
  iteration?: number;
  duration_ms?: number;
  task_id?: string;
  passed?: boolean;
  notes?: string;
  review_cycles?: number;
  verdict?: string;
  phase?: string;
  from_phase?: string;
  to_phase?: string;
  reason?: string;
  description?: string;
  commit_hash?: string;
  priority?: number;
  max_iterations?: number;
}

export interface RalphEvent {
  event_id: string;
  type: string;
  timestamp: string;
  instance_id: string;
  repo: string;
  epic: string;
  data?: EventData;
  context?: EventContext;
}

export interface InstanceState {
  instance_id: string;
  repo: string;
  epic?: string;
  status: string;
  last_event: string;
  context?: EventContext;
}

export interface Session {
  session_id: string;
  instance_id: string;
  repo: string;
  epic?: string;
  started_at: string;
  ended_at?: string;
  iterations: number;
  tasks_closed: number;
  pass_rate: number;
  end_reason?: string;
}

export interface AggregateStats {
  total_sessions: number;
  active_instances: number;
  total_tasks_closed: number;
  overall_pass_rate: number;
  total_iterations: number;
}
