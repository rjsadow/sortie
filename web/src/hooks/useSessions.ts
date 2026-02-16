import { useState, useEffect, useCallback, useRef } from 'react';
import type { Session } from '../types';
import { fetchWithAuth } from '../services/auth';
import { useSessionEvents } from './useSessionEvents';

interface UseSessionsReturn {
  sessions: Session[];
  isLoading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
  terminateSession: (id: string) => Promise<boolean>;
}

const POLL_INTERVAL_SSE = 120000; // 120s safety net when SSE is connected
const POLL_INTERVAL_NO_SSE = 30000; // 30s when SSE is disconnected

export function useSessions(autoRefresh = false): UseSessionsReturn {
  const [sessions, setSessions] = useState<Session[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [sseConnected, setSseConnected] = useState(false);
  const refreshIntervalRef = useRef<number | null>(null);

  const fetchSessions = useCallback(async () => {
    try {
      const [ownRes, sharedRes] = await Promise.all([
        fetchWithAuth('/api/sessions'),
        fetchWithAuth('/api/sessions/shared'),
      ]);
      if (!ownRes.ok) {
        throw new Error(`Failed to fetch sessions: ${ownRes.statusText}`);
      }
      const ownData: Session[] = await ownRes.json();
      let sharedData: Session[] = [];
      if (sharedRes.ok) {
        sharedData = await sharedRes.json();
      }
      // Merge, deduplicating by ID (own sessions take priority)
      const ownIds = new Set(ownData.map((s) => s.id));
      const merged = [...ownData, ...sharedData.filter((s) => !ownIds.has(s.id))];
      setSessions(merged);
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

  // SSE integration: re-fetch on any session lifecycle event
  useSessionEvents({
    onEvent: fetchSessions,
    onConnected: useCallback(() => setSseConnected(true), []),
    onDisconnected: useCallback(() => setSseConnected(false), []),
  });

  // Initial fetch
  useEffect(() => {
    fetchSessions();
  }, [fetchSessions]);

  // Adaptive polling: faster when SSE is disconnected, slower safety-net when connected
  useEffect(() => {
    if (autoRefresh) {
      const interval = sseConnected ? POLL_INTERVAL_SSE : POLL_INTERVAL_NO_SSE;
      refreshIntervalRef.current = window.setInterval(fetchSessions, interval);
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
  }, [autoRefresh, sseConnected, fetchSessions]);

  return {
    sessions,
    isLoading,
    error,
    refresh,
    terminateSession,
  };
}
