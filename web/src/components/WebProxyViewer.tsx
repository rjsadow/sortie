import { useRef, useEffect, useState } from 'react';

interface WebProxyViewerProps {
  proxyUrl: string;
  appName: string;
  onLoad?: () => void;
  onError?: (message: string) => void;
}

export function WebProxyViewer({ proxyUrl, appName, onLoad, onError }: WebProxyViewerProps) {
  const iframeRef = useRef<HTMLIFrameElement>(null);
  const [loading, setLoading] = useState(true);

  // Construct full URL from proxy path
  const fullUrl = `${window.location.origin}${proxyUrl}`;

  useEffect(() => {
    setLoading(true);
  }, [proxyUrl]);

  const handleLoad = () => {
    setLoading(false);
    onLoad?.();
  };

  const handleError = () => {
    setLoading(false);
    onError?.('Failed to load application');
  };

  return (
    <div className="w-full h-full relative">
      {loading && (
        <div className="absolute inset-0 flex items-center justify-center bg-gray-100 dark:bg-gray-800 z-10">
          <div className="flex flex-col items-center">
            <svg className="w-12 h-12 animate-spin text-blue-500" fill="none" viewBox="0 0 24 24">
              <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
              <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
            </svg>
            <p className="mt-4 text-gray-600 dark:text-gray-300">Loading {appName}...</p>
          </div>
        </div>
      )}
      <iframe
        ref={iframeRef}
        src={fullUrl}
        className="w-full h-full border-0"
        title={appName}
        onLoad={handleLoad}
        onError={handleError}
        sandbox="allow-same-origin allow-scripts allow-forms allow-popups allow-modals allow-downloads"
        allow="clipboard-read; clipboard-write"
      />
    </div>
  );
}
