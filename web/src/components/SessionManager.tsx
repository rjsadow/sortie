import { useEffect, useCallback } from 'react';
import { useSessions } from '../hooks/useSessions';
import { formatDuration } from '../utils/time';
import type { Session, SessionStatus } from '../types';

interface SessionManagerProps {
  isOpen: boolean;
  onClose: () => void;
  onReconnect: (appId: string, sessionId: string) => void;
  darkMode: boolean;
}

const STATUS_COLORS: Record<SessionStatus, { bg: string; text: string; pulse?: boolean }> = {
  creating: { bg: 'bg-blue-500', text: 'text-blue-500', pulse: true },
  running: { bg: 'bg-green-500', text: 'text-green-500', pulse: true },
  failed: { bg: 'bg-red-500', text: 'text-red-500' },
  stopped: { bg: 'bg-gray-400', text: 'text-gray-400' },
  expired: { bg: 'bg-gray-400', text: 'text-gray-400' },
};

const STATUS_LABELS: Record<SessionStatus, string> = {
  creating: 'Creating',
  running: 'Running',
  failed: 'Failed',
  stopped: 'Stopped',
  expired: 'Expired',
};

function SessionCard({
  session,
  onReconnect,
  onTerminate,
  darkMode,
}: {
  session: Session;
  onReconnect: () => void;
  onTerminate: () => void;
  darkMode: boolean;
}) {
  const statusConfig = STATUS_COLORS[session.status];
  const isRunning = session.status === 'running';
  const isCreating = session.status === 'creating';
  const canReconnect = isRunning;

  const cardBg = darkMode ? 'bg-gray-800' : 'bg-white';
  const borderColor = darkMode ? 'border-gray-700' : 'border-gray-200';
  const textColor = darkMode ? 'text-gray-100' : 'text-gray-900';
  const mutedText = darkMode ? 'text-gray-400' : 'text-gray-600';

  return (
    <div className={`${cardBg} rounded-lg border ${borderColor} p-4`}>
      <div className="flex items-start gap-3">
        {/* Status indicator */}
        <div className="flex-shrink-0 mt-1">
          <div
            className={`w-3 h-3 rounded-full ${statusConfig.bg} ${
              statusConfig.pulse ? 'animate-pulse' : ''
            }`}
          />
        </div>

        {/* Session info */}
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <h3 className={`font-medium ${textColor} truncate`}>
              {session.app_name || session.app_id}
            </h3>
            <span
              className={`text-xs px-2 py-0.5 rounded-full ${
                darkMode ? 'bg-gray-700' : 'bg-gray-100'
              } ${statusConfig.text}`}
            >
              {STATUS_LABELS[session.status]}
            </span>
          </div>
          <p className={`text-sm ${mutedText} mt-1`}>
            Started {formatDuration(session.created_at)}
          </p>
        </div>
      </div>

      {/* Actions */}
      <div className="flex gap-2 mt-4">
        {canReconnect ? (
          <button
            onClick={onReconnect}
            className="flex-1 px-3 py-2 text-sm font-medium text-white bg-brand-primary hover:bg-brand-secondary rounded-lg transition-colors"
          >
            Reconnect
          </button>
        ) : isCreating ? (
          <button
            disabled
            className={`flex-1 px-3 py-2 text-sm font-medium rounded-lg ${
              darkMode ? 'bg-gray-700 text-gray-500' : 'bg-gray-100 text-gray-400'
            } cursor-not-allowed`}
          >
            Waiting...
          </button>
        ) : null}
        <button
          onClick={onTerminate}
          className={`px-3 py-2 text-sm font-medium rounded-lg transition-colors ${
            darkMode
              ? 'bg-gray-700 text-red-400 hover:bg-red-900/30'
              : 'bg-gray-100 text-red-600 hover:bg-red-50'
          }`}
        >
          Terminate
        </button>
      </div>
    </div>
  );
}

