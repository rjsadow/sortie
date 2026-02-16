import { useState, useEffect, useCallback } from 'react';
import { listRecordings, downloadRecording, deleteRecording } from '../services/auth';
import type { Recording, RecordingStatus } from '../types';

interface RecordingsListProps {
  isOpen: boolean;
  onClose: () => void;
  darkMode: boolean;
}

const STATUS_COLORS: Record<RecordingStatus, { bg: string; text: string; pulse?: boolean }> = {
  recording: { bg: 'bg-red-500', text: 'text-red-500', pulse: true },
  uploading: { bg: 'bg-yellow-500', text: 'text-yellow-500', pulse: true },
  ready: { bg: 'bg-green-500', text: 'text-green-500' },
  failed: { bg: 'bg-gray-400', text: 'text-gray-400' },
};

const STATUS_LABELS: Record<RecordingStatus, string> = {
  recording: 'Recording',
  uploading: 'Uploading',
  ready: 'Ready',
  failed: 'Failed',
};

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`;
}

function formatDuration(seconds: number): string {
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  return `${m}:${s.toString().padStart(2, '0')}`;
}

function formatDate(dateStr: string): string {
  const date = new Date(dateStr);
  if (isNaN(date.getTime())) return dateStr;
  return date.toLocaleString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

export function RecordingsList({ isOpen, onClose, darkMode }: RecordingsListProps) {
  const [recordings, setRecordings] = useState<Recording[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const loadRecordings = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const data = await listRecordings();
      setRecordings(data || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load recordings');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (isOpen) {
      loadRecordings();
    }
  }, [isOpen, loadRecordings]);

  const handleDownload = async (recording: Recording) => {
    try {
      const blobUrl = await downloadRecording(recording.id);
      const a = document.createElement('a');
      a.href = blobUrl;
      a.download = recording.video_path
        ? recording.filename.replace(/\.[^.]+$/, '.mp4')
        : recording.filename;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(blobUrl);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to download recording');
    }
  };

  const handleDelete = async (recording: Recording) => {
    if (!confirm(`Delete recording "${recording.filename}"?`)) return;
    try {
      await deleteRecording(recording.id);
      setRecordings((prev) => prev.filter((r) => r.id !== recording.id));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete recording');
    }
  };

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center bg-black/50 backdrop-blur-sm overflow-y-auto">
      <div className={`w-full max-w-5xl mx-4 my-8 rounded-2xl shadow-2xl ${darkMode ? 'bg-gray-800' : 'bg-white'}`}>
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-gray-200 dark:border-gray-700">
          <div className="flex items-center gap-3">
            <svg className="w-6 h-6 text-brand-accent" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
            </svg>
            <h2 className="text-xl font-bold text-gray-900 dark:text-gray-100">My Recordings</h2>
            <span className="text-sm text-gray-500 dark:text-gray-400">
              {recordings.length} {recordings.length === 1 ? 'recording' : 'recordings'}
            </span>
          </div>
          <div className="flex items-center gap-2">
            <button
              onClick={loadRecordings}
              className="p-2 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors"
              aria-label="Refresh"
              title="Refresh"
            >
              <svg className="w-5 h-5 text-gray-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
              </svg>
            </button>
            <button
              onClick={onClose}
              className="p-2 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors"
              aria-label="Close"
            >
              <svg className="w-5 h-5 text-gray-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
        </div>

        {/* Content */}
        <div className="px-6 py-4">
          {error && (
            <div className="mb-4 p-3 bg-red-500/20 border border-red-500 rounded-lg text-red-500 text-sm">
              {error}
            </div>
          )}

          {loading ? (
            <div className="flex justify-center py-12">
              <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-brand-accent"></div>
            </div>
          ) : recordings.length === 0 ? (
            <div className="text-center py-12">
              <svg className="mx-auto h-12 w-12 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
              </svg>
              <h3 className="mt-2 text-sm font-medium text-gray-900 dark:text-gray-100">No recordings</h3>
              <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
                Record a session to see it here.
              </p>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className={`border-b ${darkMode ? 'border-gray-700' : 'border-gray-200'}`}>
                    <th className="text-left py-2 text-sm text-gray-500 dark:text-gray-400">Status</th>
                    <th className="text-left py-2 text-sm text-gray-500 dark:text-gray-400">Filename</th>
                    <th className="text-left py-2 text-sm text-gray-500 dark:text-gray-400">Duration</th>
                    <th className="text-left py-2 text-sm text-gray-500 dark:text-gray-400">Size</th>
                    <th className="text-left py-2 text-sm text-gray-500 dark:text-gray-400">Date</th>
                    <th className="text-right py-2 text-sm text-gray-500 dark:text-gray-400">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {recordings.map((recording) => {
                    const statusConfig = STATUS_COLORS[recording.status];
                    return (
                      <tr
                        key={recording.id}
                        className={`border-b ${darkMode ? 'border-gray-700' : 'border-gray-200'}`}
                      >
                        <td className="py-3">
                          <div className="flex items-center gap-2">
                            <span className={`inline-block w-2 h-2 rounded-full ${statusConfig.bg} ${statusConfig.pulse ? 'animate-pulse' : ''}`} />
                            <span className={`text-sm ${statusConfig.text}`}>{STATUS_LABELS[recording.status]}</span>
                          </div>
                        </td>
                        <td className={`py-3 text-sm ${darkMode ? 'text-gray-100' : 'text-gray-900'}`}>
                          {recording.filename}
                        </td>
                        <td className={`py-3 text-sm ${darkMode ? 'text-gray-400' : 'text-gray-600'}`}>
                          {formatDuration(recording.duration_seconds)}
                        </td>
                        <td className={`py-3 text-sm ${darkMode ? 'text-gray-400' : 'text-gray-600'}`}>
                          {formatBytes(recording.size_bytes)}
                        </td>
                        <td className={`py-3 text-sm ${darkMode ? 'text-gray-400' : 'text-gray-600'}`}>
                          {formatDate(recording.created_at)}
                        </td>
                        <td className="py-3 text-right">
                          <div className="flex items-center justify-end gap-2">
                            {recording.status === 'ready' && (
                              <button
                                onClick={() => handleDownload(recording)}
                                className="text-brand-accent hover:text-brand-primary text-sm"
                                title={recording.video_path ? 'Download MP4' : 'Download'}
                              >
                                {recording.video_path ? 'Download MP4' : 'Download'}
                              </button>
                            )}
                            <button
                              onClick={() => handleDelete(recording)}
                              className="text-red-500 hover:text-red-400 text-sm"
                              title="Delete"
                            >
                              Delete
                            </button>
                          </div>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>

    </div>
  );
}
