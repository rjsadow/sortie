import { useEffect, useRef, useCallback } from 'react';
import type RFB from '@novnc/novnc/lib/rfb.js';

interface VNCViewerProps {
  wsUrl: string;
  onConnect?: () => void;
  onDisconnect?: (clean: boolean) => void;
  onError?: (message: string) => void;
  viewOnly?: boolean;
  scaleViewport?: boolean;
}

export function VNCViewer({
  wsUrl,
  onConnect,
  onDisconnect,
  onError,
  viewOnly = false,
  scaleViewport = true,
}: VNCViewerProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const rfbRef = useRef<RFB | null>(null);

  const handleConnect = useCallback(() => {
    console.log('VNC connected');
    onConnect?.();
  }, [onConnect]);

  const handleDisconnect = useCallback((e: unknown) => {
    const event = e as { detail?: { clean?: boolean } };
    const clean = event?.detail?.clean ?? false;
    console.log('VNC disconnected, clean:', clean);
    onDisconnect?.(clean);
  }, [onDisconnect]);

  const handleSecurityFailure = useCallback((e: unknown) => {
    const event = e as { detail?: { reason?: string } };
    const reason = event?.detail?.reason ?? 'Unknown security failure';
    console.error('VNC security failure:', reason);
    onError?.(reason);
  }, [onError]);

  // Handle clipboard from remote VNC -> local
  const handleClipboard = useCallback((e: unknown) => {
    const event = e as { detail?: { text?: string } };
    const text = event?.detail?.text;
    if (text && navigator.clipboard) {
      navigator.clipboard.writeText(text).catch((err) => {
        console.warn('Failed to write to clipboard:', err);
      });
    }
  }, []);

  // Sync local clipboard to remote VNC when container is focused
  const syncClipboardToRemote = useCallback(async () => {
    if (!rfbRef.current || viewOnly) return;
    try {
      if (navigator.clipboard) {
        const text = await navigator.clipboard.readText();
        if (text) {
          rfbRef.current.clipboardPasteFrom(text);
        }
      }
    } catch (err) {
      // Clipboard access may be denied - that's ok
      console.debug('Clipboard read not available:', err);
    }
  }, [viewOnly]);

  // Handle paste event (Ctrl+V / Cmd+V) - more reliable than clipboard API
  const handlePaste = useCallback((e: React.ClipboardEvent) => {
    if (!rfbRef.current || viewOnly) return;
    const text = e.clipboardData.getData('text/plain');
    if (text) {
      e.preventDefault();
      rfbRef.current.clipboardPasteFrom(text);
    }
  }, [viewOnly]);

  useEffect(() => {
    if (!containerRef.current || !wsUrl) {
      return;
    }

    // Build full WebSocket URL
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const fullWsUrl = wsUrl.startsWith('ws') ? wsUrl : `${protocol}//${window.location.host}${wsUrl}`;

    // Dynamically import noVNC
    const initRFB = async () => {
      try {
        // Dynamic import
        const { default: RFBClass } = await import('@novnc/novnc/lib/rfb.js');

        if (!containerRef.current) return;

        // Create RFB connection
        const rfb = new RFBClass(containerRef.current, fullWsUrl, {
          shared: true,
        });

        // Configure RFB
        rfb.scaleViewport = scaleViewport;
        rfb.resizeSession = false;
        rfb.viewOnly = viewOnly;
        rfb.clipViewport = false;

        // Add event listeners
        rfb.addEventListener('connect', handleConnect);
        rfb.addEventListener('disconnect', handleDisconnect);
        rfb.addEventListener('securityfailure', handleSecurityFailure);
        rfb.addEventListener('clipboard', handleClipboard);

        rfbRef.current = rfb;

        // Sync local clipboard to remote when focused
        syncClipboardToRemote();
      } catch (err) {
        console.error('Failed to load or initialize noVNC:', err);
        onError?.(err instanceof Error ? err.message : 'Failed to load VNC viewer');
      }
    };

    initRFB();

    // Cleanup on unmount
    return () => {
      if (rfbRef.current) {
        rfbRef.current.removeEventListener('connect', handleConnect);
        rfbRef.current.removeEventListener('disconnect', handleDisconnect);
        rfbRef.current.removeEventListener('securityfailure', handleSecurityFailure);
        rfbRef.current.removeEventListener('clipboard', handleClipboard);
        rfbRef.current.disconnect();
        rfbRef.current = null;
      }
    };
  }, [wsUrl, viewOnly, scaleViewport, handleConnect, handleDisconnect, handleSecurityFailure, handleClipboard, syncClipboardToRemote, onError]);

  return (
    <div
      ref={containerRef}
      className="w-full h-full bg-black"
      style={{ minHeight: '400px' }}
      onFocus={syncClipboardToRemote}
      onClick={syncClipboardToRemote}
      onPaste={handlePaste}
      tabIndex={0}
    />
  );
}
