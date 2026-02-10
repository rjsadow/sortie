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
) {
  const res = await request.post(`${BASE}/api/sessions`, {
    headers: { Authorization: `Bearer ${token}` },
    data: { app_id: appId },
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