export function SessionManager({
  isOpen,
  onClose,
  onReconnect,
  darkMode,
}: SessionManagerProps) {
  const { sessions, isLoading, error, refresh, terminateSession } = useSessions(isOpen);

  // Handle escape key
  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if (e.key === 'Escape' && isOpen) {
        onClose();
      }
    },
    [isOpen, onClose]
  );

  useEffect(() => {
    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [handleKeyDown]);

  // Refresh when panel opens
  useEffect(() => {
    if (isOpen) {
      refresh();
    }
  }, [isOpen, refresh]);

  const handleTerminate = async (session: Session) => {
    if (!confirm(`Are you sure you want to terminate the session for "${session.app_name || session.app_id}"?`)) {
      return;
    }
    await terminateSession(session.id);
  };

  if (!isOpen) return null;

  const bgColor = darkMode ? 'bg-gray-900' : 'bg-gray-50';
  const headerBg = darkMode ? 'bg-gray-800' : 'bg-white';
  const textColor = darkMode ? 'text-gray-100' : 'text-gray-900';
  const mutedText = darkMode ? 'text-gray-400' : 'text-gray-600';
  const borderColor = darkMode ? 'border-gray-700' : 'border-gray-200';

  // Filter to show only active sessions (creating or running)
  const activeSessions = sessions.filter(
    (s) => s.status === 'creating' || s.status === 'running'
  );

  return (
    <>
      {/* Backdrop */}
      <div
        className="fixed inset-0 bg-black/50 z-40"
        onClick={onClose}
        aria-hidden="true"
      />

      {/* Slide-over panel */}
      <div
        className={`fixed inset-y-0 right-0 w-full max-w-md ${bgColor} shadow-xl z-50 flex flex-col`}
        role="dialog"
        aria-modal="true"
        aria-labelledby="session-manager-title"
      >
        {/* Header */}
        <div className={`flex items-center justify-between px-4 py-4 ${headerBg} border-b ${borderColor}`}>
          <h2 id="session-manager-title" className={`text-lg font-semibold ${textColor}`}>
            Sessions
          </h2>
          <div className="flex items-center gap-2">
            <button
              onClick={() => refresh()}
              className={`p-2 rounded-lg ${darkMode ? 'hover:bg-gray-700' : 'hover:bg-gray-100'} transition-colors`}
              aria-label="Refresh sessions"
              title="Refresh"
            >
              <svg
                className={`w-5 h-5 ${mutedText} ${isLoading ? 'animate-spin' : ''}`}
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
                />
              </svg>
            </button>
            <button
              onClick={onClose}
              className={`p-2 rounded-lg ${darkMode ? 'hover:bg-gray-700' : 'hover:bg-gray-100'} transition-colors`}
              aria-label="Close session manager"
            >
              <svg className={`w-5 h-5 ${textColor}`} fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
        </div>

        {/* Content */}
        <div className="flex-1 overflow-y-auto p-4">
          {error && (
            <div className="mb-4 p-3 bg-red-500/20 border border-red-500 rounded-lg text-red-500 text-sm">
              {error}
            </div>
          )}

          {isLoading && activeSessions.length === 0 ? (
            <div className="flex justify-center py-12">
              <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-brand-primary"></div>
            </div>
          ) : activeSessions.length === 0 ? (
            <div className="text-center py-12">
              <svg
                className={`mx-auto h-12 w-12 ${mutedText}`}
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"
                />
              </svg>
              <h3 className={`mt-2 text-sm font-medium ${textColor}`}>No active sessions</h3>
              <p className={`mt-1 text-sm ${mutedText}`}>
                Launch an application to create a session.
              </p>
            </div>
          ) : (
            <div className="space-y-3">
              {activeSessions.map((session) => (
                <SessionCard
                  key={session.id}
                  session={session}
                  onReconnect={() => onReconnect(session.app_id, session.id)}
                  onTerminate={() => handleTerminate(session)}
                  darkMode={darkMode}
                />
              ))}
            </div>
          )}
        </div>

        {/* Footer with count */}
        {activeSessions.length > 0 && (
          <div className={`px-4 py-3 ${headerBg} border-t ${borderColor}`}>
            <p className={`text-sm ${mutedText}`}>
              {activeSessions.length} active session{activeSessions.length !== 1 ? 's' : ''}
            </p>
          </div>
        )}
      </div>
    </>
  );
}
