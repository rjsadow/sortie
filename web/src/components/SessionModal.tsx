import { useEffect, useState, useCallback, useMemo } from 'react';
import { useSession } from '../hooks/useSession';
import { VNCViewer } from './VNCViewer';
import type { Application } from '../types';

interface SessionModalProps {
  app: Application;
  isOpen: boolean;
  onClose: () => void;
  darkMode: boolean;
}

type ConnectionState = 'idle' | 'creating' | 'waiting' | 'connecting' | 'connected' | 'error';

export function SessionModal({ app, isOpen, onClose, darkMode }: SessionModalProps) {
  const { session, isLoading, error, createSession, terminateSession } = useSession();
  const [vncConnectionState, setVncConnectionState] = useState<'idle' | 'connected' | 'error'>('idle');
  const [vncErrorMessage, setVncErrorMessage] = useState('');
  const [sessionCreationStarted, setSessionCreationStarted] = useState(false);

  // Create session when modal opens
  useEffect(() => {
    if (isOpen && !session && !sessionCreationStarted) {
      setSessionCreationStarted(true);
      createSession(app.id);
    }
  }, [isOpen, session, sessionCreationStarted, app.id, createSession]);

  // Reset state when modal closes
  useEffect(() => {
    if (!isOpen) {
      setSessionCreationStarted(false);
      setVncConnectionState('idle');
      setVncErrorMessage('');
    }
  }, [isOpen]);

  // Derive connection state and status message from session
  const { connectionState, statusMessage } = useMemo((): { connectionState: ConnectionState; statusMessage: string } => {
    // VNC-level errors take precedence
    if (vncConnectionState === 'error') {
      return { connectionState: 'error', statusMessage: vncErrorMessage || 'Connection error' };
    }
    if (vncConnectionState === 'connected') {
      return { connectionState: 'connected', statusMessage: '' };
    }

    // No session yet - we're creating
    if (!session) {
      if (sessionCreationStarted || isLoading) {
        return { connectionState: 'creating', statusMessage: 'Creating session...' };
      }
      return { connectionState: 'idle', statusMessage: '' };
    }

    // Derive from session status
    switch (session.status) {
      case 'pending':
      case 'creating':
        return { connectionState: 'waiting', statusMessage: 'Starting container...' };
      case 'running':
        if (session.websocket_url) {
          return { connectionState: 'connecting', statusMessage: 'Connecting to display...' };
        }
        return { connectionState: 'waiting', statusMessage: 'Waiting for container...' };
      case 'failed':
        return { connectionState: 'error', statusMessage: error || 'Session failed to start' };
      case 'terminated':
        return { connectionState: 'idle', statusMessage: '' };
      default:
        return { connectionState: 'idle', statusMessage: '' };
    }
  }, [session, sessionCreationStarted, vncConnectionState, vncErrorMessage, isLoading, error]);

  // Handle close
  const handleClose = useCallback(async () => {
    if (session && session.status !== 'terminated' && session.status !== 'failed') {
      await terminateSession();
    }
    onClose();
  }, [session, terminateSession, onClose]);

  // Handle VNC connection events
  const handleVNCConnect = useCallback(() => {
    setVncConnectionState('connected');
  }, []);

  const handleVNCDisconnect = useCallback((clean: boolean) => {
    if (!clean && vncConnectionState === 'connected') {
      setVncConnectionState('error');
      setVncErrorMessage('Connection lost');
    }
  }, [vncConnectionState]);

  const handleVNCError = useCallback((message: string) => {
    setVncConnectionState('error');
    setVncErrorMessage(message);
  }, []);

  if (!isOpen) return null;

  const bgColor = darkMode ? 'bg-gray-800' : 'bg-white';
  const textColor = darkMode ? 'text-gray-100' : 'text-gray-900';
  const borderColor = darkMode ? 'border-gray-700' : 'border-gray-200';
  const overlayColor = 'bg-black/50';

  return (
    <div className={`fixed inset-0 z-50 flex items-center justify-center ${overlayColor}`}>
      <div className={`relative w-full max-w-6xl h-[90vh] mx-4 rounded-lg shadow-xl ${bgColor} flex flex-col`}>
        {/* Header */}
        <div className={`flex items-center justify-between px-4 py-3 border-b ${borderColor}`}>
          <div className="flex items-center gap-3">
            {app.icon && (
              <span className="text-2xl">{app.icon}</span>
            )}
            <div>
              <h2 className={`text-lg font-semibold ${textColor}`}>{app.name}</h2>
              {connectionState === 'connected' && (
                <span className="text-xs text-green-500">Connected</span>
              )}
            </div>
          </div>
          <button
            onClick={handleClose}
            className={`p-2 rounded-lg hover:bg-gray-200 dark:hover:bg-gray-700 transition-colors ${textColor}`}
            aria-label="Close session"
          >
            <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        {/* Content */}
        <div className="flex-1 relative overflow-hidden">
          {/* Loading/Status overlay */}
          {connectionState !== 'connected' && (
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
                  {/* Loading spinner */}
                  <div className="mb-4">
                    <svg className="w-12 h-12 animate-spin text-blue-500" fill="none" viewBox="0 0 24 24">
                      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                      <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
                    </svg>
                  </div>
                  <p className={`text-lg ${textColor}`}>{statusMessage || 'Loading...'}</p>
                  {isLoading && (
                    <p className={`text-sm mt-2 ${darkMode ? 'text-gray-400' : 'text-gray-500'}`}>
                      This may take a moment...
                    </p>
                  )}
                </>
              )}
            </div>
          )}

          {/* VNC Viewer */}
          {session?.websocket_url && connectionState !== 'error' && (
            <VNCViewer
              wsUrl={session.websocket_url}
              onConnect={handleVNCConnect}
              onDisconnect={handleVNCDisconnect}
              onError={handleVNCError}
            />
          )}
        </div>

        {/* Footer */}
        <div className={`px-4 py-2 border-t ${borderColor} flex items-center justify-between`}>
          <div className={`text-sm ${darkMode ? 'text-gray-400' : 'text-gray-500'}`}>
            {session && (
              <span>Session: {session.id.slice(0, 8)}...</span>
            )}
          </div>
          <button
            onClick={handleClose}
            className={`px-4 py-2 rounded-lg transition-colors ${
              darkMode
                ? 'bg-gray-700 hover:bg-gray-600 text-gray-200'
                : 'bg-gray-200 hover:bg-gray-300 text-gray-800'
            }`}
          >
            End Session
          </button>
        </div>
      </div>
    </div>
  );
}
