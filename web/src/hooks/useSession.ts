import { useState, useCallback, useRef, useEffect } from 'react';
import type { Session, CreateSessionRequest } from '../types';
import { fetchWithAuth } from '../services/auth';

interface UseSessionReturn {
  session: Session | null;
  isLoading: boolean;
  error: string | null;
  createSession: (appId: string, screenWidth?: number, screenHeight?: number) => Promise<Session | null>;
  reconnectToSession: (sessionId: string) => Promise<Session | null>;
  terminateSession: () => Promise<void>;
  connectWebSocket: () => WebSocket | null;
  disconnectWebSocket: () => void;
  wsConnected: boolean;
}

const POLL_INTERVAL = 2000; // 2 seconds
const MAX_RETRIES = 3;

export function useSession(): UseSessionReturn {
  const [session, setSession] = useState<Session | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [wsConnected, setWsConnected] = useState(false);

  const wsRef = useRef<WebSocket | null>(null);
  const pollIntervalRef = useRef<number | null>(null);
  const retriesRef = useRef(0);

  // Cleanup function
  const cleanup = useCallback(() => {
    if (pollIntervalRef.current) {
      clearInterval(pollIntervalRef.current);
      pollIntervalRef.current = null;
    }
    if (wsRef.current) {
      wsRef.current.close();
      wsRef.current = null;
    }
    setWsConnected(false);
  }, []);

  // Poll session status until ready
  const pollSessionStatus = useCallback(async (sessionId: string): Promise<Session | null> => {
    try {
      const response = await fetchWithAuth(`/api/sessions/${sessionId}`);
      if (!response.ok) {
        throw new Error(`Failed to get session: ${response.statusText}`);
      }
      const data: Session = await response.json();
      setSession(data);
      return data;
    } catch (err) {
      console.error('Error polling session:', err);
      return null;
    }
  }, []);

  // Start polling for session status
  const startPolling = useCallback((sessionId: string) => {
    if (pollIntervalRef.current) {
      clearInterval(pollIntervalRef.current);
    }

    pollIntervalRef.current = window.setInterval(async () => {
      const updatedSession = await pollSessionStatus(sessionId);
      if (updatedSession) {
        // Stop polling if session is in a terminal state
        if (['running', 'stopped', 'expired', 'failed'].includes(updatedSession.status)) {
          if (pollIntervalRef.current) {
            clearInterval(pollIntervalRef.current);
            pollIntervalRef.current = null;
          }
        }
      }
    }, POLL_INTERVAL);
  }, [pollSessionStatus]);

  // Create a new session
  const createSession = useCallback(async (appId: string, screenWidth?: number, screenHeight?: number): Promise<Session | null> => {
    setIsLoading(true);
    setError(null);
    cleanup();

    try {
      const request: CreateSessionRequest = {
        app_id: appId,
        screen_width: screenWidth,
        screen_height: screenHeight,
      };

      const response = await fetchWithAuth('/api/sessions', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(request),
      });

      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(errorText || `Failed to create session: ${response.statusText}`);
      }

      const data: Session = await response.json();
      setSession(data);

      // Start polling if session is not yet running
      if (data.status !== 'running') {
        startPolling(data.id);
      }

      return data;
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to create session';
      setError(message);
      return null;
    } finally {
      setIsLoading(false);
    }
  }, [cleanup, startPolling]);

  // Reconnect to an existing session
  const reconnectToSession = useCallback(async (sessionId: string): Promise<Session | null> => {
    setIsLoading(true);
    setError(null);
    cleanup();

    try {
      const response = await fetchWithAuth(`/api/sessions/${sessionId}`);
      if (!response.ok) {
        throw new Error(`Failed to get session: ${response.statusText}`);
      }

      const data: Session = await response.json();
      setSession(data);

      // Start polling if session is not yet running
      if (data.status === 'creating') {
        startPolling(data.id);
      }

      return data;
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to reconnect to session';
      setError(message);
      return null;
    } finally {
      setIsLoading(false);
    }
  }, [cleanup, startPolling]);

  // Terminate the current session
  const terminateSession = useCallback(async () => {
    if (!session) return;

    cleanup();
    setIsLoading(true);
    setError(null);

    try {
      const response = await fetchWithAuth(`/api/sessions/${session.id}`, {
        method: 'DELETE',
      });

      if (!response.ok && response.status !== 404) {
        throw new Error(`Failed to terminate session: ${response.statusText}`);
      }

      setSession(null);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to terminate session';
      setError(message);
    } finally {
      setIsLoading(false);
    }
  }, [session, cleanup]);

  // Connect to WebSocket for VNC streaming
  const connectWebSocket = useCallback((): WebSocket | null => {
    if (!session || session.status !== 'running' || !session.websocket_url) {
      setError('Session is not ready for WebSocket connection');
      return null;
    }

    // Close existing connection
    if (wsRef.current) {
      wsRef.current.close();
    }

    // Build WebSocket URL
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}${session.websocket_url}`;

    try {
      const ws = new WebSocket(wsUrl);

      ws.onopen = () => {
        console.log('WebSocket connected');
        setWsConnected(true);
        retriesRef.current = 0;
      };

      ws.onclose = (event) => {
        console.log('WebSocket closed:', event.code, event.reason);
        setWsConnected(false);

        // Auto-reconnect if not a normal closure
        if (event.code !== 1000 && retriesRef.current < MAX_RETRIES) {
          retriesRef.current++;
          console.log(`Attempting reconnect (${retriesRef.current}/${MAX_RETRIES})...`);
          setTimeout(() => connectWebSocket(), 1000 * retriesRef.current);
        }
      };

      ws.onerror = (event) => {
        console.error('WebSocket error:', event);
        setError('WebSocket connection error');
      };

      wsRef.current = ws;
      return ws;
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to connect WebSocket';
      setError(message);
      return null;
    }
  }, [session]);

  // Disconnect WebSocket
  const disconnectWebSocket = useCallback(() => {
    if (wsRef.current) {
      wsRef.current.close(1000, 'User disconnected');
      wsRef.current = null;
    }
    setWsConnected(false);
  }, []);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      cleanup();
    };
  }, [cleanup]);

  return {
    session,
    isLoading,
    error,
    createSession,
    reconnectToSession,
    terminateSession,
    connectWebSocket,
    disconnectWebSocket,
    wsConnected,
  };
}
