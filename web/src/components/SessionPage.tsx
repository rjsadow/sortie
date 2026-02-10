import { useEffect, useState, useCallback, useMemo } from 'react';
import { useSession } from '../hooks/useSession';
import { SessionViewer } from './SessionViewer';
import type { Application, ClipboardPolicy } from '../types';

interface SessionPageProps {
  app: Application;
  onClose: () => void;
  darkMode: boolean;
  sessionId?: string; // If provided, reconnect to this existing session instead of creating a new one
  clipboardPolicy?: ClipboardPolicy;
}

type ConnectionState = 'idle' | 'creating' | 'waiting' | 'connecting' | 'connected' | 'error';

export function SessionPage({ app, onClose, darkMode, sessionId, clipboardPolicy = 'bidirectional' }: SessionPageProps) {
  const { session, isLoading, error, createSession, reconnectToSession, terminateSession } = useSession();
  const [viewerConnectionState, setViewerConnectionState] = useState<'idle' | 'connected' | 'error'>('idle');
  const [viewerErrorMessage, setViewerErrorMessage] = useState('');
  const [sessionCreationStarted, setSessionCreationStarted] = useState(false);

  // Handle close - defined early so it can be used in effects below
  const handleClose = useCallback(async () => {
    // Only terminate if not already in a terminal state AND this is not a reconnection
    const terminalStates = ['stopped', 'expired', 'failed'];
    if (session && !terminalStates.includes(session.status) && !sessionId) {
      await terminateSession();
    }
    if (window.history.state?.sessionPage) {
      window.history.back();
    }
    onClose();
  }, [session, sessionId, terminateSession, onClose]);

  // Create or reconnect to session when page mounts
  useEffect(() => {
    if (!session && !sessionCreationStarted) {
      setSessionCreationStarted(true);
      if (sessionId) {
        reconnectToSession(sessionId);
      } else {
        const screenWidth = window.innerWidth;
        const screenHeight = window.innerHeight - 48;
        createSession(app.id, screenWidth, screenHeight);
      }
    }
  }, [session, sessionCreationStarted, app.id, sessionId, createSession, reconnectToSession]);

  // Handle browser back button via History API
  useEffect(() => {
    window.history.pushState({ sessionPage: true }, '');

    const handlePopState = (event: PopStateEvent) => {
      if (!event.state?.sessionPage) {
        handleClose();
      }
    };

    window.addEventListener('popstate', handlePopState);
    return () => {
      window.removeEventListener('popstate', handlePopState);
    };
  }, [handleClose]);

  // Handle Escape key to close
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        handleClose();
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [handleClose]);

  // Derive connection state and status message from session
  const { connectionState, statusMessage } = useMemo((): { connectionState: ConnectionState; statusMessage: string } => {
    if (viewerConnectionState === 'error') {
      return { connectionState: 'error', statusMessage: viewerErrorMessage || 'Connection error' };
    }
    if (viewerConnectionState === 'connected') {
      return { connectionState: 'connected', statusMessage: '' };
    }

    if (!session) {
      if (sessionCreationStarted || isLoading) {
        return { connectionState: 'creating', statusMessage: 'Creating session...' };
      }
      return { connectionState: 'idle', statusMessage: '' };
    }

    switch (session.status) {
      case 'creating':
        return { connectionState: 'waiting', statusMessage: 'Starting container...' };
      case 'running':
        if (session.websocket_url || session.guacamole_url || session.proxy_url) {
          return { connectionState: 'connecting', statusMessage: 'Connecting to display...' };
        }
        return { connectionState: 'waiting', statusMessage: 'Waiting for container...' };
      case 'failed':
        return { connectionState: 'error', statusMessage: error || 'Session failed to start' };
      case 'stopped':
      case 'expired':
        return { connectionState: 'idle', statusMessage: '' };
      default:
        return { connectionState: 'idle', statusMessage: '' };
    }
  }, [session, sessionCreationStarted, viewerConnectionState, viewerErrorMessage, isLoading, error]);

  const handleViewerConnect = useCallback(() => {
    setViewerConnectionState('connected');
  }, []);

  const handleViewerDisconnect = useCallback((clean: boolean) => {
    if (!clean && viewerConnectionState === 'connected') {
      setViewerConnectionState('error');
      setViewerErrorMessage('Connection lost');
    }
  }, [viewerConnectionState]);

  const handleViewerError = useCallback((message: string) => {
    setViewerConnectionState('error');
    setViewerErrorMessage(message);
  }, []);

  const bgColor = darkMode ? 'bg-gray-900' : 'bg-gray-100';
  const headerBgColor = darkMode ? 'bg-gray-800' : 'bg-white';
  const textColor = darkMode ? 'text-gray-100' : 'text-gray-900';
  const subtextColor = darkMode ? 'text-gray-400' : 'text-gray-500';
  const borderColor = darkMode ? 'border-gray-700' : 'border-gray-200';

  return (
    <div className={`fixed inset-0 z-50 flex flex-col ${bgColor}`}>
      {/* Header - 48px */}
      <header className={`flex-shrink-0 h-12 flex items-center justify-between px-4 ${headerBgColor} border-b ${borderColor}`}>
        {/* Left side - Back button and app info */}
        <div className="flex items-center gap-3">
          <button
            onClick={handleClose}
            className={`flex items-center gap-1 px-2 py-1 rounded-xl hover:bg-gray-200 dark:hover:bg-gray-700 transition-colors ${textColor}`}
            aria-label="Back to dashboard"
          >
            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
            </svg>
            <span className="text-sm font-medium hidden sm:inline">Back</span>
          </button>

          <div className={`h-6 w-px ${darkMode ? 'bg-gray-700' : 'bg-gray-300'}`} />

          <div className="flex items-center gap-2">
            {app.icon && (
              <img
                src={app.icon}
                alt={`${app.name} icon`}
                className="w-6 h-6 object-contain"
                onError={(e) => {
                  (e.target as HTMLImageElement).src =
                    'data:image/svg+xml,<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="%23636A51"><rect width="24" height="24" rx="4"/><text x="12" y="16" text-anchor="middle" fill="white" font-size="12">' +
                    app.name.charAt(0) +
                    '</text></svg>';
                }}
              />
            )}
            <span className={`font-medium ${textColor}`}>{app.name}</span>
          </div>
        </div>

        {/* Center - Connection status */}
        <div className="flex items-center gap-2">
          {connectionState === 'connected' ? (
            <>
              <span className="w-2 h-2 bg-green-500 rounded-full animate-pulse" />
              <span className={`text-sm ${subtextColor}`}>Connected</span>
            </>
          ) : connectionState === 'error' ? (
            <>
              <span className="w-2 h-2 bg-red-500 rounded-full" />
              <span className="text-sm text-red-500">Error</span>
            </>
          ) : (
            <>
              <svg className="w-4 h-4 animate-spin text-blue-500" fill="none" viewBox="0 0 24 24">
                <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
              </svg>
              <span className={`text-sm ${subtextColor}`}>{statusMessage || 'Connecting...'}</span>
            </>
          )}
        </div>

        {/* Right side - Close button */}
        <div className="flex items-center gap-1">
          <button
            onClick={handleClose}
            className={`p-2 rounded-lg hover:bg-gray-200 dark:hover:bg-gray-700 transition-colors ${textColor}`}
            aria-label="Close session"
            title="Close session (Esc)"
          >
            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
      </header>

      {/* Content - fills remaining space */}
      <div className="flex-1 relative overflow-hidden">
        {/* Loading/Status overlay (shown when not yet connected to a viewer) */}
        {connectionState !== 'connected' && !session?.websocket_url && !session?.guacamole_url && !session?.proxy_url && (
          <div className={`absolute inset-0 flex flex-col items-center justify-center ${bgColor} z-10`}>
            {connectionState === 'error' ? (
              <>
                <div className="text-red-500 mb-4">
                  <svg className="w-16 h-16" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                  </svg>
                </div>
                <p className={`text-lg ${textColor}`}>{error || statusMessage}</p>
                <button
                  onClick={handleClose}
                  className="mt-4 px-4 py-2 bg-red-500 text-white rounded-lg hover:bg-red-600 transition-colors"
                >
                  Close
                </button>
              </>
            ) : (
              <>
                <div className="mb-4">
                  <svg className="w-12 h-12 animate-spin text-blue-500" fill="none" viewBox="0 0 24 24">
                    <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                    <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
                  </svg>
                </div>
                <p className={`text-lg ${textColor}`}>{statusMessage || 'Loading...'}</p>
                {isLoading && (
                  <p className={`text-sm mt-2 ${subtextColor}`}>
                    This may take a moment...
                  </p>
                )}
              </>
            )}
          </div>
        )}

        {/* SessionViewer - renders when session has a viewer URL */}
        {session && (session.websocket_url || session.guacamole_url || session.proxy_url) && connectionState !== 'error' && (
          <SessionViewer
            session={session}
            app={app}
            darkMode={darkMode}
            clipboardPolicy={clipboardPolicy}
            onConnect={handleViewerConnect}
            onDisconnect={handleViewerDisconnect}
            onError={handleViewerError}
          />
        )}
      </div>
    </div>
  );
}
