import { useState, useCallback } from 'react';
import type { Application, ApplicationTemplate } from '../../types';
import { useTemplates, CATEGORY_LABELS } from '../../hooks/useTemplates';
import { TemplateCard } from './TemplateCard';
import { TemplatePreview } from './TemplatePreview';

interface TemplateBrowserProps {
  isOpen: boolean;
  onClose: () => void;
  onAddApp: (app: Application) => Promise<void>;
  darkMode: boolean;
}

export function TemplateBrowser({ isOpen, onClose, onAddApp, darkMode }: TemplateBrowserProps) {
  const {
    categories,
    searchQuery,
    setSearchQuery,
    selectedCategory,
    setSelectedCategory,
    filteredTemplates,
    catalogVersion,
  } = useTemplates();

  const [selectedTemplate, setSelectedTemplate] = useState<ApplicationTemplate | null>(null);

  const handleAddToSortie = useCallback(
    async (app: Application) => {
      await onAddApp(app);
      setSelectedTemplate(null);
      onClose();
    },
    [onAddApp, onClose]
  );

  const handleBackFromPreview = useCallback(() => {
    setSelectedTemplate(null);
  }, []);

  if (!isOpen) return null;

  const bgColor = darkMode ? 'bg-gray-800' : 'bg-white';
  const textColor = darkMode ? 'text-gray-100' : 'text-gray-900';
  const borderColor = darkMode ? 'border-gray-700' : 'border-gray-200';
  const sidebarBg = darkMode ? 'bg-gray-900' : 'bg-gray-50';

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/30 backdrop-blur-sm">
      <div
        className={`relative w-full max-w-5xl h-[85vh] mx-4 rounded-2xl shadow-2xl ${bgColor} flex flex-col overflow-hidden`}
      >
        {/* Header */}
        <div className={`flex items-center justify-between px-6 py-4 border-b ${borderColor}`}>
          <div>
            <h2 className={`text-xl font-bold ${textColor}`}>Template Marketplace</h2>
            <p className={`text-sm ${darkMode ? 'text-gray-400' : 'text-gray-500'}`}>
              Browse and add pre-configured applications (v{catalogVersion})
            </p>
          </div>
          <button
            onClick={onClose}
            className={`p-2 rounded-lg hover:bg-gray-200 dark:hover:bg-gray-700 transition-colors ${textColor}`}
            aria-label="Close"
          >
            <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M6 18L18 6M6 6l12 12"
              />
            </svg>
          </button>
        </div>

        {/* Main content */}
        <div className="flex-1 flex overflow-hidden">
          {/* Show preview if a template is selected */}
          {selectedTemplate ? (
            <TemplatePreview
              template={selectedTemplate}
              onBack={handleBackFromPreview}
              onAddToSortie={handleAddToSortie}
              darkMode={darkMode}
            />
          ) : (
            <>
              {/* Sidebar */}
              <div className={`w-48 flex-shrink-0 ${sidebarBg} border-r ${borderColor} p-4`}>
                <h3 className={`text-xs font-semibold uppercase tracking-wider mb-3 ${darkMode ? 'text-gray-400' : 'text-gray-500'}`}>
                  Categories
                </h3>
                <nav className="space-y-1">
                  <button
                    onClick={() => setSelectedCategory('all')}
                    className={`w-full text-left px-3 py-2 rounded-lg text-sm transition-colors ${
                      selectedCategory === 'all'
                        ? 'bg-brand-accent text-white'
                        : darkMode
                        ? 'text-gray-300 hover:bg-gray-700'
                        : 'text-gray-700 hover:bg-gray-200'
                    }`}
                  >
                    All Templates
                  </button>
                  {categories.map((category) => (
                    <button
                      key={category}
                      onClick={() => setSelectedCategory(category)}
                      className={`w-full text-left px-3 py-2 rounded-lg text-sm transition-colors ${
                        selectedCategory === category
                          ? 'bg-brand-accent text-white'
                          : darkMode
                          ? 'text-gray-300 hover:bg-gray-700'
                          : 'text-gray-700 hover:bg-gray-200'
                      }`}
                    >
                      {CATEGORY_LABELS[category]}
                    </button>
                  ))}
                </nav>
              </div>

              {/* Main content area */}
              <div className="flex-1 flex flex-col overflow-hidden">
                {/* Search bar */}
                <div className={`px-6 py-4 border-b ${borderColor}`}>
                  <div className="relative">
                    <input
                      type="text"
                      placeholder="Search templates by name, description, or tags..."
                      value={searchQuery}
                      onChange={(e) => setSearchQuery(e.target.value)}
                      className={`w-full px-4 py-2 pl-10 rounded-lg border ${borderColor} ${
                        darkMode ? 'bg-gray-700 text-gray-100' : 'bg-white text-gray-900'
                      } placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-brand-accent`}
                    />
                    <svg
                      className="absolute left-3 top-2.5 w-5 h-5 text-gray-400"
                      fill="none"
                      stroke="currentColor"
                      viewBox="0 0 24 24"
                    >
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth={2}
                        d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"
                      />
                    </svg>
                  </div>
                </div>

                {/* Template grid */}
                <div className="flex-1 overflow-y-auto p-6">
                  {filteredTemplates.length === 0 ? (
                    <div className="flex flex-col items-center justify-center h-full">
                      <svg
                        className={`w-12 h-12 ${darkMode ? 'text-gray-600' : 'text-gray-400'}`}
                        fill="none"
                        stroke="currentColor"
                        viewBox="0 0 24 24"
                      >
                        <path
                          strokeLinecap="round"
                          strokeLinejoin="round"
                          strokeWidth={2}
                          d="M9.172 16.172a4 4 0 015.656 0M9 10h.01M15 10h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
                        />
                      </svg>
                      <p className={`mt-4 text-sm ${darkMode ? 'text-gray-400' : 'text-gray-500'}`}>
                        No templates found matching your search.
                      </p>
                    </div>
                  ) : (
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                      {filteredTemplates.map((template) => (
                        <TemplateCard
                          key={template.template_id}
                          template={template}
                          onClick={() => setSelectedTemplate(template)}
                          darkMode={darkMode}
                        />
                      ))}
                    </div>
                  )}
                </div>

                {/* Footer with count */}
                <div className={`px-6 py-3 border-t ${borderColor}`}>
                  <p className={`text-sm ${darkMode ? 'text-gray-400' : 'text-gray-500'}`}>
                    {filteredTemplates.length} template{filteredTemplates.length !== 1 ? 's' : ''}{' '}
                    {selectedCategory !== 'all' && `in ${CATEGORY_LABELS[selectedCategory]}`}
                  </p>
                </div>
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
