export type LaunchType = 'url' | 'container';

export interface Application {
  id: string;
  name: string;
  description: string;
  url: string;
  icon: string;
  category: string;
  launch_type: LaunchType;
  container_image?: string;
}

export interface AppConfig {
  applications: Application[];
}

// Session state machine:
//   creating -> running (pod ready)
//   creating -> failed  (pod creation failed)
//   running  -> stopped (user terminated)
//   running  -> expired (timeout cleanup)
//   running  -> failed  (runtime error)
export type SessionStatus = 'creating' | 'running' | 'failed' | 'stopped' | 'expired';

export interface Session {
  id: string;
  user_id: string;
  app_id: string;
  app_name?: string;
  pod_name: string;
  status: SessionStatus;
  websocket_url?: string;
  created_at: string;
  updated_at: string;
}

export interface CreateSessionRequest {
  app_id: string;
  user_id?: string;
}

export interface User {
  username: string;
  displayName?: string;
}
