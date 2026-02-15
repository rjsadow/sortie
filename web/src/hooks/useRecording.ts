import { useState, useRef, useCallback } from 'react';

interface UseRecordingReturn {
  isRecording: boolean;
  duration: number;
  startRecording: (canvas: HTMLCanvasElement, sessionId: string) => Promise<void>;
  stopRecording: () => Promise<void>;
  error: string | null;
}

export function useRecording(): UseRecordingReturn {
  const [isRecording, setIsRecording] = useState(false);
  const [duration, setDuration] = useState(0);
  const [error, setError] = useState<string | null>(null);

  const mediaRecorderRef = useRef<MediaRecorder | null>(null);
  const chunksRef = useRef<Blob[]>([]);
  const recordingIdRef = useRef<string>('');
  const sessionIdRef = useRef<string>('');
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const startTimeRef = useRef<number>(0);

  const startRecording = useCallback(async (canvas: HTMLCanvasElement, sessionId: string) => {
    setError(null);
    sessionIdRef.current = sessionId;

    // Check browser support
    if (!canvas.captureStream || !window.MediaRecorder) {
      setError('Browser does not support recording');
      return;
    }

    try {
      // Create recording record on server
      const res = await fetch(`/api/sessions/${sessionId}/recording/start`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${localStorage.getItem('access_token')}`,
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

      // Capture canvas stream at 10 FPS
      const stream = canvas.captureStream(10);

      // Try VP9 first, fall back to VP8
      const mimeType = MediaRecorder.isTypeSupported('video/webm;codecs=vp9')
        ? 'video/webm;codecs=vp9'
        : 'video/webm;codecs=vp8';

      const recorder = new MediaRecorder(stream, { mimeType });
      chunksRef.current = [];

      recorder.ondataavailable = (e) => {
        if (e.data.size > 0) {
          chunksRef.current.push(e.data);
        }
      };

      recorder.onerror = () => {
        setError('Recording error');
        setIsRecording(false);
      };

      recorder.start(1000); // Collect chunks every second
      mediaRecorderRef.current = recorder;
      startTimeRef.current = Date.now();
      setIsRecording(true);
      setDuration(0);

      // Update duration counter
      timerRef.current = setInterval(() => {
        setDuration(Math.floor((Date.now() - startTimeRef.current) / 1000));
      }, 1000);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to start recording');
    }
  }, []);

  const stopRecording = useCallback(async () => {
    if (!mediaRecorderRef.current || mediaRecorderRef.current.state === 'inactive') {
      return;
    }

    // Clear timer
    if (timerRef.current) {
      clearInterval(timerRef.current);
      timerRef.current = null;
    }

    const recordingId = recordingIdRef.current;
    const sessionId = sessionIdRef.current;
    const elapsedSeconds = (Date.now() - startTimeRef.current) / 1000;

    // Stop the recorder and wait for final data
    await new Promise<void>((resolve) => {
      const recorder = mediaRecorderRef.current!;
      recorder.onstop = () => resolve();
      recorder.stop();
    });

    setIsRecording(false);
    mediaRecorderRef.current = null;

    // Notify server recording stopped
    try {
      await fetch(`/api/sessions/${sessionId}/recording/stop`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${localStorage.getItem('access_token')}`,
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ recording_id: recordingId }),
      });
    } catch {
      // Non-fatal: upload will still work
    }

    // Assemble blob and upload
    const blob = new Blob(chunksRef.current, { type: 'video/webm' });
    chunksRef.current = [];

    const formData = new FormData();
    formData.append('recording_id', recordingId);
    formData.append('duration', elapsedSeconds.toFixed(1));
    formData.append('file', blob, `${recordingId}.webm`);

    try {
      const res = await fetch(`/api/sessions/${sessionId}/recording/upload`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${localStorage.getItem('access_token')}`,
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

  return { isRecording, duration, startRecording, stopRecording, error };
}
