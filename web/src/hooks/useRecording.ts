import { useState, useRef, useCallback } from 'react';
import { getAccessToken } from '../services/auth';

interface UseRecordingReturn {
  isRecording: boolean;
  duration: number;
  /** Attach to a WebSocket immediately on creation to passively buffer all traffic. */
  attachWebSocket: (ws: WebSocket, screenWidth: number, screenHeight: number) => void;
  /** Start a recording (calls server API). Requires attachWebSocket to have been called.
   *  Pass current screen dimensions to ensure the VREC header and FBU request use the actual VNC resolution. */
  startRecording: (sessionId: string, screenWidth?: number, screenHeight?: number) => Promise<void>;
  stopRecording: () => Promise<void>;
  error: string | null;
}

const VREC_MAGIC = 0x56524543; // "VREC"

/**
 * Binary frame format (per message):
 *   fromClient: 1 byte  (0 = server→client, 1 = client→server)
 *   timestamp:  4 bytes (uint32 BE, ms since recording start)
 *   dataLen:    4 bytes (uint32 BE)
 *   data:       dataLen bytes
 */
function packFrame(fromClient: boolean, timestampMs: number, data: ArrayBuffer): ArrayBuffer {
  const header = new ArrayBuffer(9);
  const view = new DataView(header);
  view.setUint8(0, fromClient ? 1 : 0);
  view.setUint32(1, timestampMs >>> 0, false); // big-endian
  view.setUint32(5, data.byteLength, false);
  const frame = new Uint8Array(9 + data.byteLength);
  frame.set(new Uint8Array(header), 0);
  frame.set(new Uint8Array(data), 9);
  return frame.buffer;
}

function buildVrecHeader(width: number, height: number): ArrayBuffer {
  const buf = new ArrayBuffer(12);
  const view = new DataView(buf);
  view.setUint32(0, VREC_MAGIC, false);  // "VREC"
  view.setUint16(4, 1, false);           // version 1
  view.setUint16(6, width, false);
  view.setUint16(8, height, false);
  view.setUint16(10, 0, false);          // reserved
  return buf;
}

/** Build a VNC SetEncodings message requesting only Raw encoding. */
function buildSetEncodingsRaw(): ArrayBuffer {
  const buf = new ArrayBuffer(8);
  const view = new DataView(buf);
  view.setUint8(0, 2);           // message-type: SetEncodings
  view.setUint8(1, 0);           // padding
  view.setUint16(2, 1, false);   // number-of-encodings: 1
  view.setInt32(4, 0, false);    // encoding: Raw (0)
  return buf;
}

/** Build a VNC FramebufferUpdateRequest for a full (non-incremental) screen refresh. */
function buildFBURequest(width: number, height: number): ArrayBuffer {
  const buf = new ArrayBuffer(10);
  const view = new DataView(buf);
  view.setUint8(0, 3);               // message-type: FramebufferUpdateRequest
  view.setUint8(1, 0);               // incremental: 0 (full)
  view.setUint16(2, 0, false);       // x
  view.setUint16(4, 0, false);       // y
  view.setUint16(6, width, false);   // width
  view.setUint16(8, height, false);  // height
  return buf;
}

/** Build a VNC SetEncodings message restoring noVNC's preferred encoding list. */
function buildSetEncodingsRestore(): ArrayBuffer {
  // Matches noVNC's default preference order
  const encodings = [
    1,     // CopyRect
    7,     // Tight
    16,    // ZRLE
    5,     // Hextile
    2,     // RRE
    0,     // Raw
    -223,  // DesktopSize
    -224,  // LastRect
    -239,  // Cursor
    -257,  // ExtendedClipboard
    -258,  // QEMUExtendedKeyEvent
    -308,  // ExtendedDesktopSize
  ];
  const buf = new ArrayBuffer(4 + encodings.length * 4);
  const view = new DataView(buf);
  view.setUint8(0, 2);                         // message-type
  view.setUint8(1, 0);                         // padding
  view.setUint16(2, encodings.length, false);   // count
  for (let i = 0; i < encodings.length; i++) {
    view.setInt32(4 + i * 4, encodings[i], false);
  }
  return buf;
}

