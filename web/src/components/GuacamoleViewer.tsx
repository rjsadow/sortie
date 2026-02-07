import { useEffect, useRef, useCallback } from 'react';

interface GuacamoleViewerProps {
  wsUrl: string;
  onConnect?: () => void;
  onDisconnect?: (clean: boolean) => void;
  onError?: (message: string) => void;
  viewOnly?: boolean;
  scaleViewport?: boolean;
}

export function GuacamoleViewer({
  wsUrl,
  onConnect,
  onDisconnect,
  onError,
  viewOnly = false,
  scaleViewport = true,
}: GuacamoleViewerProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const clientRef = useRef<any>(null);

  const handleConnect = useCallback(() => {
    console.log('Guacamole connected');
    onConnect?.();
  }, [onConnect]);

  const handleDisconnect = useCallback((clean: boolean) => {
    console.log('Guacamole disconnected, clean:', clean);
    onDisconnect?.(clean);
  }, [onDisconnect]);

  const handleError = useCallback((message: string) => {
    console.error('Guacamole error:', message);
    onError?.(message);
  }, [onError]);

  useEffect(() => {
    if (!containerRef.current || !wsUrl) {
      return;
    }

    // Build full WebSocket URL
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const fullWsUrl = wsUrl.startsWith('ws') ? wsUrl : `${protocol}//${window.location.host}${wsUrl}`;

    const initGuacamole = async () => {
      try {
        // Dynamic import of guacamole-common-js
        const Guacamole = (await import('guacamole-common-js')).default;

        if (!containerRef.current) return;

        // Create WebSocket tunnel
        const tunnel = new Guacamole.WebSocketTunnel(fullWsUrl);

        // Create Guacamole client
        const client = new Guacamole.Client(tunnel);
        clientRef.current = client;

        // Get the display element and add it to our container
        const displayElement = client.getDisplay().getElement();
        displayElement.style.width = '100%';
        displayElement.style.height = '100%';
        containerRef.current.appendChild(displayElement);

        // Handle state changes
        client.onstatechange = (state: number) => {
          // Guacamole.Client.State: IDLE=0, CONNECTING=1, WAITING=2, CONNECTED=3, DISCONNECTING=4, DISCONNECTED=5
          switch (state) {
            case 3: // CONNECTED
              handleConnect();
              break;
            case 5: // DISCONNECTED
              handleDisconnect(true);
              break;
          }
        };

        // Handle errors
        client.onerror = (status: { code: number; message: string }) => {
          handleError(status?.message || `Error code: ${status?.code}`);
        };

        // Scale display to fit container
        if (scaleViewport) {
          const display = client.getDisplay();
          const resizeObserver = new ResizeObserver(() => {
            if (!containerRef.current) return;
            const containerWidth = containerRef.current.clientWidth;
            const containerHeight = containerRef.current.clientHeight;
            const displayWidth = display.getWidth();
            const displayHeight = display.getHeight();

            if (displayWidth > 0 && displayHeight > 0) {
              const scale = Math.min(
                containerWidth / displayWidth,
                containerHeight / displayHeight
              );
              display.scale(scale);
            }
          });
          resizeObserver.observe(containerRef.current);

          // Also listen for display size changes from guacd
          display.onresize = () => {
            if (!containerRef.current) return;
            const containerWidth = containerRef.current.clientWidth;
            const containerHeight = containerRef.current.clientHeight;
            const displayWidth = display.getWidth();
            const displayHeight = display.getHeight();

            if (displayWidth > 0 && displayHeight > 0) {
              const scale = Math.min(
                containerWidth / displayWidth,
                containerHeight / displayHeight
              );
              display.scale(scale);
            }
          };
        }

        // Set up keyboard and mouse input (unless viewOnly)
        if (!viewOnly) {
          const mouse = new Guacamole.Mouse(displayElement);
          mouse.onmousedown = mouse.onmouseup = mouse.onmousemove = (mouseState: unknown) => {
            client.sendMouseState(mouseState);
          };

          const keyboard = new Guacamole.Keyboard(document);
          keyboard.onkeydown = (keysym: number) => {
            client.sendKeyEvent(1, keysym);
          };
          keyboard.onkeyup = (keysym: number) => {
            client.sendKeyEvent(0, keysym);
          };
        }

        // Handle clipboard sync from remote to local
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

        // Connect
        client.connect();
      } catch (err) {
        console.error('Failed to initialize Guacamole:', err);
        onError?.(err instanceof Error ? err.message : 'Failed to load Guacamole viewer');
      }
    };

    initGuacamole();

    // Cleanup on unmount
    return () => {
      if (clientRef.current) {
        clientRef.current.disconnect();
        clientRef.current = null;
      }
      if (containerRef.current) {
        containerRef.current.innerHTML = '';
      }
    };
  }, [wsUrl, viewOnly, scaleViewport, handleConnect, handleDisconnect, handleError, onError]);

  return (
    <div
      ref={containerRef}
      className="w-full h-full bg-black"
      style={{ minHeight: '400px' }}
      tabIndex={0}
    />
  );
}
