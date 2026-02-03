import type { User } from '../types';

// Auth response types
export interface AuthResponse {
  access_token: string;
  refresh_token: string;
  expires_in: number;
  user: User;
}

// Storage keys
const ACCESS_TOKEN_KEY = 'launchpad-access-token';
const REFRESH_TOKEN_KEY = 'launchpad-refresh-token';
const USER_KEY = 'launchpad-user';

// Get stored tokens
export function getAccessToken(): string | null {
  return localStorage.getItem(ACCESS_TOKEN_KEY);
}

export function getRefreshToken(): string | null {
  return localStorage.getItem(REFRESH_TOKEN_KEY);
}

// Store tokens
export function setTokens(accessToken: string, refreshToken: string): void {
  localStorage.setItem(ACCESS_TOKEN_KEY, accessToken);
  localStorage.setItem(REFRESH_TOKEN_KEY, refreshToken);
}

// Clear all auth data
export function clearTokens(): void {
  localStorage.removeItem(ACCESS_TOKEN_KEY);
  localStorage.removeItem(REFRESH_TOKEN_KEY);
  localStorage.removeItem(USER_KEY);
}

// Get stored user
export function getStoredUser(): User | null {
  const stored = localStorage.getItem(USER_KEY);
  return stored ? JSON.parse(stored) : null;
}

// Store user
export function setStoredUser(user: User): void {
  localStorage.setItem(USER_KEY, JSON.stringify(user));
}

// Login with username and password
export async function login(username: string, password: string): Promise<AuthResponse> {
  const response = await fetch('/api/auth/login', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ username, password }),
  });

  if (!response.ok) {
    const error = await response.text();
    throw new Error(error || 'Login failed');
  }

  const data: AuthResponse = await response.json();

  // Store tokens and user
  setTokens(data.access_token, data.refresh_token);
  setStoredUser(data.user);

  return data;
}

// Logout - clear tokens
export async function logout(): Promise<void> {
  const token = getAccessToken();

  // Try to notify server (optional, JWT logout is client-side)
  if (token) {
    try {
      await fetch('/api/auth/logout', {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${token}`,
        },
      });
    } catch {
      // Ignore errors - logout is primarily client-side for JWT
    }
  }

  clearTokens();
}

// Refresh access token using refresh token
export async function refreshAccessToken(): Promise<AuthResponse | null> {
  const refreshToken = getRefreshToken();

  if (!refreshToken) {
    return null;
  }

  try {
    const response = await fetch('/api/auth/refresh', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ refresh_token: refreshToken }),
    });

    if (!response.ok) {
      // Refresh token is invalid - clear everything
      clearTokens();
      return null;
    }

    const data: AuthResponse = await response.json();

    // Store new tokens
    setTokens(data.access_token, data.refresh_token);
    setStoredUser(data.user);

    return data;
  } catch {
    clearTokens();
    return null;
  }
}

// Get current user from server (validates token)
export async function getCurrentUser(): Promise<User | null> {
  const token = getAccessToken();

  if (!token) {
    return null;
  }

  try {
    const response = await fetch('/api/auth/me', {
      headers: {
        'Authorization': `Bearer ${token}`,
      },
    });

    if (!response.ok) {
      if (response.status === 401) {
        // Token expired, try to refresh
        const refreshed = await refreshAccessToken();
        if (refreshed) {
          return refreshed.user;
        }
      }
      clearTokens();
      return null;
    }

    const user: User = await response.json();
    setStoredUser(user);
    return user;
  } catch {
    return null;
  }
}

// Fetch wrapper with automatic token handling
export async function fetchWithAuth(
  url: string,
  options: RequestInit = {}
): Promise<Response> {
  let token = getAccessToken();

  if (!token) {
    throw new Error('Not authenticated');
  }

  // Add auth header
  const headers = new Headers(options.headers);
  headers.set('Authorization', `Bearer ${token}`);

  let response = await fetch(url, {
    ...options,
    headers,
  });

  // If unauthorized, try to refresh token and retry
  if (response.status === 401) {
    const refreshed = await refreshAccessToken();

    if (!refreshed) {
      throw new Error('Session expired');
    }

    // Retry with new token
    token = refreshed.access_token;
    headers.set('Authorization', `Bearer ${token}`);

    response = await fetch(url, {
      ...options,
      headers,
    });
  }

  return response;
}

// Check if user is authenticated (has valid tokens)
export function isAuthenticated(): boolean {
  return getAccessToken() !== null;
}

// Register a new user
export async function register(
  username: string,
  password: string,
  email?: string,
  displayName?: string
): Promise<AuthResponse> {
  const response = await fetch('/api/auth/register', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      username,
      password,
      email,
      display_name: displayName,
    }),
  });

  if (!response.ok) {
    const error = await response.text();
    throw new Error(error || 'Registration failed');
  }

  const data: AuthResponse = await response.json();

  // Store tokens and user if provided (auto-login)
  if (data.access_token) {
    setTokens(data.access_token, data.refresh_token);
    setStoredUser(data.user);
  }

  return data;
}

// Admin: Get settings
export async function getAdminSettings(): Promise<Record<string, unknown>> {
  const response = await fetchWithAuth('/api/admin/settings');
  if (!response.ok) {
    throw new Error('Failed to get settings');
  }
  return response.json();
}

// Admin: Update settings
export async function updateAdminSettings(
  settings: Record<string, string>
): Promise<void> {
  const response = await fetchWithAuth('/api/admin/settings', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(settings),
  });
  if (!response.ok) {
    throw new Error('Failed to update settings');
  }
}

// Admin user type
export interface AdminUser {
  id: string;
  username: string;
  email?: string;
  display_name?: string;
  roles: string[];
  created_at: string;
}

// Admin: List users
export async function listUsers(): Promise<AdminUser[]> {
  const response = await fetchWithAuth('/api/admin/users');
  if (!response.ok) {
    throw new Error('Failed to list users');
  }
  return response.json();
}

// Admin: Create user
export async function createUser(user: {
  username: string;
  password: string;
  email?: string;
  display_name?: string;
  roles?: string[];
}): Promise<{ id: string; username: string }> {
  const response = await fetchWithAuth('/api/admin/users', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(user),
  });
  if (!response.ok) {
    const error = await response.text();
    throw new Error(error || 'Failed to create user');
  }
  return response.json();
}

// Admin: Delete user
export async function deleteUser(id: string): Promise<void> {
  const response = await fetchWithAuth(`/api/admin/users/${id}`, {
    method: 'DELETE',
  });
  if (!response.ok) {
    const error = await response.text();
    throw new Error(error || 'Failed to delete user');
  }
}
