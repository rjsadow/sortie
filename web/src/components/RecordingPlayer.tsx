import { useEffect, useRef, useState, useCallback } from 'react';
import type RFB from '@novnc/novnc/lib/rfb.js';
import { fetchWithAuth } from '../services/auth';

interface RecordingPlayerProps {
  recordingId: string;
  onClose: () => void;
  darkMode: boolean;
}

interface RecordedFrame {
  fromClient: boolean;
  timestamp: number; // ms since start
  data: ArrayBuffer;
}

const VREC_MAGIC = 0x56524543; // "VREC"
const VREC_HEADER_SIZE = 12;

/** Parse the optional VREC header for screen dimensions. */
function parseVrecHeader(buffer: ArrayBuffer): { width: number; height: number; dataOffset: number } | null {
  if (buffer.byteLength < VREC_HEADER_SIZE) return null;
  const view = new DataView(buffer);
  if (view.getUint32(0, false) !== VREC_MAGIC) return null;
  const version = view.getUint16(4, false);
  if (version !== 1) return null;
  return {
    width: view.getUint16(6, false),
    height: view.getUint16(8, false),
    dataOffset: VREC_HEADER_SIZE,
  };
}

/** Parse the binary vncrec frame format into an array of frames. */
function parseRecording(buffer: ArrayBuffer): RecordedFrame[] {
  const view = new DataView(buffer);
  const frames: RecordedFrame[] = [];
  let offset = 0;

  while (offset + 9 <= buffer.byteLength) {
    const fromClient = view.getUint8(offset) === 1;
    const timestamp = view.getUint32(offset + 1, false);
    const dataLen = view.getUint32(offset + 5, false);
    offset += 9;

    if (offset + dataLen > buffer.byteLength) break;

    const data = buffer.slice(offset, offset + dataLen);
    frames.push({ fromClient, timestamp, data });
    offset += dataLen;
  }

  return frames;
}

/**
 * Build the synthetic VNC server handshake as a single blob.
 * noVNC's _handleMessage loop processes all stages in one call
 * as long as there's enough data in the receive buffer.
 */
function buildSyntheticHandshake(width: number, height: number): ArrayBuffer {
  const serverName = 'Sortie Replay';
  const nameBytes = new TextEncoder().encode(serverName);
  // ProtocolVersion(12) + SecurityTypes(2) + SecurityResult(4) + ServerInit(24+name)
  const totalSize = 12 + 2 + 4 + 24 + nameBytes.length;
  const buf = new ArrayBuffer(totalSize);
  const view = new DataView(buf);
  const bytes = new Uint8Array(buf);
  let off = 0;

  // 1. ProtocolVersion: "RFB 003.008\n" (12 bytes)
  const ver = 'RFB 003.008\n';
  for (let i = 0; i < ver.length; i++) bytes[off++] = ver.charCodeAt(i);

  // 2. SecurityTypes: 1 type available, type=None(1)
  bytes[off++] = 1; // count
  bytes[off++] = 1; // None

  // 3. SecurityResult: OK (uint32 BE = 0)
  view.setUint32(off, 0, false);
  off += 4;

  // 4. ServerInit
  view.setUint16(off, width, false);
  view.setUint16(off + 2, height, false);
  off += 4;

  // Pixel format (16 bytes) — standard 32bpp RGBX
  bytes[off] = 32;      // bits-per-pixel
  bytes[off + 1] = 24;  // depth
  bytes[off + 2] = 0;   // big-endian-flag
  bytes[off + 3] = 1;   // true-colour-flag
  view.setUint16(off + 4, 255, false);  // red-max
  view.setUint16(off + 6, 255, false);  // green-max
  view.setUint16(off + 8, 255, false);  // blue-max
  bytes[off + 10] = 16; // red-shift
  bytes[off + 11] = 8;  // green-shift
  bytes[off + 12] = 0;  // blue-shift
  // bytes 13-15 padding (already 0)
  off += 16;

  // Server name
  view.setUint32(off, nameBytes.length, false);
  off += 4;
  bytes.set(nameBytes, off);

  return buf;
}

// Minimal interface matching what noVNC's Websock.attach() needs
interface MockChannel {
  binaryType: string;
  protocol: string;
  readyState: number;
  onopen: ((ev: Event) => void) | null;
  onmessage: ((ev: MessageEvent) => void) | null;
  onclose: ((ev: CloseEvent) => void) | null;
  onerror: ((ev: Event) => void) | null;
  send: () => void;
  close: () => void;
}

