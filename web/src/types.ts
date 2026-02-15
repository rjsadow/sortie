export type LaunchType = 'url' | 'container' | 'web_proxy';
export type OsType = 'linux' | 'windows';

// Clipboard policy controls what clipboard operations are allowed between local and remote.
//   'none'          - clipboard sync disabled entirely
//   'read'          - remote → local only (copy from remote)
//   'write'         - local → remote only (paste into remote)
//   'bidirectional' - full two-way clipboard sync
export type ClipboardPolicy = 'none' | 'read' | 'write' | 'bidirectional';

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
  visibility?: AppVisibility;
  launch_type: LaunchType;
  os_type?: OsType;
  container_image?: string;
  container_port?: number;  // Port web app listens on (default: 8080 for web_proxy)
  container_args?: string[]; // Extra arguments to pass to the container
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
  websocket_url?: string;    // For Linux container apps (VNC)
  guacamole_url?: string;    // For Windows container apps (RDP via Guacamole)
  proxy_url?: string;        // For web_proxy apps
  is_shared?: boolean;
  owner_username?: string;
  share_permission?: 'read_only' | 'read_write';
  share_id?: string;
  created_at: string;
  updated_at: string;
}

export interface SessionShare {
  id: string;
  session_id: string;
  user_id?: string;
  username?: string;
  permission: 'read_only' | 'read_write';
  share_url?: string;
  created_at: string;
}

export interface CreateSessionRequest {
  app_id: string;
  user_id?: string;
  screen_width?: number;
  screen_height?: number;
}

export interface User {
  id?: string;
  username: string;
  email?: string;
  name?: string;
  displayName?: string;
  roles?: string[];
  admin_categories?: string[];
}

export interface AuthResponse {
  access_token: string;
  refresh_token: string;
  expires_in: number;
  user: User;
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
  container_args?: string[]; // Extra arguments to pass to the container
}

export interface TemplateCatalog {
  version: string;
  templates: ApplicationTemplate[];
}

// App-level visibility controls who can see each application
export type AppVisibility = 'admin_only' | 'approved' | 'public';
// Keep alias for backwards compatibility
export type CategoryVisibility = AppVisibility;

export interface Category {
  id: string;
  name: string;
  description: string;
  tenant_id?: string;
  created_at: string;
  updated_at: string;
}

// Video recording types
export type RecordingStatus = 'recording' | 'uploading' | 'ready' | 'failed';

export interface Recording {
  id: string;
  session_id: string;
  user_id: string;
  filename: string;
  size_bytes: number;
  duration_seconds: number;
  format: string;
  status: RecordingStatus;
  created_at: string;
  completed_at?: string;
}

// Audit log types
export interface AuditLogEntry {
  id: number;
  timestamp: string;
  user: string;
  action: string;
  details: string;
}

export interface AuditLogPage {
  logs: AuditLogEntry[];
  total: number;
}

export interface AuditLogFilters {
  actions: string[];
  users: string[];
}
