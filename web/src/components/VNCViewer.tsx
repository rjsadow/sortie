import { useEffect, useRef, useCallback, useState } from 'react';
import type RFB from '@novnc/novnc/lib/rfb.js';

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
  viewOnly?: boolean;
  scaleViewport?: boolean;
  showStats?: boolean;
}

export function VNCViewer({
  wsUrl,
  onConnect,
  onDisconnect,
  onError,
  viewOnly = false,
  scaleViewport = true,
  showStats = false,
}: VNCViewerProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const rfbRef = useRef<RFB | null>(null);
  const [stats, setStats] = useState<VNCStats>({ fps: 0, frameTime: 0, drawOps: 0 });

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

      // Intercept drawImage on the visible canvas context to detect VNC blits
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

    // noVNC dynamically inserts its canvas — watch for it
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
      // Restore original drawImage
      if (ctx && origDrawImage) {
        ctx.drawImage = origDrawImage;
      }
    };
  }, [showStats]);

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
        rfb.resizeSession = true;
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
      className="w-full h-full bg-black relative"
      style={{ minHeight: '400px' }}
      onFocus={syncClipboardToRemote}
      onClick={syncClipboardToRemote}
      onPaste={handlePaste}
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
