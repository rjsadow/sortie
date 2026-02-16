import { useEffect, useRef, useCallback, useState } from 'react';
import type RFB from '@novnc/novnc/lib/rfb.js';
import type { ClipboardPolicy } from '../types';

export interface VNCStats {
  fps: number;
  frameTime: number;
  drawOps: number;
}

interface VNCViewerProps {
  wsUrl: string;
  onConnect?: () => void;
  onDisconnect?: (clean: boolean) => void;
  onError?: (message: string) => void;
  onReconnecting?: (attempt: number, maxAttempts: number) => void;
  onReconnected?: () => void;
  onCanvasReady?: (canvas: HTMLCanvasElement) => void;
  onWebSocketReady?: (ws: WebSocket) => void;
  viewOnly?: boolean;
  scaleViewport?: boolean;
  showStats?: boolean;
  clipboardPolicy?: ClipboardPolicy;
  maxReconnectAttempts?: number;
  reconnectBackoffMs?: number;
}

export function VNCViewer({
  wsUrl,
  onConnect,
  onDisconnect,
  onError,
  onReconnecting,
  onReconnected,
  onCanvasReady,
  onWebSocketReady,
  viewOnly = false,
  scaleViewport = true,
  showStats = false,
  clipboardPolicy = 'bidirectional',
  maxReconnectAttempts = 3,
  reconnectBackoffMs = 1000,
}: VNCViewerProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const rfbRef = useRef<RFB | null>(null);
  const reconnectAttemptRef = useRef(0);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const wasConnectedRef = useRef(false);
  const unmountedRef = useRef(false);
  const [stats, setStats] = useState<VNCStats>({ fps: 0, frameTime: 0, drawOps: 0 });

  const canReadRemote = clipboardPolicy === 'read' || clipboardPolicy === 'bidirectional';
  const canWriteRemote = clipboardPolicy === 'write' || clipboardPolicy === 'bidirectional';

  // Build full WebSocket URL once
  const fullWsUrl = useRef('');
  useEffect(() => {
    if (!wsUrl) return;
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    fullWsUrl.current = wsUrl.startsWith('ws') ? wsUrl : `${protocol}//${window.location.host}${wsUrl}`;
  }, [wsUrl]);

  const clearReconnectTimer = useCallback(() => {
    if (reconnectTimerRef.current) {
      clearTimeout(reconnectTimerRef.current);
      reconnectTimerRef.current = null;
    }
  }, []);

  // Forward-declare connectRFB so disconnect handler can schedule reconnects
  const connectRFBRef = useRef<() => void>(() => {});

  const handleConnect = useCallback(() => {
    console.log('VNC connected');
    const wasReconnect = reconnectAttemptRef.current > 0;
    reconnectAttemptRef.current = 0;
    wasConnectedRef.current = true;
    if (wasReconnect) {
      onReconnected?.();
    }
    onConnect?.();
  }, [onConnect, onReconnected]);

  const handleDisconnect = useCallback((e: unknown) => {
    const event = e as { detail?: { clean?: boolean } };
    const clean = event?.detail?.clean ?? false;
    console.log('VNC disconnected, clean:', clean);

    // Clean up the old RFB instance
    if (rfbRef.current) {
      rfbRef.current = null;
    }

    if (!clean && wasConnectedRef.current && !unmountedRef.current) {
      // Unclean disconnect while we were connected — attempt reconnect
      const attempt = reconnectAttemptRef.current + 1;
      if (attempt <= maxReconnectAttempts) {
        reconnectAttemptRef.current = attempt;
        const delay = reconnectBackoffMs * Math.pow(2, attempt - 1);
        console.log(`VNC reconnect attempt ${attempt}/${maxReconnectAttempts} in ${delay}ms`);
        onReconnecting?.(attempt, maxReconnectAttempts);
        reconnectTimerRef.current = setTimeout(() => {
          if (!unmountedRef.current) {
            connectRFBRef.current();
          }
        }, delay);
        return;
      }
    }

    onDisconnect?.(clean);
  }, [maxReconnectAttempts, reconnectBackoffMs, onDisconnect, onReconnecting]);

  const handleSecurityFailure = useCallback((e: unknown) => {
    const event = e as { detail?: { reason?: string } };
    const reason = event?.detail?.reason ?? 'Unknown security failure';
    console.error('VNC security failure:', reason);
    onError?.(reason);
  }, [onError]);

  // Handle clipboard from remote VNC -> local (gated by policy)
  const handleClipboard = useCallback((e: unknown) => {
    if (!canReadRemote) return;
    const event = e as { detail?: { text?: string } };
    const text = event?.detail?.text;
    if (text && navigator.clipboard) {
      navigator.clipboard.writeText(text).catch((err) => {
        console.warn('Failed to write to clipboard:', err);
      });
    }
  }, [canReadRemote]);

  // Sync local clipboard to remote VNC when container is focused (gated by policy)
  const syncClipboardToRemote = useCallback(async () => {
    if (!rfbRef.current || viewOnly || !canWriteRemote) return;
    try {
      if (navigator.clipboard) {
        const text = await navigator.clipboard.readText();
        if (text) {
          rfbRef.current.clipboardPasteFrom(text);
        }
      }
    } catch {
      // Clipboard access may be denied - that's ok
    }
  }, [viewOnly, canWriteRemote]);

  // Handle paste event (Ctrl+V / Cmd+V) - gated by policy
  const handlePaste = useCallback((e: React.ClipboardEvent) => {
    if (!rfbRef.current || viewOnly || !canWriteRemote) return;
    const text = e.clipboardData.getData('text/plain');
    if (text) {
      e.preventDefault();
      rfbRef.current.clipboardPasteFrom(text);
    }
  }, [viewOnly, canWriteRemote]);

  // Notify parent when canvas element becomes available (for recording, etc.)
  useEffect(() => {
    if (!onCanvasReady || !containerRef.current) return;

    const existing = containerRef.current.querySelector('canvas');
    if (existing) {
      onCanvasReady(existing);
      return;
    }

    const observer = new MutationObserver((mutations) => {
      for (const mutation of mutations) {
        for (const node of Array.from(mutation.addedNodes)) {
          if (node instanceof HTMLCanvasElement) {
            observer.disconnect();
            onCanvasReady(node);
            return;
          }
          if (node instanceof HTMLElement) {
            const c = node.querySelector('canvas');
            if (c) {
              observer.disconnect();
              onCanvasReady(c);
              return;
            }
          }
        }
      }
    });
    observer.observe(containerRef.current, { childList: true, subtree: true });

    return () => observer.disconnect();
  }, [onCanvasReady]);

  // FPS instrumentation: intercept canvas drawImage to count visible VNC frame updates
  useEffect(() => {
    if (!showStats || !containerRef.current) return;

    let rafId = 0;
    let dirty = false;
    let drawCount = 0;
    let frameCount = 0;
    let lastTime = performance.now();
    let origDrawImage: CanvasRenderingContext2D['drawImage'] | null = null;
    let ctx: CanvasRenderingContext2D | null = null;

    const startMonitoring = (canvas: HTMLCanvasElement) => {
      ctx = canvas.getContext('2d');
      if (!ctx) return;

      origDrawImage = ctx.drawImage;
      const interceptedCtx = ctx;
      interceptedCtx.drawImage = function (this: CanvasRenderingContext2D, ...args: Parameters<CanvasRenderingContext2D['drawImage']>) {
        dirty = true;
        drawCount++;
        return origDrawImage!.apply(this, args);
      } as CanvasRenderingContext2D['drawImage'];

      lastTime = performance.now();

      const tick = (now: number) => {
        if (dirty) {
          frameCount++;
          dirty = false;
        }
        const elapsed = now - lastTime;
        if (elapsed >= 1000) {
          const fps = Math.round((frameCount * 1000) / elapsed);
          const ft = frameCount > 0 ? Math.round((elapsed / frameCount) * 10) / 10 : 0;
          setStats({ fps, frameTime: ft, drawOps: drawCount });
          frameCount = 0;
          drawCount = 0;
          lastTime = now;
        }
        rafId = requestAnimationFrame(tick);
      };

      rafId = requestAnimationFrame(tick);
    };

    const existing = containerRef.current.querySelector('canvas');
    let observer: MutationObserver | null = null;

    if (existing) {
      startMonitoring(existing);
    } else {
      observer = new MutationObserver((mutations) => {
        for (const mutation of mutations) {
          for (const node of Array.from(mutation.addedNodes)) {
            if (node instanceof HTMLCanvasElement) {
              observer?.disconnect();
              observer = null;
              startMonitoring(node);
              return;
            }
            if (node instanceof HTMLElement) {
              const c = node.querySelector('canvas');
              if (c) {
                observer?.disconnect();
                observer = null;
                startMonitoring(c);
                return;
              }
            }
          }
        }
      });
      observer.observe(containerRef.current, { childList: true, subtree: true });
    }

    return () => {
      cancelAnimationFrame(rafId);
      observer?.disconnect();
      if (ctx && origDrawImage) {
        ctx.drawImage = origDrawImage;
      }
    };
  }, [showStats]);

  // Main connection effect
  useEffect(() => {
    if (!containerRef.current || !wsUrl) return;

    unmountedRef.current = false;
    reconnectAttemptRef.current = 0;
    wasConnectedRef.current = false;

    const connectRFB = async () => {
      if (unmountedRef.current || !containerRef.current) return;

      try {
        const { default: RFBClass } = await import('@novnc/novnc/lib/rfb.js');
        if (unmountedRef.current || !containerRef.current) return;

        // Clear container of any previous noVNC elements before reconnect
        // (noVNC appends its own canvas/screen elements)
        if (reconnectAttemptRef.current > 0) {
          const screen = containerRef.current.querySelector('.noVNC_screen');
          if (screen) screen.remove();
        }

        // Temporarily wrap WebSocket to capture the instance noVNC creates
        const OriginalWebSocket = window.WebSocket;
        if (onWebSocketReady) {
          const onWsReady = onWebSocketReady;
          const WrappedWS = function (url: string | URL, protocols?: string | string[]) {
            const ws = new OriginalWebSocket(url, protocols);
            onWsReady(ws);
            return ws;
          } as unknown as typeof WebSocket;
          // Copy static properties so noVNC's readyState checks work
          Object.defineProperties(WrappedWS, Object.getOwnPropertyDescriptors(OriginalWebSocket));
          window.WebSocket = WrappedWS;
        }

        const rfb = new RFBClass(containerRef.current, fullWsUrl.current, {
          shared: true,
        });

        // Restore original WebSocket immediately
        if (onWebSocketReady) {
          window.WebSocket = OriginalWebSocket;
        }

        rfb.scaleViewport = scaleViewport;
        rfb.resizeSession = true;
        rfb.viewOnly = viewOnly;
        rfb.clipViewport = false;

        rfb.addEventListener('connect', handleConnect);
        rfb.addEventListener('disconnect', handleDisconnect);
        rfb.addEventListener('securityfailure', handleSecurityFailure);
        rfb.addEventListener('clipboard', handleClipboard);

        rfbRef.current = rfb;

        if (canWriteRemote) {
          syncClipboardToRemote();
        }
      } catch (err) {
        console.error('Failed to load or initialize noVNC:', err);
        onError?.(err instanceof Error ? err.message : 'Failed to load VNC viewer');
      }
    };

    connectRFBRef.current = connectRFB;
    connectRFB();

    return () => {
      unmountedRef.current = true;
      clearReconnectTimer();
      if (rfbRef.current) {
        rfbRef.current.removeEventListener('connect', handleConnect);
        rfbRef.current.removeEventListener('disconnect', handleDisconnect);
        rfbRef.current.removeEventListener('securityfailure', handleSecurityFailure);
        rfbRef.current.removeEventListener('clipboard', handleClipboard);
        rfbRef.current.disconnect();
        rfbRef.current = null;
      }
    };
  }, [wsUrl, viewOnly, scaleViewport, handleConnect, handleDisconnect, handleSecurityFailure, handleClipboard, syncClipboardToRemote, canWriteRemote, onError, onWebSocketReady, clearReconnectTimer]);

  return (
    <div
      ref={containerRef}
      className="w-full h-full bg-black relative"
      style={{ minHeight: '400px' }}
      onFocus={canWriteRemote ? syncClipboardToRemote : undefined}
      onClick={canWriteRemote ? syncClipboardToRemote : undefined}
      onPaste={canWriteRemote ? handlePaste : undefined}
      tabIndex={0}
    >
      {showStats && (
        <div className="absolute top-2 right-2 z-20 bg-black/75 text-green-400 text-xs font-mono px-2 py-1.5 rounded select-none pointer-events-none leading-relaxed">
          <div>{stats.fps} FPS</div>
          <div>{stats.frameTime}ms frame</div>
          <div>{stats.drawOps} draws/s</div>
        </div>
      )}
    </div>
  );
}