function createMockChannel(): {
  channel: MockChannel;
  dispatchOpen: () => void;
  dispatchMessage: (data: ArrayBuffer) => void;
  dispatchClose: () => void;
} {
  const channel: MockChannel = {
    binaryType: 'arraybuffer',
    protocol: '',
    readyState: WebSocket.CONNECTING,
    onopen: null,
    onmessage: null,
    onclose: null,
    onerror: null,
    send() { /* swallow client→server messages during replay */ },
    close() {
      channel.readyState = WebSocket.CLOSED;
      channel.onclose?.({ code: 1000, reason: '', wasClean: true } as unknown as CloseEvent);
    },
  };

  return {
    channel,
    dispatchOpen() {
      channel.readyState = WebSocket.OPEN;
      channel.onopen?.({ type: 'open' } as unknown as Event);
    },
    dispatchMessage(data: ArrayBuffer) {
      channel.onmessage?.({ data } as unknown as MessageEvent);
    },
    dispatchClose() {
      channel.readyState = WebSocket.CLOSED;
      channel.onclose?.({ code: 1000, reason: '', wasClean: true } as unknown as CloseEvent);
    },
  };
}

/**
 * Detect if a recording includes the VNC handshake by checking if the first
 * server→client frame looks like a protocol version string ("RFB xxx.yyy\n").
 */
function startsWithHandshake(frames: RecordedFrame[]): boolean {
  const first = frames.find((f) => !f.fromClient);
  if (!first || first.data.byteLength !== 12) return false;
  const bytes = new Uint8Array(first.data);
  // Check for "RFB " prefix
  return bytes[0] === 0x52 && bytes[1] === 0x46 && bytes[2] === 0x42 && bytes[3] === 0x20;
}

