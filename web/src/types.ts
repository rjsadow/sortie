export type LaunchType = 'url' | 'container';

// ResourceLimits defines CPU and memory constraints for container applications
export interface ResourceLimits {
  cpu_request?: string;    // CPU request (e.g., "100m", "0.5")
  cpu_limit?: string;      // CPU limit (e.g., "1", "2")
  memory_request?: string; // Memory request (e.g., "256Mi", "1Gi")
  memory_limit?: string;   // Memory limit (e.g., "512Mi", "2Gi")
}

export interface Application {
  id: string;
  name: string;
  description: string;
  url: string;
  icon: string;
  category: string;
  launch_type: LaunchType;
  container_image?: string;
  resource_limits?: ResourceLimits; // Resource limits for container apps
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

// Template types for the Template Marketplace
export type TemplateCategory =
  | 'development'
  | 'productivity'
  | 'communication'
  | 'browsers'
  | 'monitoring'
  | 'databases'
  | 'creative';

export interface ApplicationTemplate extends Omit<Application, 'id'> {
  template_id: string;
  template_version: string;
  template_category: TemplateCategory;
  tags: string[];
  maintainer?: string;
  documentation_url?: string;
  recommended_limits?: ResourceLimits;
}

export interface TemplateCatalog {
  version: string;
  templates: ApplicationTemplate[];
}
