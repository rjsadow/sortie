import { useState, useEffect, useRef, useCallback } from 'react';
import type { Application } from '../types';

interface CommandPaletteProps {
  isOpen: boolean;
  onClose: () => void;
  apps: Application[];
  isAdmin: boolean;
  darkMode: boolean;
  onLaunchApp: (app: Application) => void;
  onOpenTemplates: () => void;
  onToggleDarkMode: () => void;
  onOpenAdmin: () => void;
  onOpenAuditLog: () => void;
}

interface PaletteItem {
  id: string;
  label: string;
  description?: string;
  icon: React.ReactNode;
  section: string;
  onSelect: () => void;
}

export function CommandPalette({
  isOpen,
  onClose,
  apps,
  isAdmin,
  darkMode,
  onLaunchApp,
  onOpenTemplates,
  onToggleDarkMode,
  onOpenAdmin,
  onOpenAuditLog,
}: CommandPaletteProps) {
  const [query, setQuery] = useState('');
  const [selectedIndex, setSelectedIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const previousFocusRef = useRef<HTMLElement | null>(null);

  const buildItems = useCallback((): PaletteItem[] => {
    const q = query.toLowerCase();
    const items: PaletteItem[] = [];

    // Applications section
    const matchedApps = query
      ? apps.filter(
          (app) =>
            app.name.toLowerCase().includes(q) ||
            app.description.toLowerCase().includes(q) ||
            app.category.toLowerCase().includes(q)
        )
      : apps.slice(0, 5);

    matchedApps.forEach((app) => {
      items.push({
        id: `app-${app.id}`,
        label: app.name,
        description: app.description,
        section: 'Applications',
        icon: (
          <img
            src={app.icon}
            alt=""
            className="w-5 h-5 object-contain"
            onError={(e) => {
              (e.target as HTMLImageElement).style.display = 'none';
              (e.target as HTMLImageElement).nextElementSibling?.classList.remove('hidden');
            }}
          />
        ),
        onSelect: () => onLaunchApp(app),
      });
    });

    // Actions section
    const actions: PaletteItem[] = [
      {
        id: 'action-templates',
        label: 'Browse Templates',
        description: 'Add pre-configured applications',
        section: 'Actions',
        icon: (
          <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 5a1 1 0 011-1h14a1 1 0 011 1v2a1 1 0 01-1 1H5a1 1 0 01-1-1V5zM4 13a1 1 0 011-1h6a1 1 0 011 1v6a1 1 0 01-1 1H5a1 1 0 01-1-1v-6zM16 13a1 1 0 011-1h2a1 1 0 011 1v6a1 1 0 01-1 1h-2a1 1 0 01-1-1v-6z" />
          </svg>
        ),
        onSelect: onOpenTemplates,
      },
      {
        id: 'action-darkmode',
        label: darkMode ? 'Switch to Light Mode' : 'Switch to Dark Mode',
        description: 'Toggle appearance',
        section: 'Actions',
        icon: darkMode ? (
          <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 3v1m0 16v1m9-9h-1M4 12H3m15.364 6.364l-.707-.707M6.343 6.343l-.707-.707m12.728 0l-.707.707M6.343 17.657l-.707.707M16 12a4 4 0 11-8 0 4 4 0 018 0z" />
          </svg>
        ) : (
          <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z" />
          </svg>
        ),
        onSelect: onToggleDarkMode,
      },
      {
        id: 'action-docs',
        label: 'Documentation',
        description: 'Open docs in new tab',
        section: 'Actions',
        icon: (
          <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253" />
          </svg>
        ),
        onSelect: () => window.open('/docs/', '_blank', 'noopener,noreferrer'),
      },
    ];

    const filteredActions = query
      ? actions.filter((a) => a.label.toLowerCase().includes(q) || a.description?.toLowerCase().includes(q))
      : actions;

    items.push(...filteredActions);

    // Admin section
    if (isAdmin) {
      const adminItems: PaletteItem[] = [
        {
          id: 'admin-panel',
          label: 'Admin Panel',
          description: 'Manage users and applications',
          section: 'Admin',
          icon: (
            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
            </svg>
          ),
          onSelect: onOpenAdmin,
        },
        {
          id: 'admin-audit',
          label: 'Audit Log',
          description: 'View system audit trail',
          section: 'Admin',
          icon: (
            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
            </svg>
          ),
          onSelect: onOpenAuditLog,
        },
      ];

      const filteredAdmin = query
        ? adminItems.filter((a) => a.label.toLowerCase().includes(q) || a.description?.toLowerCase().includes(q))
        : adminItems;

      items.push(...filteredAdmin);
    }

    return items;
  }, [query, apps, isAdmin, darkMode, onLaunchApp, onOpenTemplates, onToggleDarkMode, onOpenAdmin, onOpenAuditLog]);

  const items = buildItems();

  // Reset selection when query changes
  useEffect(() => {
    setSelectedIndex(0);
  }, [query]);

  // Focus input on open, restore focus on close
  useEffect(() => {
    if (isOpen) {
      previousFocusRef.current = document.activeElement as HTMLElement;
      // Small delay to allow animation to start
      requestAnimationFrame(() => inputRef.current?.focus());
    } else {
      setQuery('');
      setSelectedIndex(0);
      previousFocusRef.current?.focus();
    }
  }, [isOpen]);

  // Scroll selected item into view
  useEffect(() => {
    if (!listRef.current) return;
    const selected = listRef.current.querySelector('[data-selected="true"]');
    selected?.scrollIntoView({ block: 'nearest' });
  }, [selectedIndex]);

  // Global Escape handler — works regardless of focus position
  useEffect(() => {
    if (!isOpen) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        onClose();
      }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [isOpen, onClose]);

  const handleKeyDown = (e: React.KeyboardEvent) => {
    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault();
        setSelectedIndex((prev) => (prev < items.length - 1 ? prev + 1 : 0));
        break;
      case 'ArrowUp':
        e.preventDefault();
        setSelectedIndex((prev) => (prev > 0 ? prev - 1 : items.length - 1));
        break;
      case 'Enter':
        e.preventDefault();
        if (items[selectedIndex]) {
          items[selectedIndex].onSelect();
          onClose();
        }
        break;
    }
  };

  if (!isOpen) return null;

  // Group items by section for rendering
  const sections: { name: string; items: (PaletteItem & { flatIndex: number })[] }[] = [];
  let flatIndex = 0;
  items.forEach((item) => {
    let section = sections.find((s) => s.name === item.section);
    if (!section) {
      section = { name: item.section, items: [] };
      sections.push(section);
    }
    section.items.push({ ...item, flatIndex: flatIndex++ });
  });

  return (
    <div className="fixed inset-0 z-50" onKeyDown={handleKeyDown}>
      {/* Backdrop */}
      <div
        className="absolute inset-0 bg-black/30 backdrop-blur-sm"
        onClick={onClose}
      />

      {/* Modal */}
      <div className="relative flex justify-center" style={{ paddingTop: '15vh' }}>
        <div className="w-full max-w-xl mx-4 bg-white dark:bg-gray-800 rounded-2xl shadow-2xl border border-gray-200 dark:border-gray-700 overflow-hidden animate-palette-in">
          {/* Search bar */}
          <div className="flex items-center gap-3 px-4 py-3 border-b border-gray-200 dark:border-gray-700">
            <svg className="w-5 h-5 text-gray-400 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
            </svg>
            <input
              ref={inputRef}
              type="text"
              placeholder="Search apps and actions..."
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              className="flex-1 bg-transparent text-gray-900 dark:text-gray-100 placeholder-gray-400 dark:placeholder-gray-500 outline-none text-sm"
            />
            <kbd className="hidden sm:inline-flex items-center px-1.5 py-0.5 text-[10px] font-medium text-gray-400 dark:text-gray-500 bg-gray-100 dark:bg-gray-700 rounded border border-gray-200 dark:border-gray-600">
              ESC
            </kbd>
          </div>

          {/* Results */}
          <div ref={listRef} className="max-h-80 overflow-y-auto" role="listbox">
            {items.length === 0 ? (
              <div className="px-4 py-8 text-center text-sm text-gray-500 dark:text-gray-400">
                No results found
              </div>
            ) : (
              sections.map((section) => (
                <div key={section.name}>
                  <div className="px-4 pt-3 pb-1">
                    <span className="text-[11px] font-semibold text-gray-400 dark:text-gray-500 uppercase tracking-wider">
                      {section.name}
                    </span>
                  </div>
                  {section.items.map((item) => (
                    <button
                      key={item.id}
                      role="option"
                      aria-selected={item.flatIndex === selectedIndex}
                      data-selected={item.flatIndex === selectedIndex}
                      className={`w-full flex items-center gap-3 px-4 py-2.5 text-left transition-colors ${
                        item.flatIndex === selectedIndex
                          ? 'bg-brand-accent/10 dark:bg-brand-accent/20 text-brand-accent'
                          : 'text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-700/50'
                      }`}
                      onClick={() => {
                        item.onSelect();
                        onClose();
                      }}
                      onMouseEnter={() => setSelectedIndex(item.flatIndex)}
                    >
                      <div className={`w-8 h-8 flex items-center justify-center rounded-lg flex-shrink-0 ${
                        item.flatIndex === selectedIndex
                          ? 'bg-brand-accent/10 dark:bg-brand-accent/20'
                          : 'bg-gray-100 dark:bg-gray-700'
                      }`}>
                        {item.icon}
                        {/* Fallback initial for app icons */}
                        {item.section === 'Applications' && (
                          <span className="hidden text-xs font-bold text-gray-500 dark:text-gray-400">
                            {item.label.charAt(0)}
                          </span>
                        )}
                      </div>
                      <div className="flex-1 min-w-0">
                        <div className="text-sm font-medium truncate">{item.label}</div>
                        {item.description && (
                          <div className="text-xs text-gray-500 dark:text-gray-400 truncate">
                            {item.description}
                          </div>
                        )}
                      </div>
                    </button>
                  ))}
                </div>
              ))
            )}
          </div>

          {/* Footer hints */}
          <div className="flex items-center gap-4 px-4 py-2.5 border-t border-gray-200 dark:border-gray-700 text-[11px] text-gray-400 dark:text-gray-500">
            <span className="flex items-center gap-1">
              <kbd className="px-1 py-0.5 bg-gray-100 dark:bg-gray-700 rounded border border-gray-200 dark:border-gray-600 font-mono">&#8593;&#8595;</kbd>
              navigate
            </span>
            <span className="flex items-center gap-1">
              <kbd className="px-1 py-0.5 bg-gray-100 dark:bg-gray-700 rounded border border-gray-200 dark:border-gray-600 font-mono">&#9166;</kbd>
              select
            </span>
            <span className="flex items-center gap-1">
              <kbd className="px-1 py-0.5 bg-gray-100 dark:bg-gray-700 rounded border border-gray-200 dark:border-gray-600 font-mono">esc</kbd>
              close
            </span>
          </div>
        </div>
      </div>
    </div>
  );
}
