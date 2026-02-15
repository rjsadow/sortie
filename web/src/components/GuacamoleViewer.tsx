import { useEffect, useRef } from 'react';
import type { ClipboardPolicy } from '../types';

interface GuacamoleViewerProps {
  wsUrl: string;
  onConnect?: () => void;
  onDisconnect?: (clean: boolean) => void;
  onError?: (message: string) => void;
  onReconnecting?: (attempt: number, maxAttempts: number) => void;
  onReconnected?: () => void;
  onCanvasReady?: (canvas: HTMLCanvasElement) => void;
  viewOnly?: boolean;
  scaleViewport?: boolean;
  clipboardPolicy?: ClipboardPolicy;
  maxReconnectAttempts?: number;
  reconnectBackoffMs?: number;
}

export function GuacamoleViewer({
  wsUrl,
  onConnect,
  onDisconnect,
  onError,
  onReconnecting,
  onReconnected,
  onCanvasReady,
  viewOnly = false,
  scaleViewport = true,
  clipboardPolicy = 'bidirectional',
  maxReconnectAttempts = 3,
  reconnectBackoffMs = 1000,
}: GuacamoleViewerProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const clientRef = useRef<any>(null);
  const reconnectAttemptRef = useRef(0);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const wasConnectedRef = useRef(false);
  const unmountedRef = useRef(false);

  // Store callback props in refs so the useEffect doesn't depend on them.
  // This prevents the parent re-rendering (after onConnect fires) from
  // tearing down and re-creating the Guacamole connection.
  const onConnectRef = useRef(onConnect);
  const onDisconnectRef = useRef(onDisconnect);
  const onErrorRef = useRef(onError);
  const onReconnectingRef = useRef(onReconnecting);
  const onReconnectedRef = useRef(onReconnected);
  const onCanvasReadyRef = useRef(onCanvasReady);
  onConnectRef.current = onConnect;
  onDisconnectRef.current = onDisconnect;
  onErrorRef.current = onError;
  onReconnectingRef.current = onReconnecting;
  onReconnectedRef.current = onReconnected;
  onCanvasReadyRef.current = onCanvasReady;

  const canReadRemote = clipboardPolicy === 'read' || clipboardPolicy === 'bidirectional';
  const canWriteRemote = clipboardPolicy === 'write' || clipboardPolicy === 'bidirectional';

  // Forward-declare so disconnect handler can schedule reconnects
  const connectGuacRef = useRef<() => void>(() => {});

  useEffect(() => {
    if (!containerRef.current || !wsUrl) return;

    unmountedRef.current = false;
    reconnectAttemptRef.current = 0;
    wasConnectedRef.current = false;

    const clearReconnectTimer = () => {
      if (reconnectTimerRef.current) {
        clearTimeout(reconnectTimerRef.current);
        reconnectTimerRef.current = null;
      }
    };

    const handleConnect = () => {
      console.log('Guacamole connected');
      const wasReconnect = reconnectAttemptRef.current > 0;
      reconnectAttemptRef.current = 0;
      wasConnectedRef.current = true;
      if (wasReconnect) {
        onReconnectedRef.current?.();
      }
      onConnectRef.current?.();

      // Extract the Guacamole canvas for recording
      if (onCanvasReadyRef.current && clientRef.current) {
        try {
          const canvas = clientRef.current.getDisplay().getDefaultLayer().getCanvas();
          if (canvas) {
            onCanvasReadyRef.current(canvas);
          }
        } catch {
          // Canvas extraction may fail if display isn't ready yet
        }
      }
    };

    const handleDisconnect = (clean: boolean) => {
      console.log('Guacamole disconnected, clean:', clean);

      if (clientRef.current) {
        clientRef.current = null;
      }

      if (!clean && wasConnectedRef.current && !unmountedRef.current) {
        const attempt = reconnectAttemptRef.current + 1;
        if (attempt <= maxReconnectAttempts) {
          reconnectAttemptRef.current = attempt;
          const delay = reconnectBackoffMs * Math.pow(2, attempt - 1);
          console.log(`Guacamole reconnect attempt ${attempt}/${maxReconnectAttempts} in ${delay}ms`);
          onReconnectingRef.current?.(attempt, maxReconnectAttempts);
          reconnectTimerRef.current = setTimeout(() => {
            if (!unmountedRef.current) {
              connectGuacRef.current();
            }
          }, delay);
          return;
        }
      }

      onDisconnectRef.current?.(clean);
    };

    const handleError = (message: string) => {
      console.error('Guacamole error:', message);
      onErrorRef.current?.(message);
    };

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const fullWsUrl = wsUrl.startsWith('ws') ? wsUrl : `${protocol}//${window.location.host}${wsUrl}`;

    const connectGuacamole = async () => {
      if (unmountedRef.current || !containerRef.current) return;

      // Measure container to pass actual viewport dimensions to the RDP session
      const containerWidth = containerRef.current.clientWidth;
      const containerHeight = containerRef.current.clientHeight;
      const sep = fullWsUrl.includes('?') ? '&' : '?';
      const tunnelUrl = `${fullWsUrl}${sep}width=${containerWidth}&height=${containerHeight}`;

      try {
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        const Guacamole = (await import('guacamole-common-js')).default as any;
        if (unmountedRef.current || !containerRef.current) return;

        // Clear container on reconnect
        if (reconnectAttemptRef.current > 0) {
          containerRef.current.innerHTML = '';
        }

        const tunnel = new Guacamole.WebSocketTunnel(tunnelUrl);
        const client = new Guacamole.Client(tunnel);
        clientRef.current = client;

        // Let Guacamole manage the display element dimensions via display.scale()
        // Do NOT override with CSS width/height as it causes mouse coordinate misalignment
        const displayElement = client.getDisplay().getElement();
        containerRef.current.appendChild(displayElement);

        // State changes
        client.onstatechange = (state: number) => {
          // IDLE=0, CONNECTING=1, WAITING=2, CONNECTED=3, DISCONNECTING=4, DISCONNECTED=5
          switch (state) {
            case 3:
              handleConnect();
              break;
            case 5:
              handleDisconnect(true);
              break;
          }
        };

        client.onerror = (status: { code: number; message: string }) => {
          handleError(status?.message || `Error code: ${status?.code}`);
        };

        // Scale display
        if (scaleViewport) {
          const display = client.getDisplay();
          const doScale = () => {
            if (!containerRef.current) return;
            const containerWidth = containerRef.current.clientWidth;
            const containerHeight = containerRef.current.clientHeight;
            const displayWidth = display.getWidth();
            const displayHeight = display.getHeight();
            if (displayWidth > 0 && displayHeight > 0) {
              const scale = Math.min(containerWidth / displayWidth, containerHeight / displayHeight);
              display.scale(scale);
            }
          };

          const resizeObserver = new ResizeObserver(doScale);
          resizeObserver.observe(containerRef.current);
          display.onresize = doScale;
        }

        // Keyboard & mouse input
        if (!viewOnly) {
          const mouse = new Guacamole.Mouse(displayElement);
          mouse.onmousedown = mouse.onmouseup = mouse.onmousemove = (mouseState: unknown) => {
            client.sendMouseState(mouseState);
          };

          const keyboard = new Guacamole.Keyboard(containerRef.current);
          keyboard.onkeydown = (keysym: number) => {
            client.sendKeyEvent(1, keysym);
          };
          keyboard.onkeyup = (keysym: number) => {
            client.sendKeyEvent(0, keysym);
          };
        }

        // Clipboard: remote → local (gated by policy)
        if (canReadRemote) {
          client.onclipboard = (stream: { onblob: ((data: string) => void) | null; onend: (() => void) | null }, mimetype: string) => {
            if (mimetype === 'text/plain') {
              let clipboardData = '';
              stream.onblob = (data: string) => {
                clipboardData += atob(data);
              };
              stream.onend = () => {
                if (clipboardData && navigator.clipboard) {
                  navigator.clipboard.writeText(clipboardData).catch((err) => {
                    console.warn('Failed to write to clipboard:', err);
                  });
                }
              };
            }
          };
        }

        // Clipboard: local → remote (gated by policy)
        if (canWriteRemote && !viewOnly) {
          const syncClipboard = async () => {
            if (!clientRef.current) return;
            try {
              const text = await navigator.clipboard.readText();
              if (text) {
                const stream = clientRef.current.createClipboardStream('text/plain');
                const encoded = btoa(text);
                stream.sendBlob(encoded);
                stream.sendEnd();
              }
            } catch {
              // Clipboard access may be denied
            }
          };

          containerRef.current.addEventListener('focus', syncClipboard);
          containerRef.current.addEventListener('click', syncClipboard);
        }

        client.connect();
      } catch (err) {
        console.error('Failed to initialize Guacamole:', err);
        onErrorRef.current?.(err instanceof Error ? err.message : 'Failed to load Guacamole viewer');
      }
    };

    connectGuacRef.current = connectGuacamole;
    connectGuacamole();

    const container = containerRef.current;
    return () => {
      unmountedRef.current = true;
      clearReconnectTimer();
      if (clientRef.current) {
        clientRef.current.disconnect();
        clientRef.current = null;
      }
      if (container) {
        container.innerHTML = '';
      }
    };
  // Only re-run when structural config changes, not when callback refs change
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [wsUrl, viewOnly, scaleViewport, canReadRemote, canWriteRemote, maxReconnectAttempts, reconnectBackoffMs]);

  return (
    <div
      ref={containerRef}
      className="w-full h-full bg-black"
      style={{ minHeight: '400px' }}
      tabIndex={0}
    />
  );
}