export function RecordingPlayer({ recordingId, onClose, darkMode }: RecordingPlayerProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const rfbRef = useRef<RFB | null>(null);
  const framesRef = useRef<RecordedFrame[]>([]);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const mockRef = useRef<ReturnType<typeof createMockChannel> | null>(null);

  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [playing, setPlaying] = useState(false);
  const [progress, setProgress] = useState(0); // 0-100
  const [totalDuration, setTotalDuration] = useState(0);
  const [currentTime, setCurrentTime] = useState(0);

  const frameIndexRef = useRef(0);
  const playStartRef = useRef(0);
  const playOffsetRef = useRef(0); // ms offset into recording when play started

  const cancelPlayback = useCallback(() => {
    if (timerRef.current) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }
  }, []);

  const scheduleNextFrame = useCallback(() => {
    const frames = framesRef.current;
    const mock = mockRef.current;
    if (!mock || frameIndexRef.current >= frames.length) {
      setPlaying(false);
      return;
    }

    const frame = frames[frameIndexRef.current];
    const elapsedTarget = frame.timestamp - playOffsetRef.current;
    const elapsedActual = performance.now() - playStartRef.current;
    const delay = Math.max(0, elapsedTarget - elapsedActual);

    timerRef.current = setTimeout(() => {
      if (!frame.fromClient) {
        mock.dispatchMessage(frame.data);
      }

      frameIndexRef.current++;
      setCurrentTime(frame.timestamp);
      if (totalDuration > 0) {
        setProgress(Math.min(100, (frame.timestamp / totalDuration) * 100));
      }

      scheduleNextFrame();
    }, delay);
  }, [totalDuration]);

  // Load recording data
  useEffect(() => {
    let cancelled = false;

    async function load() {
      try {
        const response = await fetchWithAuth(`/api/recordings/${recordingId}/download`);
        if (!response.ok) {
          throw new Error('Failed to download recording');
        }
        const rawBuffer = await response.arrayBuffer();
        if (cancelled) return;

        // Check for VREC header
        const header = parseVrecHeader(rawBuffer);
        const frameData = header ? rawBuffer.slice(header.dataOffset) : rawBuffer;
        const screenWidth = header?.width || 1024;
        const screenHeight = header?.height || 768;

        const frames = parseRecording(frameData);
        if (frames.length === 0) {
          setError('Recording contains no frames');
          setLoading(false);
          return;
        }

        // Only keep server→client frames for replay
        const serverFrames = frames.filter((f) => !f.fromClient);
        framesRef.current = serverFrames;

        const duration = serverFrames[serverFrames.length - 1]?.timestamp ?? 0;
        setTotalDuration(duration);
        setLoading(false);

        // Set up RFB with mock WebSocket
        if (!containerRef.current) return;

        const { default: RFBClass } = await import('@novnc/novnc/lib/rfb.js');
        if (cancelled || !containerRef.current) return;

        const mock = createMockChannel();
        mockRef.current = mock;

        // noVNC RFB accepts a raw channel object as second parameter
        const rfb = new RFBClass(containerRef.current, mock.channel as unknown as string);
        rfb.scaleViewport = true;
        rfb.resizeSession = false;
        rfb.viewOnly = true;
        rfbRef.current = rfb;

        // Trigger WebSocket "open" so noVNC begins protocol negotiation
        mock.dispatchOpen();

        // If the recording doesn't start with the VNC handshake (legacy recordings
        // made after the connection was already established), inject a synthetic
        // handshake so noVNC reaches the connected state before we replay frames.
        if (!startsWithHandshake(serverFrames)) {
          const handshake = buildSyntheticHandshake(screenWidth, screenHeight);
          mock.dispatchMessage(handshake);
        }
        // If the recording includes the handshake (captured from WS creation),
        // noVNC will process it naturally during frame playback.
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : 'Failed to load recording');
          setLoading(false);
        }
      }
    }

    load();

    return () => {
      cancelled = true;
      cancelPlayback();
      if (rfbRef.current) {
        rfbRef.current.disconnect();
        rfbRef.current = null;
      }
    };
  }, [recordingId, cancelPlayback]);

  const handlePlay = useCallback(() => {
    if (playing) {
      cancelPlayback();
      setPlaying(false);
      return;
    }

    if (frameIndexRef.current >= framesRef.current.length) {
      // Restart from beginning
      frameIndexRef.current = 0;
      playOffsetRef.current = 0;
    } else {
      playOffsetRef.current = framesRef.current[frameIndexRef.current]?.timestamp ?? 0;
    }

    playStartRef.current = performance.now();
    setPlaying(true);
    scheduleNextFrame();
  }, [playing, cancelPlayback, scheduleNextFrame]);

  const handleSeek = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    const pct = parseFloat(e.target.value);
    const targetTime = (pct / 100) * totalDuration;

    cancelPlayback();
    setPlaying(false);

    // Find the frame closest to targetTime
    const frames = framesRef.current;
    let idx = 0;
    for (let i = 0; i < frames.length; i++) {
      if (frames[i].timestamp <= targetTime) {
        idx = i;
      } else {
        break;
      }
    }

    frameIndexRef.current = idx;
    setProgress(pct);
    setCurrentTime(targetTime);
  }, [totalDuration, cancelPlayback]);

  const formatTime = (ms: number) => {
    const totalSec = Math.floor(ms / 1000);
    const m = Math.floor(totalSec / 60);
    const s = totalSec % 60;
    return `${m}:${s.toString().padStart(2, '0')}`;
  };

  const bgColor = darkMode ? 'bg-gray-900' : 'bg-gray-100';
  const textColor = darkMode ? 'text-gray-100' : 'text-gray-900';
  const subtextColor = darkMode ? 'text-gray-400' : 'text-gray-500';

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm">
      <div className={`w-full max-w-5xl mx-4 rounded-2xl shadow-2xl overflow-hidden ${bgColor}`}>
        {/* Header */}
        <div className={`flex items-center justify-between px-4 py-3 border-b ${darkMode ? 'border-gray-700' : 'border-gray-200'}`}>
          <h3 className={`text-sm font-medium ${textColor}`}>Recording Playback</h3>
          <button
            onClick={onClose}
            className={`p-1 rounded hover:bg-gray-200 dark:hover:bg-gray-700 ${subtextColor}`}
            aria-label="Close"
          >
            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        {/* Viewer */}
        <div className="relative bg-black" style={{ minHeight: '400px', maxHeight: '70vh' }}>
          {loading && (
            <div className="absolute inset-0 flex items-center justify-center">
              <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-white"></div>
            </div>
          )}
          {error && (
            <div className="absolute inset-0 flex items-center justify-center">
              <p className="text-red-400 text-sm">{error}</p>
            </div>
          )}
          <div ref={containerRef} className="w-full h-full" style={{ minHeight: '400px' }} />
        </div>

        {/* Playback controls */}
        {!loading && !error && (
          <div className={`flex items-center gap-3 px-4 py-3 border-t ${darkMode ? 'border-gray-700' : 'border-gray-200'}`}>
            <button
              onClick={handlePlay}
              className={`p-2 rounded-lg ${darkMode ? 'hover:bg-gray-700' : 'hover:bg-gray-200'} ${textColor}`}
              aria-label={playing ? 'Pause' : 'Play'}
            >
              {playing ? (
                <svg className="w-5 h-5" fill="currentColor" viewBox="0 0 24 24">
                  <path d="M6 4h4v16H6V4zm8 0h4v16h-4V4z" />
                </svg>
              ) : (
                <svg className="w-5 h-5" fill="currentColor" viewBox="0 0 24 24">
                  <path d="M8 5v14l11-7z" />
                </svg>
              )}
            </button>

            <span className={`text-xs font-mono ${subtextColor} min-w-[4rem]`}>
              {formatTime(currentTime)}
            </span>

            <input
              type="range"
              min={0}
              max={100}
              step={0.1}
              value={progress}
              onChange={handleSeek}
              className="flex-1 h-1 accent-blue-500 cursor-pointer"
            />

            <span className={`text-xs font-mono ${subtextColor} min-w-[4rem] text-right`}>
              {formatTime(totalDuration)}
            </span>
          </div>
        )}
      </div>
    </div>
  );
}
