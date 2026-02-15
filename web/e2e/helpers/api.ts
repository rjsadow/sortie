import { APIRequestContext } from '@playwright/test';

const BASE = 'http://localhost:3847';

export async function getAdminToken(request: APIRequestContext): Promise<string> {
  const res = await request.post(`${BASE}/api/auth/login`, {
    data: { username: 'admin', password: 'admin123' },
  });
  const body = await res.json();
  return body.access_token;
}

export async function createTestApp(
  request: APIRequestContext,
  token: string,
  app: {
    id: string;
    name: string;
    description?: string;
    category?: string;
    launch_type?: string;
    url?: string;
  },
) {
  const res = await request.post(`${BASE}/api/apps`, {
    headers: { Authorization: `Bearer ${token}` },
    data: {
      id: app.id,
      name: app.name,
      description: app.description ?? 'Test app description',
      category: app.category ?? 'Development',
      launch_type: app.launch_type ?? 'url',
      url: app.url ?? 'https://example.com',
    },
  });
  return res;
}

export async function deleteTestApp(
  request: APIRequestContext,
  token: string,
  id: string,
) {
  return request.delete(`${BASE}/api/apps/${id}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
}

export async function createTestUser(
  request: APIRequestContext,
  token: string,
  user: { username: string; password: string; email?: string; roles?: string[] },
) {
  return request.post(`${BASE}/api/admin/users`, {
    headers: { Authorization: `Bearer ${token}` },
    data: {
      username: user.username,
      password: user.password,
      email: user.email ?? '',
      roles: user.roles ?? ['user'],
    },
  });
}

export async function deleteTestUser(
  request: APIRequestContext,
  token: string,
  id: string,
) {
  return request.delete(`${BASE}/api/admin/users/${id}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
}

// --- Session helpers ---

export async function createTestSession(
  request: APIRequestContext,
  token: string,
  appId: string,
  userId?: string,
) {
  const data: Record<string, string> = { app_id: appId };
  if (userId) data.user_id = userId;
  const res = await request.post(`${BASE}/api/sessions`, {
    headers: { Authorization: `Bearer ${token}` },
    data,
  });
  return res;
}

export async function listSessions(
  request: APIRequestContext,
  token: string,
) {
  return request.get(`${BASE}/api/sessions`, {
    headers: { Authorization: `Bearer ${token}` },
  });
}

export async function getSession(
  request: APIRequestContext,
  token: string,
  id: string,
) {
  return request.get(`${BASE}/api/sessions/${id}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
}

export async function stopSession(
  request: APIRequestContext,
  token: string,
  id: string,
) {
  return request.post(`${BASE}/api/sessions/${id}/stop`, {
    headers: { Authorization: `Bearer ${token}` },
  });
}

export async function terminateTestSession(
  request: APIRequestContext,
  token: string,
  id: string,
) {
  return request.delete(`${BASE}/api/sessions/${id}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
}

export async function waitForSessionRunning(
  request: APIRequestContext,
  token: string,
  id: string,
  timeoutMs = 10_000,
): Promise<void> {
  const start = Date.now();
  while (Date.now() - start < timeoutMs) {
    const res = await getSession(request, token, id);
    if (res.ok()) {
      const body = await res.json();
      if (body.status === 'running') return;
      if (body.status === 'failed') throw new Error(`Session ${id} failed`);
    }
    await new Promise((r) => setTimeout(r, 250));
  }
  throw new Error(`Session ${id} did not reach running within ${timeoutMs}ms`);
}

// --- Auth helpers ---

export async function loginAs(
  request: APIRequestContext,
  username: string,
  password: string,
): Promise<string> {
  const res = await request.post(`${BASE}/api/auth/login`, {
    data: { username, password },
  });
  const body = await res.json();
  return body.access_token;
}

// --- Session sharing helpers ---

export async function createSessionShare(
  request: APIRequestContext,
  token: string,
  sessionId: string,
  data: { username?: string; permission?: string; link_share?: boolean },
) {
  return request.post(`${BASE}/api/sessions/${sessionId}/shares`, {
    headers: { Authorization: `Bearer ${token}` },
    data,
  });
}

export async function listSessionShares(
  request: APIRequestContext,
  token: string,
  sessionId: string,
) {
  return request.get(`${BASE}/api/sessions/${sessionId}/shares`, {
    headers: { Authorization: `Bearer ${token}` },
  });
}

export async function deleteSessionShare(
  request: APIRequestContext,
  token: string,
  sessionId: string,
  shareId: string,
) {
  return request.delete(`${BASE}/api/sessions/${sessionId}/shares/${shareId}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
}

export async function listSharedSessions(
  request: APIRequestContext,
  token: string,
) {
  return request.get(`${BASE}/api/sessions/shared`, {
    headers: { Authorization: `Bearer ${token}` },
  });
}

export async function joinSessionShare(
  request: APIRequestContext,
  token: string,
  shareToken: string,
) {
  return request.post(`${BASE}/api/sessions/shares/join`, {
    headers: { Authorization: `Bearer ${token}` },
    data: { token: shareToken },
  });
}

// --- Recording helpers ---

export async function listRecordings(
  request: APIRequestContext,
  token: string,
) {
  return request.get(`${BASE}/api/recordings`, {
    headers: { Authorization: `Bearer ${token}` },
  });
}

export async function listAdminRecordings(
  request: APIRequestContext,
  token: string,
) {
  return request.get(`${BASE}/api/admin/recordings`, {
    headers: { Authorization: `Bearer ${token}` },
  });
}

export async function startRecording(
  request: APIRequestContext,
  token: string,
  sessionId: string,
) {
  return request.post(`${BASE}/api/sessions/${sessionId}/recording/start`, {
    headers: { Authorization: `Bearer ${token}` },
  });
}

export async function stopRecording(
  request: APIRequestContext,
  token: string,
  sessionId: string,
  recordingId: string,
) {
  return request.post(`${BASE}/api/sessions/${sessionId}/recording/stop`, {
    headers: { Authorization: `Bearer ${token}` },
    data: { recording_id: recordingId },
  });
}

export async function uploadRecording(
  request: APIRequestContext,
  token: string,
  sessionId: string,
  recordingId: string,
  fileContent: Buffer,
  duration?: number,
) {
  return request.post(`${BASE}/api/sessions/${sessionId}/recording/upload`, {
    headers: { Authorization: `Bearer ${token}` },
    multipart: {
      recording_id: recordingId,
      duration: String(duration ?? 5.0),
      file: {
        name: 'recording.webm',
        mimeType: 'video/webm',
        buffer: fileContent,
      },
    },
  });
}

export async function downloadRecording(
  request: APIRequestContext,
  token: string,
  recordingId: string,
) {
  return request.get(`${BASE}/api/recordings/${recordingId}/download`, {
    headers: { Authorization: `Bearer ${token}` },
  });
}

export async function deleteRecording(
  request: APIRequestContext,
  token: string,
  recordingId: string,
) {
  return request.delete(`${BASE}/api/recordings/${recordingId}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
}

// --- Settings helpers ---

export async function getAdminSettings(
  request: APIRequestContext,
  token: string,
) {
  return request.get(`${BASE}/api/admin/settings`, {
    headers: { Authorization: `Bearer ${token}` },
  });
}

export async function updateAdminSettings(
  request: APIRequestContext,
  token: string,
  settings: Record<string, string>,
) {
  return request.put(`${BASE}/api/admin/settings`, {
    headers: { Authorization: `Bearer ${token}` },
    data: settings,
  });
}