export function useRecording(): UseRecordingReturn {
  const [isRecording, setIsRecording] = useState(false);
  const [duration, setDuration] = useState(0);
  const [error, setError] = useState<string | null>(null);

  // Passive capture state (from WS creation)
  const chunksRef = useRef<ArrayBuffer[]>([]);
  const captureStartRef = useRef<number>(0); // performance.now() when capture started
  const wsRef = useRef<WebSocket | null>(null);
  const origSendRef = useRef<WebSocket['send'] | null>(null);
  const messageHandlerRef = useRef<((ev: MessageEvent) => void) | null>(null);
  const attachedRef = useRef(false);
  const screenWidthRef = useRef(0);
  const screenHeightRef = useRef(0);

  // Active recording state
  const recordingIdRef = useRef<string>('');
  const sessionIdRef = useRef<string>('');
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const recordStartTimeRef = useRef<number>(0); // Date.now() when recording started

  // Detach capture hooks from the WebSocket
  const detachCapture = useCallback(() => {
    const ws = wsRef.current;
    if (ws) {
      if (origSendRef.current) {
        ws.send = origSendRef.current;
        origSendRef.current = null;
      }
      if (messageHandlerRef.current) {
        ws.removeEventListener('message', messageHandlerRef.current);
        messageHandlerRef.current = null;
      }
    }
    wsRef.current = null;
    attachedRef.current = false;
  }, []);

  const attachWebSocket = useCallback((ws: WebSocket, screenWidth: number, screenHeight: number) => {
    // Clean up previous attachment if any
    detachCapture();

    wsRef.current = ws;
    screenWidthRef.current = screenWidth;
    screenHeightRef.current = screenHeight;
    chunksRef.current = [];
    const startTime = performance.now();
    captureStartRef.current = startTime;

    // Capture incoming (server→client) messages
    const onMessage = (ev: MessageEvent) => {
      const ts = Math.round(performance.now() - startTime);
      if (ev.data instanceof ArrayBuffer) {
        chunksRef.current.push(packFrame(false, ts, ev.data));
      } else if (ev.data instanceof Blob) {
        ev.data.arrayBuffer().then((buf) => {
          chunksRef.current.push(packFrame(false, ts, buf));
        });
      }
    };
    messageHandlerRef.current = onMessage;
    ws.addEventListener('message', onMessage);

    // Wrap ws.send() to capture outgoing (client→server) messages
    const origSend = ws.send.bind(ws);
    origSendRef.current = ws.send;
    ws.send = function (data: string | ArrayBuffer | Blob | ArrayBufferView) {
      const ts = Math.round(performance.now() - startTime);
      if (data instanceof ArrayBuffer) {
        chunksRef.current.push(packFrame(true, ts, data));
      } else if (ArrayBuffer.isView(data)) {
        const copy = new Uint8Array(data.byteLength);
        copy.set(new Uint8Array(data.buffer as ArrayBuffer, data.byteOffset, data.byteLength));
        chunksRef.current.push(packFrame(true, ts, copy.buffer));
      }
      return origSend(data);
    };

    attachedRef.current = true;
  }, [detachCapture]);

  const startRecording = useCallback(async (sessionId: string, screenWidth?: number, screenHeight?: number) => {
    setError(null);
    sessionIdRef.current = sessionId;

    if (!attachedRef.current || !wsRef.current) {
      setError('WebSocket not attached');
      return;
    }

    // Update screen dimensions if provided (canvas is now available with actual VNC resolution)
    if (screenWidth && screenWidth > 0) screenWidthRef.current = screenWidth;
    if (screenHeight && screenHeight > 0) screenHeightRef.current = screenHeight;

    try {
      // Create recording record on server
      const res = await fetch(`/api/sessions/${sessionId}/recording/start`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${getAccessToken()}`,
          'Content-Type': 'application/json',
        },
      });

      if (!res.ok) {
        const text = await res.text();
        setError(text || 'Failed to start recording');
        return;
      }

      const data = await res.json();
      recordingIdRef.current = data.recording_id;

      // Clear all previously captured frames (they use non-Raw encodings
      // like Tight/ZRLE that the converter can't decode). Reset the capture
      // timer so timestamps start from zero.
      chunksRef.current = [];
      captureStartRef.current = performance.now();

      // Switch VNC server to Raw encoding so the recording is self-contained
      // (no accumulated zlib state from before the recording).
      // Use the underlying send to avoid recording these injected messages.
      const rawSend = origSendRef.current;
      if (rawSend) {
        rawSend.call(wsRef.current, buildSetEncodingsRaw());
        const w = screenWidthRef.current;
        const h = screenHeightRef.current;
        if (w > 0 && h > 0) {
          rawSend.call(wsRef.current, buildFBURequest(w, h));
        }
      }

      setIsRecording(true);
      setDuration(0);
      recordStartTimeRef.current = Date.now();

      // Update duration counter
      timerRef.current = setInterval(() => {
        setDuration(Math.floor((Date.now() - recordStartTimeRef.current) / 1000));
      }, 1000);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to start recording');
    }
  }, []);

  const stopRecording = useCallback(async () => {
    if (timerRef.current) {
      clearInterval(timerRef.current);
      timerRef.current = null;
    }

    const recordingId = recordingIdRef.current;
    const sessionId = sessionIdRef.current;
    const elapsedSeconds = (Date.now() - recordStartTimeRef.current) / 1000;

    setIsRecording(false);

    // Restore preferred encodings so the live session goes back to efficient compression
    const rawSend = origSendRef.current;
    if (rawSend && wsRef.current) {
      try {
        rawSend.call(wsRef.current, buildSetEncodingsRestore());
      } catch {
        // Non-fatal: WS might already be closed
      }
    }

    // Notify server recording stopped
    try {
      await fetch(`/api/sessions/${sessionId}/recording/stop`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${getAccessToken()}`,
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ recording_id: recordingId }),
      });
    } catch {
      // Non-fatal: upload will still work
    }

    // Build VREC header
    const vrecHeader = buildVrecHeader(screenWidthRef.current, screenHeightRef.current);

    // Assemble binary blob: VREC header + all captured frames
    const totalFrameSize = chunksRef.current.reduce((sum, buf) => sum + buf.byteLength, 0);
    const combined = new Uint8Array(vrecHeader.byteLength + totalFrameSize);
    combined.set(new Uint8Array(vrecHeader), 0);
    let offset = vrecHeader.byteLength;
    for (const buf of chunksRef.current) {
      combined.set(new Uint8Array(buf), offset);
      offset += buf.byteLength;
    }

    // Don't clear chunks — keep capturing for potential future recordings
    // But reset chunks for the next recording
    chunksRef.current = [];
    // Restart the capture timer so a subsequent recording has fresh timestamps
    captureStartRef.current = performance.now();

    const blob = new Blob([combined], { type: 'application/octet-stream' });

    if (blob.size <= vrecHeader.byteLength) {
      setError('No recording data captured');
      return;
    }

    const formData = new FormData();
    formData.append('recording_id', recordingId);
    formData.append('duration', elapsedSeconds.toFixed(1));
    formData.append('file', blob, `${recordingId}.vncrec`);

    try {
      const res = await fetch(`/api/sessions/${sessionId}/recording/upload`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${getAccessToken()}`,
        },
        body: formData,
      });

      if (!res.ok) {
        const text = await res.text();
        setError(text || 'Failed to upload recording');
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to upload recording');
    }
  }, []);

  return { isRecording, duration, attachWebSocket, startRecording, stopRecording, error };
}
