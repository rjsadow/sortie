import { useState, useEffect, useCallback, useRef } from 'react';
import type { Session } from '../types';
import { fetchWithAuth } from '../services/auth';

interface UseSessionsReturn {
  sessions: Session[];
  isLoading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
  terminateSession: (id: string) => Promise<boolean>;
}

const REFRESH_INTERVAL = 30000; // 30 seconds

export function useSessions(autoRefresh = false): UseSessionsReturn {
  const [sessions, setSessions] = useState<Session[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const refreshIntervalRef = useRef<number | null>(null);

  const fetchSessions = useCallback(async () => {
    try {
      const response = await fetchWithAuth('/api/sessions');
      if (!response.ok) {
        throw new Error(`Failed to fetch sessions: ${response.statusText}`);
      }
      const data: Session[] = await response.json();
      setSessions(data);
      setError(null);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to fetch sessions';
      setError(message);
    } finally {
      setIsLoading(false);
    }
  }, []);

  const refresh = useCallback(async () => {
    setIsLoading(true);
    await fetchSessions();
  }, [fetchSessions]);

  const terminateSession = useCallback(async (id: string): Promise<boolean> => {
    try {
      const response = await fetchWithAuth(`/api/sessions/${id}`, {
        method: 'DELETE',
      });

      if (!response.ok && response.status !== 404) {
        throw new Error(`Failed to terminate session: ${response.statusText}`);
      }

      // Optimistically remove the session from the list
      setSessions((prev) => prev.filter((s) => s.id !== id));
      return true;
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to terminate session';
      setError(message);
      return false;
    }
  }, []);

  // Initial fetch
  useEffect(() => {
    fetchSessions();
  }, [fetchSessions]);

  // Auto-refresh when enabled
  useEffect(() => {
    if (autoRefresh) {
      refreshIntervalRef.current = window.setInterval(fetchSessions, REFRESH_INTERVAL);
    } else if (refreshIntervalRef.current) {
      clearInterval(refreshIntervalRef.current);
      refreshIntervalRef.current = null;
    }

    return () => {
      if (refreshIntervalRef.current) {
        clearInterval(refreshIntervalRef.current);
        refreshIntervalRef.current = null;
      }
    };
  }, [autoRefresh, fetchSessions]);

  return {
    sessions,
    isLoading,
    error,
    refresh,
    terminateSession,
  };
}
