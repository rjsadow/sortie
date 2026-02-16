import { useEffect, useRef, useCallback } from 'react';
import { getAccessToken } from '../services/auth';

interface UseSessionEventsOptions {
  onEvent: () => void;
  onConnected: () => void;
  onDisconnected: () => void;
}

export function useSessionEvents({
  onEvent,
  onConnected,
  onDisconnected,
}: UseSessionEventsOptions) {
  const esRef = useRef<EventSource | null>(null);
  const retryTimerRef = useRef<number | null>(null);

  // Use refs for the callbacks so the EventSource doesn't need to
  // be recreated when the callbacks change.
  const onEventRef = useRef(onEvent);
  const onConnectedRef = useRef(onConnected);
  const onDisconnectedRef = useRef(onDisconnected);

  useEffect(() => {
    onEventRef.current = onEvent;
  }, [onEvent]);
  useEffect(() => {
    onConnectedRef.current = onConnected;
  }, [onConnected]);
  useEffect(() => {
    onDisconnectedRef.current = onDisconnected;
  }, [onDisconnected]);

  const connect = useCallback(() => {
    const token = getAccessToken();
    if (!token) return;

    const es = new EventSource(`/api/sessions/events?token=${encodeURIComponent(token)}`);
    esRef.current = es;

    es.addEventListener('connected', () => {
      onConnectedRef.current();
    });

    es.addEventListener('session', () => {
      onEventRef.current();
    });

    es.onerror = () => {
      es.close();
      esRef.current = null;
      onDisconnectedRef.current();

      // Retry after 5 seconds
      retryTimerRef.current = window.setTimeout(() => {
        retryTimerRef.current = null;
        connect();
      }, 5000);
    };
  }, []);

  useEffect(() => {
    connect();

    return () => {
      if (esRef.current) {
        esRef.current.close();
        esRef.current = null;
      }
      if (retryTimerRef.current !== null) {
        clearTimeout(retryTimerRef.current);
        retryTimerRef.current = null;
      }
    };
  }, [connect]);
}
