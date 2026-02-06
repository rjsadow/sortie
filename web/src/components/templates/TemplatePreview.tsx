import { useState, useCallback } from 'react';
import type { Application, ApplicationTemplate } from '../../types';

interface TemplatePreviewProps {
  template: ApplicationTemplate;
  onBack: () => void;
  onAddToSortie: (app: Application) => Promise<void>;
  darkMode: boolean;
}

export function TemplatePreview({
  template,
  onBack,
  onAddToSortie,
  darkMode,
}: TemplatePreviewProps) {
  const [customId, setCustomId] = useState(`app-${template.template_id}`);
  const [isAdding, setIsAdding] = useState(false);
  const [copySuccess, setCopySuccess] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const generateApplication = useCallback((): Application => {
    return {
      id: customId,
      name: template.name,
      description: template.description,
      url: template.url,
      icon: template.icon,
      category: template.category,
      launch_type: template.launch_type,
      container_image: template.container_image,
      container_port: template.container_port,
      container_args: template.container_args,
      resource_limits: template.recommended_limits,
    };
  }, [customId, template]);

  const handleAddToSortie = async () => {
    setIsAdding(true);
    setError(null);
    try {
      const app = generateApplication();
      await onAddToSortie(app);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to add application');
    } finally {
      setIsAdding(false);
    }
  };

  const handleCopyJson = async () => {
    const app = generateApplication();
    try {
      await navigator.clipboard.writeText(JSON.stringify(app, null, 2));
      setCopySuccess(true);
      setTimeout(() => setCopySuccess(false), 2000);
    } catch {
      setError('Failed to copy to clipboard');
    }
  };

  const bgColor = darkMode ? 'bg-gray-800' : 'bg-white';
  const textColor = darkMode ? 'text-gray-100' : 'text-gray-900';
  const secondaryTextColor = darkMode ? 'text-gray-400' : 'text-gray-500';
  const borderColor = darkMode ? 'border-gray-700' : 'border-gray-200';
  const inputBg = darkMode ? 'bg-gray-700' : 'bg-gray-50';

  return (
    <div className={`h-full flex flex-col ${bgColor}`}>
      {/* Header with back button */}
      <div className={`flex items-center gap-3 px-6 py-4 border-b ${borderColor}`}>
        <button
          onClick={onBack}
          className={`p-2 rounded-lg hover:bg-gray-200 dark:hover:bg-gray-700 transition-colors ${textColor}`}
          aria-label="Go back"
        >
          <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M15 19l-7-7 7-7"
            />
          </svg>
        </button>
        <h2 className={`text-lg font-semibold ${textColor}`}>Template Details</h2>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-y-auto p-6">
        {/* Template header */}
        <div className="flex items-start gap-4 mb-6">
          <div
            className={`w-16 h-16 rounded-lg flex items-center justify-center overflow-hidden ${
              darkMode ? 'bg-gray-700' : 'bg-gray-100'
            }`}
          >
            <img
              src={template.icon}
              alt={`${template.name} icon`}
              className="w-12 h-12 object-contain"
              onError={(e) => {
                (e.target as HTMLImageElement).src =
                  'data:image/svg+xml,<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="%23636A51"><rect width="24" height="24" rx="4"/><text x="12" y="16" text-anchor="middle" fill="white" font-size="12">' +
                  template.name.charAt(0) +
                  '</text></svg>';
              }}
            />
          </div>
          <div>
            <h3 className={`text-xl font-bold ${textColor}`}>{template.name}</h3>
            <p className={`mt-1 ${secondaryTextColor}`}>{template.description}</p>
            {template.maintainer && (
              <p className={`mt-1 text-sm ${secondaryTextColor}`}>
                Maintained by {template.maintainer}
              </p>
            )}
          </div>
        </div>

        {/* Tags */}
        <div className="mb-6">
          <h4 className={`text-sm font-medium mb-2 ${textColor}`}>Tags</h4>
          <div className="flex flex-wrap gap-2">
            {template.tags.map((tag) => (
              <span
                key={tag}
                className={`inline-block px-2 py-1 text-sm rounded ${
                  darkMode ? 'bg-gray-700 text-gray-300' : 'bg-gray-100 text-gray-700'
                }`}
              >
                {tag}
              </span>
            ))}
          </div>
        </div>

        {/* Container details */}
        <div className={`mb-6 p-4 rounded-lg ${darkMode ? 'bg-gray-700' : 'bg-gray-50'}`}>
          <h4 className={`text-sm font-medium mb-3 ${textColor}`}>Container Configuration</h4>
          <div className="space-y-2">
            <div className="flex justify-between text-sm">
              <span className={secondaryTextColor}>Image:</span>
              <code className={`${darkMode ? 'text-blue-400' : 'text-blue-600'}`}>
                {template.container_image}
              </code>
            </div>
            <div className="flex justify-between text-sm">
              <span className={secondaryTextColor}>Category:</span>
              <span className={textColor}>{template.category}</span>
            </div>
            <div className="flex justify-between text-sm">
              <span className={secondaryTextColor}>Version:</span>
              <span className={textColor}>{template.template_version}</span>
            </div>
          </div>
        </div>

        {/* Resource limits */}
        {template.recommended_limits && (
          <div className={`mb-6 p-4 rounded-lg ${darkMode ? 'bg-gray-700' : 'bg-gray-50'}`}>
            <h4 className={`text-sm font-medium mb-3 ${textColor}`}>Recommended Resource Limits</h4>
            <div className="grid grid-cols-2 gap-4">
              <div>
                <p className={`text-xs ${secondaryTextColor}`}>CPU Request</p>
                <p className={`text-sm font-medium ${textColor}`}>
                  {template.recommended_limits.cpu_request || 'Not set'}
                </p>
              </div>
              <div>
                <p className={`text-xs ${secondaryTextColor}`}>CPU Limit</p>
                <p className={`text-sm font-medium ${textColor}`}>
                  {template.recommended_limits.cpu_limit || 'Not set'}
                </p>
              </div>
              <div>
                <p className={`text-xs ${secondaryTextColor}`}>Memory Request</p>
                <p className={`text-sm font-medium ${textColor}`}>
                  {template.recommended_limits.memory_request || 'Not set'}
                </p>
              </div>
              <div>
                <p className={`text-xs ${secondaryTextColor}`}>Memory Limit</p>
                <p className={`text-sm font-medium ${textColor}`}>
                  {template.recommended_limits.memory_limit || 'Not set'}
                </p>
              </div>
            </div>
          </div>
        )}

        {/* Documentation link */}
        {template.documentation_url && (
          <div className="mb-6">
            <a
              href={template.documentation_url}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-2 text-sm text-brand-accent hover:text-brand-accent transition-colors"
            >
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"
                />
              </svg>
              View Documentation
            </a>
          </div>
        )}

        {/* Custom ID input */}
        <div className="mb-6">
          <label className={`block text-sm font-medium mb-2 ${textColor}`}>
            Application ID
          </label>
          <input
            type="text"
            value={customId}
            onChange={(e) => setCustomId(e.target.value)}
            className={`w-full px-3 py-2 rounded-lg border ${borderColor} ${inputBg} ${textColor} focus:outline-none focus:ring-2 focus:ring-brand-accent`}
            placeholder="Enter a unique ID for the application"
          />
          <p className={`mt-1 text-xs ${secondaryTextColor}`}>
            This ID must be unique among all applications in your Sortie instance.
          </p>
        </div>

        {/* Error message */}
        {error && (
          <div className="mb-4 p-3 rounded-lg bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-300 text-sm">
            {error}
          </div>
        )}
      </div>

      {/* Footer with actions */}
      <div className={`flex items-center justify-end gap-3 px-6 py-4 border-t ${borderColor}`}>
        <button
          onClick={handleCopyJson}
          className={`px-4 py-2 rounded-lg transition-colors ${
            copySuccess
              ? 'bg-green-500 text-white'
              : darkMode
              ? 'bg-gray-700 hover:bg-gray-600 text-gray-200'
              : 'bg-gray-200 hover:bg-gray-300 text-gray-800'
          }`}
        >
          {copySuccess ? 'Copied!' : 'Copy JSON'}
        </button>
        <button
          onClick={handleAddToSortie}
          disabled={isAdding || !customId.trim()}
          className="px-4 py-2 rounded-lg bg-brand-accent text-white hover:bg-brand-primary transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {isAdding ? 'Adding...' : 'Add to Sortie'}
        </button>
      </div>
    </div>
  );
}
