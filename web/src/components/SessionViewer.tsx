import { useState, useCallback, useRef, useEffect } from 'react';
import { VNCViewer } from './VNCViewer';
import { GuacamoleViewer } from './GuacamoleViewer';
import { useRecording } from '../hooks/useRecording';
import type { Session, Application, ClipboardPolicy } from '../types';

type ViewerState = 'connecting' | 'connected' | 'reconnecting' | 'error';

interface SessionViewerProps {
  session: Session;
  app: Application;
  darkMode: boolean;
  clipboardPolicy?: ClipboardPolicy;
  viewOnly?: boolean;
  showStats?: boolean;
  onConnect?: () => void;
  onDisconnect?: (clean: boolean) => void;
  onError?: (message: string) => void;
}

const CLIPBOARD_LABELS: Record<ClipboardPolicy, string> = {
  none: 'Clipboard disabled',
  read: 'Copy from remote only',
  write: 'Paste to remote only',
  bidirectional: 'Clipboard sync enabled',
};

export function SessionViewer({
  session,
  app,
  darkMode,
  clipboardPolicy = 'bidirectional',
  viewOnly = false,
  showStats: showStatsProp = false,
  onConnect,
  onDisconnect,
  onError,
}: SessionViewerProps) {
  const viewerContainerRef = useRef<HTMLDivElement>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const [viewerState, setViewerState] = useState<ViewerState>('connecting');
  const [reconnectInfo, setReconnectInfo] = useState<{ attempt: number; max: number } | null>(null);
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [showStats, setShowStats] = useState(showStatsProp);
  const [showClipboardToast, setShowClipboardToast] = useState(false);
  const [errorMessage, setErrorMessage] = useState('');
  const [hasWs, setHasWs] = useState(false);
  const { isRecording, duration: recordingDuration, attachWebSocket, startRecording, stopRecording, error: recordingError } = useRecording();

  const handleWebSocketReady = useCallback((ws: WebSocket) => {
    wsRef.current = ws;
    // Start passive capture immediately so the VNC handshake is included
    // in any future recording. Use a default size; it will be updated once
    // the canvas is available, but the handshake capture starts right away.
    const canvas = viewerContainerRef.current?.querySelector('canvas');
    const w = canvas?.width || 1024;
    const h = canvas?.height || 768;
    attachWebSocket(ws, w, h);
    setHasWs(true);
  }, [attachWebSocket]);

  const toggleRecording = useCallback(async () => {
    if (isRecording) {
      await stopRecording();
    } else {
      // Pass the actual VNC canvas dimensions (available now that the connection is up)
      const canvas = viewerContainerRef.current?.querySelector('canvas');
      await startRecording(session.id, canvas?.width, canvas?.height);
    }
  }, [isRecording, startRecording, stopRecording, session.id]);

  // Auto-start recording when admin policy is "auto" and WebSocket is ready
  useEffect(() => {
    if (
      session.recording_policy === 'auto' &&
      hasWs &&
      !isRecording
    ) {
      const canvas = viewerContainerRef.current?.querySelector('canvas');
      startRecording(session.id, canvas?.width, canvas?.height);
    }
  }, [hasWs, session.recording_policy, session.id, isRecording, startRecording]);

  const formatDuration = (seconds: number) => {
    const m = Math.floor(seconds / 60);
    const s = seconds % 60;
    return `${m}:${s.toString().padStart(2, '0')}`;
  };

  // Detect fullscreen changes (user might exit via Escape)
  useEffect(() => {
    const handleFullscreenChange = () => {
      setIsFullscreen(!!document.fullscreenElement);
    };
    document.addEventListener('fullscreenchange', handleFullscreenChange);
    return () => document.removeEventListener('fullscreenchange', handleFullscreenChange);
  }, []);

  const toggleFullscreen = useCallback(async () => {
    if (!viewerContainerRef.current) return;
    try {
      if (document.fullscreenElement) {
        await document.exitFullscreen();
      } else {
        await viewerContainerRef.current.requestFullscreen();
      }
    } catch (err) {
      console.warn('Fullscreen not available:', err);
    }
  }, []);

  const handleViewerConnect = useCallback(() => {
    setViewerState('connected');
    setReconnectInfo(null);
    onConnect?.();
  }, [onConnect]);

  const handleViewerDisconnect = useCallback((clean: boolean) => {
    if (!clean) {
      setViewerState('error');
      setErrorMessage('Connection lost');
    }
    onDisconnect?.(clean);
  }, [onDisconnect]);

  const handleViewerError = useCallback((message: string) => {
    setViewerState('error');
    setErrorMessage(message);
    onError?.(message);
  }, [onError]);

  const handleReconnecting = useCallback((attempt: number, max: number) => {
    setViewerState('reconnecting');
    setReconnectInfo({ attempt, max });
  }, []);

  const handleReconnected = useCallback(() => {
    setViewerState('connected');
    setReconnectInfo(null);
  }, []);

  const toggleClipboardToast = useCallback(() => {
    setShowClipboardToast(true);
    setTimeout(() => setShowClipboardToast(false), 3000);
  }, []);

  // Determine which viewer to render
  const isVNC = !!session.websocket_url && !session.guacamole_url;
  const isGuacamole = !!session.guacamole_url;
  const isWebProxy = !!session.proxy_url && !session.websocket_url && !session.guacamole_url;

  const overlayBg = darkMode ? 'bg-gray-900/90' : 'bg-gray-100/90';
  const overlayText = darkMode ? 'text-gray-100' : 'text-gray-900';
  const btnBg = darkMode ? 'bg-gray-700 hover:bg-gray-600' : 'bg-gray-200 hover:bg-gray-300';

  return (
    <div ref={viewerContainerRef} className="w-full h-full relative bg-black">
      {/* Viewer */}
      {isVNC && (
        <VNCViewer
          wsUrl={session.websocket_url!}
          viewOnly={viewOnly}
          onConnect={handleViewerConnect}
          onDisconnect={handleViewerDisconnect}
          onError={handleViewerError}
          onReconnecting={handleReconnecting}
          onReconnected={handleReconnected}
          onWebSocketReady={handleWebSocketReady}
          showStats={showStats}
          clipboardPolicy={viewOnly ? 'none' : clipboardPolicy}
        />
      )}

      {isGuacamole && (
        <GuacamoleViewer
          wsUrl={session.guacamole_url!}
          viewOnly={viewOnly}
          onConnect={handleViewerConnect}
          onDisconnect={handleViewerDisconnect}
          onError={handleViewerError}
          onReconnecting={handleReconnecting}
          onReconnected={handleReconnected}
          clipboardPolicy={viewOnly ? 'none' : clipboardPolicy}
        />
      )}

      {isWebProxy && (
        <iframe
          src={`${window.location.origin}${session.proxy_url}`}
          className="w-full h-full border-0"
          title={`${app.name} session`}
          sandbox="allow-same-origin allow-scripts allow-forms allow-popups"
          onLoad={() => setViewerState('connected')}
        />
      )}

      {/* Reconnecting overlay */}
      {viewerState === 'reconnecting' && reconnectInfo && (
        <div className={`absolute inset-0 flex flex-col items-center justify-center ${overlayBg} z-20`}>
          <svg className="w-10 h-10 animate-spin text-blue-500 mb-4" fill="none" viewBox="0 0 24 24">
            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
          </svg>
          <p className={`text-lg font-medium ${overlayText}`}>Reconnecting...</p>
          <p className={`text-sm mt-1 ${darkMode ? 'text-gray-400' : 'text-gray-500'}`}>
            Attempt {reconnectInfo.attempt} of {reconnectInfo.max}
          </p>
        </div>
      )}

      {/* Error overlay */}
      {viewerState === 'error' && (
        <div className={`absolute inset-0 flex flex-col items-center justify-center ${overlayBg} z-20`}>
          <svg className="w-12 h-12 text-red-500 mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
          </svg>
          <p className={`text-lg font-medium ${overlayText}`}>{errorMessage || 'Connection error'}</p>
        </div>
      )}

      {/* Floating toolbar (bottom-center, visible when connected) */}
      {viewerState === 'connected' && (
        <div className="absolute bottom-4 left-1/2 -translate-x-1/2 z-30 flex items-center gap-1 px-2 py-1 rounded-lg bg-black/60 backdrop-blur-sm opacity-0 hover:opacity-100 focus-within:opacity-100 transition-opacity">
          {/* Fullscreen toggle */}
          <button
            onClick={toggleFullscreen}
            className={`p-1.5 rounded ${btnBg} text-white transition-colors`}
            title={isFullscreen ? 'Exit fullscreen (F11)' : 'Enter fullscreen (F11)'}
            aria-label={isFullscreen ? 'Exit fullscreen' : 'Enter fullscreen'}
          >
            {isFullscreen ? (
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 9V4.5M9 9H4.5M9 9L3.75 3.75M9 15v4.5M9 15H4.5M9 15l-5.25 5.25M15 9h4.5M15 9V4.5M15 9l5.25-5.25M15 15h4.5M15 15v4.5m0-4.5l5.25 5.25" />
              </svg>
            ) : (
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3.75 3.75v4.5m0-4.5h4.5m-4.5 0L9 9M3.75 20.25v-4.5m0 4.5h4.5m-4.5 0L9 15M20.25 3.75h-4.5m4.5 0v4.5m0-4.5L15 9m5.25 11.25h-4.5m4.5 0v-4.5m0 4.5L15 15" />
              </svg>
            )}
          </button>

          {/* Record toggle */}
          {isVNC && hasWs && (
            <button
              onClick={toggleRecording}
              className={`p-1.5 rounded transition-colors ${isRecording ? 'bg-red-600 text-white' : `${btnBg} text-white`}`}
              title={isRecording ? `Stop recording (${formatDuration(recordingDuration)})` : 'Start recording'}
              aria-label={isRecording ? 'Stop recording' : 'Start recording'}
            >
              {isRecording ? (
                <span className="flex items-center gap-1">
                  <span className="w-2 h-2 rounded-full bg-white animate-pulse" />
                  <span className="text-xs font-mono">{formatDuration(recordingDuration)}</span>
                </span>
              ) : (
                <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 24 24">
                  <circle cx="12" cy="12" r="8" />
                </svg>
              )}
            </button>
          )}

          {/* Stats toggle (VNC only) */}
          {isVNC && (
            <button
              onClick={() => setShowStats((s) => !s)}
              className={`p-1.5 rounded transition-colors ${showStats ? 'bg-green-600 text-white' : `${btnBg} text-white`}`}
              title="Toggle performance stats"
              aria-label="Toggle performance stats"
            >
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
              </svg>
            </button>
          )}

          {/* Clipboard policy indicator */}
          <button
            onClick={toggleClipboardToast}
            className={`p-1.5 rounded ${btnBg} text-white transition-colors`}
            title={CLIPBOARD_LABELS[clipboardPolicy]}
            aria-label={CLIPBOARD_LABELS[clipboardPolicy]}
          >
            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              {clipboardPolicy === 'none' ? (
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M18.364 18.364A9 9 0 005.636 5.636m12.728 12.728A9 9 0 015.636 5.636m12.728 12.728L5.636 5.636" />
              ) : (
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2" />
              )}
            </svg>
          </button>
        </div>
      )}

      {/* Recording error toast */}
      {recordingError && (
        <div className="absolute top-4 left-1/2 -translate-x-1/2 z-30 px-3 py-2 rounded-lg bg-red-600/90 text-white text-sm whitespace-nowrap">
          Recording error: {recordingError}
        </div>
      )}

      {/* Clipboard policy toast */}
      {showClipboardToast && (
        <div className="absolute bottom-16 left-1/2 -translate-x-1/2 z-30 px-3 py-2 rounded-lg bg-black/80 text-white text-sm whitespace-nowrap animate-fade-in">
          {CLIPBOARD_LABELS[clipboardPolicy]}
        </div>
      )}
    </div>
  );
}
