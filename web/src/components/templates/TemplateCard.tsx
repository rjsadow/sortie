import type { ApplicationTemplate } from '../../types';

interface TemplateCardProps {
  template: ApplicationTemplate;
  onClick: () => void;
  darkMode: boolean;
}

export function TemplateCard({ template, onClick, darkMode }: TemplateCardProps) {
  return (
    <button
      onClick={onClick}
      className={`group text-left w-full p-4 rounded-lg border transition-all duration-200 hover:shadow-md ${
        darkMode
          ? 'bg-gray-700 border-gray-600 hover:border-brand-accent'
          : 'bg-gray-50 border-gray-200 hover:border-brand-accent'
      }`}
    >
      <div className="flex items-start gap-3">
        <div
          className={`flex-shrink-0 w-12 h-12 rounded-lg flex items-center justify-center overflow-hidden relative ${
            darkMode ? 'bg-gray-600' : 'bg-white'
          }`}
        >
          <img
            src={template.icon}
            alt={`${template.name} icon`}
            className="w-8 h-8 object-contain"
            onError={(e) => {
              (e.target as HTMLImageElement).src =
                'data:image/svg+xml,<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="%23636A51"><rect width="24" height="24" rx="4"/><text x="12" y="16" text-anchor="middle" fill="white" font-size="12">' +
                template.name.charAt(0) +
                '</text></svg>';
            }}
          />
          {/* Container badge */}
          <div
            className="absolute -top-1 -right-1 w-4 h-4 bg-blue-500 rounded-full flex items-center justify-center"
            title="Container App"
          >
            <svg className="w-2.5 h-2.5 text-white" fill="currentColor" viewBox="0 0 24 24">
              <path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z" />
            </svg>
          </div>
        </div>

        <div className="flex-1 min-w-0">
          <h3
            className={`text-sm font-medium truncate group-hover:text-brand-accent ${
              darkMode ? 'text-gray-100' : 'text-gray-900'
            }`}
          >
            {template.name}
          </h3>
          <p
            className={`mt-1 text-xs line-clamp-2 ${
              darkMode ? 'text-gray-400' : 'text-gray-500'
            }`}
          >
            {template.description}
          </p>

          {/* Tags */}
          <div className="mt-2 flex flex-wrap gap-1">
            {template.tags.slice(0, 3).map((tag) => (
              <span
                key={tag}
                className={`inline-block px-1.5 py-0.5 text-xs rounded ${
                  darkMode
                    ? 'bg-gray-600 text-gray-300'
                    : 'bg-gray-200 text-gray-600'
                }`}
              >
                {tag}
              </span>
            ))}
            {template.tags.length > 3 && (
              <span
                className={`inline-block px-1.5 py-0.5 text-xs rounded ${
                  darkMode
                    ? 'bg-gray-600 text-gray-400'
                    : 'bg-gray-200 text-gray-500'
                }`}
              >
                +{template.tags.length - 3}
              </span>
            )}
          </div>
        </div>

        <svg
          className={`w-4 h-4 flex-shrink-0 group-hover:text-brand-accent ${
            darkMode ? 'text-gray-500' : 'text-gray-400'
          }`}
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            strokeWidth={2}
            d="M9 5l7 7-7 7"
          />
        </svg>
      </div>
    </button>
  );
}
