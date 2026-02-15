import { useState, useEffect, useCallback } from 'react';
import { fetchWithAuth } from '../services/auth';
import type { SessionShare } from '../types';

interface ShareSessionDialogProps {
  sessionId: string;
  isOpen: boolean;
  onClose: () => void;
  darkMode: boolean;
}

export function ShareSessionDialog({ sessionId, isOpen, onClose, darkMode }: ShareSessionDialogProps) {
  const [shares, setShares] = useState<SessionShare[]>([]);
  const [username, setUsername] = useState('');
  const [permission, setPermission] = useState<'read_only' | 'read_write'>('read_only');
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [linkUrl, setLinkUrl] = useState<string | null>(null);

  const fetchShares = useCallback(async () => {
    try {
      const res = await fetchWithAuth(`/api/sessions/${sessionId}/shares`);
      if (res.ok) {
        const data = await res.json();
        setShares(data || []);
      }
    } catch {
      // ignore
    }
  }, [sessionId]);

  useEffect(() => {
    if (isOpen) {
      fetchShares();
      setError(null);
      setLinkUrl(null);
    }
  }, [isOpen, fetchShares]);

  const handleInvite = async () => {
    if (!username.trim()) return;
    setIsLoading(true);
    setError(null);
    try {
      const res = await fetchWithAuth(`/api/sessions/${sessionId}/shares`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username: username.trim(), permission }),
      });
      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || 'Failed to invite user');
      }
      setUsername('');
      fetchShares();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to invite user');
    } finally {
      setIsLoading(false);
    }
  };

  const handleGenerateLink = async () => {
    setIsLoading(true);
    setError(null);
    try {
      const res = await fetchWithAuth(`/api/sessions/${sessionId}/shares`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ link_share: true, permission }),
      });
      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || 'Failed to generate link');
      }
      const data = await res.json();
      if (data.share_url) {
        setLinkUrl(window.location.origin + data.share_url);
      }
      fetchShares();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to generate link');
    } finally {
      setIsLoading(false);
    }
  };

  const handleRevoke = async (shareId: string) => {
    try {
      await fetchWithAuth(`/api/sessions/${sessionId}/shares/${shareId}`, {
        method: 'DELETE',
      });
      fetchShares();
    } catch {
      // ignore
    }
  };

  const handleCopyLink = () => {
    if (linkUrl) {
      navigator.clipboard.writeText(linkUrl);
    }
  };

  if (!isOpen) return null;

  const bgColor = darkMode ? 'bg-gray-800' : 'bg-white';
  const textColor = darkMode ? 'text-gray-100' : 'text-gray-900';
  const mutedText = darkMode ? 'text-gray-400' : 'text-gray-600';
  const borderColor = darkMode ? 'border-gray-700' : 'border-gray-200';
  const inputBg = darkMode ? 'bg-gray-700 text-gray-100' : 'bg-gray-50 text-gray-900';

  return (
    <>
      {/* Backdrop */}
      <div
        className="fixed inset-0 bg-black/40 backdrop-blur-sm z-50"
        onClick={onClose}
      />

      {/* Dialog */}
      <div className={`fixed top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 z-50 w-full max-w-md ${bgColor} rounded-xl shadow-2xl border ${borderColor}`}>
        {/* Header */}
        <div className={`flex items-center justify-between px-5 py-4 border-b ${borderColor}`}>
          <h3 className={`text-lg font-semibold ${textColor}`}>Share Session</h3>
          <button
            onClick={onClose}
            className={`p-1 rounded-lg ${darkMode ? 'hover:bg-gray-700' : 'hover:bg-gray-100'}`}
          >
            <svg className={`w-5 h-5 ${textColor}`} fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        <div className="px-5 py-4 space-y-4">
          {error && (
            <div className="p-3 bg-red-500/20 border border-red-500 rounded-lg text-red-500 text-sm">
              {error}
            </div>
          )}

          {/* Permission selector */}
          <div>
            <label htmlFor="share-permission" className={`block text-sm font-medium mb-1 ${mutedText}`}>Permission</label>
            <select
              id="share-permission"
              value={permission}
              onChange={(e) => setPermission(e.target.value as 'read_only' | 'read_write')}
              className={`w-full px-3 py-2 rounded-lg border ${borderColor} ${inputBg}`}
            >
              <option value="read_only">View Only</option>
              <option value="read_write">Full Access</option>
            </select>
          </div>

          {/* Invite by username */}
          <div>
            <label htmlFor="share-username" className={`block text-sm font-medium mb-1 ${mutedText}`}>Invite by Username</label>
            <div className="flex gap-2">
              <input
                id="share-username"
                type="text"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                placeholder="Enter username"
                className={`flex-1 px-3 py-2 rounded-lg border ${borderColor} ${inputBg}`}
                onKeyDown={(e) => e.key === 'Enter' && handleInvite()}
              />
              <button
                onClick={handleInvite}
                disabled={isLoading || !username.trim()}
                className="px-4 py-2 text-sm font-medium text-white bg-brand-accent hover:bg-brand-primary rounded-lg disabled:opacity-50 transition-colors"
              >
                Invite
              </button>
            </div>
          </div>

          {/* Generate link */}
          <div>
            <button
              onClick={handleGenerateLink}
              disabled={isLoading}
              className={`w-full px-4 py-2 text-sm font-medium rounded-lg border ${borderColor} ${darkMode ? 'hover:bg-gray-700' : 'hover:bg-gray-50'} ${textColor} transition-colors disabled:opacity-50`}
            >
              Generate Share Link
            </button>
            {linkUrl && (
              <div className="mt-2 flex gap-2">
                <input
                  readOnly
                  value={linkUrl}
                  className={`flex-1 px-3 py-2 text-sm rounded-lg border ${borderColor} ${inputBg} truncate`}
                />
                <button
                  onClick={handleCopyLink}
                  className="px-3 py-2 text-sm font-medium text-white bg-brand-accent hover:bg-brand-primary rounded-lg transition-colors"
                >
                  Copy
                </button>
              </div>
            )}
          </div>

          {/* Current shares */}
          {shares.length > 0 && (
            <div>
              <h4 className={`text-sm font-medium mb-2 ${mutedText}`}>Current Shares</h4>
              <div className="space-y-2 max-h-48 overflow-y-auto">
                {shares.map((share) => (
                  <div
                    key={share.id}
                    className={`flex items-center justify-between p-2 rounded-lg border ${borderColor}`}
                  >
                    <div>
                      <span className={`text-sm ${textColor}`}>
                        {share.username || (share.share_url ? 'Link share' : share.user_id || 'Unknown')}
                      </span>
                      <span className={`text-xs ml-2 px-1.5 py-0.5 rounded ${
                        share.permission === 'read_write'
                          ? 'bg-green-500/20 text-green-500'
                          : 'bg-blue-500/20 text-blue-500'
                      }`}>
                        {share.permission === 'read_write' ? 'Full Access' : 'View Only'}
                      </span>
                    </div>
                    <button
                      onClick={() => handleRevoke(share.id)}
                      className="text-red-500 hover:text-red-400 text-sm"
                    >
                      Revoke
                    </button>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      </div>
    </>
  );
}
