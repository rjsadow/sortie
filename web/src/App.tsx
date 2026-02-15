import { useState, useEffect, useRef, useCallback } from 'react';
import type { Application, User } from './types';
import { useSessions } from './hooks/useSessions';
import { SessionPage } from './components/SessionPage';
import { Login } from './components/Login';
import { Register } from './components/Register';
import { Admin } from './components/Admin';
import { AuditLog } from './components/AuditLog';
import { SessionManager } from './components/SessionManager';
import { RecordingsList } from './components/RecordingsList';
import { TemplateBrowser } from './components/templates/TemplateBrowser';
import {
  getStoredUser,
  setStoredUser,
  logout as authLogout,
  getCurrentUser,
  isAuthenticated,
  fetchWithAuth
} from './services/auth';
import { CommandPalette } from './components/CommandPalette';
import { UserMenu } from './components/UserMenu';
import sortieIconWhite from './assets/sortie-icon-white.svg';

function App() {
  const [user, setUser] = useState<User | null>(() => getStoredUser());
  const [authLoading, setAuthLoading] = useState(true);
  const [apps, setApps] = useState<Application[]>([]);
  const [isCommandPaletteOpen, setIsCommandPaletteOpen] = useState(false);
  const [loading, setLoading] = useState(true);
  const [darkMode, setDarkMode] = useState(() => {
    const stored = localStorage.getItem('sortie-theme');
    if (stored) return stored === 'dark';
    return window.matchMedia('(prefers-color-scheme: dark)').matches;
  });
  const [collapsedCategories, setCollapsedCategories] = useState<Set<string>>(() => {
    const stored = localStorage.getItem('sortie-collapsed');
    return stored ? new Set(JSON.parse(stored)) : new Set();
  });
  const [selectedCategory, setSelectedCategory] = useState<string | null>(() => {
    const stored = localStorage.getItem('sortie-category-filter');
    return stored && stored !== 'null' ? stored : null;
  });
  const [favorites, setFavorites] = useState<Set<string>>(() => {
    const stored = localStorage.getItem('sortie-favorites');
    return stored ? new Set(JSON.parse(stored)) : new Set();
  });
  const [recentApps, setRecentApps] = useState<string[]>(() => {
    const stored = localStorage.getItem('sortie-recents');
    return stored ? JSON.parse(stored) : [];
  });
  const [focusedIndex, setFocusedIndex] = useState(-1);
  const [selectedContainerApp, setSelectedContainerApp] = useState<Application | null>(null);
  const [reconnectSessionId, setReconnectSessionId] = useState<string | null>(null);
  const [sessionShareInfo, setSessionShareInfo] = useState<{ viewOnly: boolean; ownerUsername?: string; sharePermission?: string } | null>(null);
  const [isTemplateBrowserOpen, setIsTemplateBrowserOpen] = useState(false);
  const [showRegister, setShowRegister] = useState(false);
  const [showAdmin, setShowAdmin] = useState(false);
  const [showAuditLog, setShowAuditLog] = useState(false);
  const [showSessionManager, setShowSessionManager] = useState(false);
  const [showRecordings, setShowRecordings] = useState(false);
  const [allowRegistration, setAllowRegistration] = useState(false);
  const [ssoEnabled, setSsoEnabled] = useState(false);
  const [showKeyboardHint, setShowKeyboardHint] = useState(false);
  const appRefs = useRef<(HTMLButtonElement | HTMLAnchorElement | null)[]>([]);

  const { sessions } = useSessions(true);
  const activeSessionCount = sessions.filter(
    (s) => s.status === 'creating' || s.status === 'running'
  ).length;

  // Validate token on app load and fetch config
  useEffect(() => {
    const validateAuth = async () => {
      // Fetch config for registration setting
      try {
        const configRes = await fetch('/api/config');
        if (configRes.ok) {
          const config = await configRes.json();
          setAllowRegistration(config.allow_registration === true);
          setSsoEnabled(config.sso_enabled === true);
        }
      } catch {
        // Ignore config fetch errors
      }

      if (isAuthenticated()) {
        try {
          const currentUser = await getCurrentUser();
          if (currentUser) {
            setUser(currentUser);
          } else {
            setUser(null);
          }
        } catch {
          setUser(null);
        }
      }
      setAuthLoading(false);
    };

    validateAuth();
  }, []);

  // Handle share link URLs: /session/{id}?share_token={token}
  useEffect(() => {
    if (authLoading || !user) return;

    const path = window.location.pathname;
    const params = new URLSearchParams(window.location.search);
    const shareToken = params.get('share_token');

    const match = path.match(/^\/session\/([^/]+)$/);
    if (!match || !shareToken) return;

    const shareSessionId = match[1];

    // Clear the URL immediately to prevent re-processing on re-renders
    window.history.replaceState({}, '', '/');

    const joinShare = async () => {
      try {
        const response = await fetchWithAuth('/api/sessions/shares/join', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ token: shareToken }),
        });

        if (!response.ok) {
          console.error('Failed to join shared session:', response.statusText);
          return;
        }

        const session = await response.json();

        // Construct a minimal app object from the join response.
        // The shared user may not have this app in their own apps list.
        const app: Application = {
          id: session.app_id,
          name: session.app_name || 'Shared Session',
          description: '',
          url: '',
          icon: '',
          category: '',
          launch_type: 'container',
        };

        setReconnectSessionId(shareSessionId);
        setSessionShareInfo({
          viewOnly: session.share_permission === 'read_only',
          ownerUsername: session.owner_username,
          sharePermission: session.share_permission,
        });
        setSelectedContainerApp(app);
      } catch (err) {
        console.error('Failed to join shared session:', err);
      }
    };

    joinShare();
  }, [authLoading, user]);

  // Fetch apps after authentication is validated
  useEffect(() => {
    if (authLoading || !user) return;

    const loadApps = async () => {
      try {
        const response = await fetchWithAuth('/api/apps');
        if (response.ok) {
          const data = await response.json();
          setApps(data);
        } else {
          console.error('Failed to load apps:', response.statusText);
        }
      } catch (err) {
        console.error('Failed to load apps:', err);
      } finally {
        setLoading(false);
      }
    };

    loadApps();
  }, [authLoading, user]);

  useEffect(() => {
    document.documentElement.classList.toggle('dark', darkMode);
    localStorage.setItem('sortie-theme', darkMode ? 'dark' : 'light');
  }, [darkMode]);

  useEffect(() => {
    localStorage.setItem('sortie-collapsed', JSON.stringify([...collapsedCategories]));
  }, [collapsedCategories]);

  useEffect(() => {
    localStorage.setItem('sortie-category-filter', selectedCategory || 'null');
  }, [selectedCategory]);

  useEffect(() => {
    localStorage.setItem('sortie-favorites', JSON.stringify([...favorites]));
  }, [favorites]);

  useEffect(() => {
    localStorage.setItem('sortie-recents', JSON.stringify(recentApps));
  }, [recentApps]);

  // Get all unique categories from all apps (before search filtering)
  const allCategories = [...new Set(apps.map((app) => app.category))].sort();

  const filteredApps = apps.filter((app) => {
    return selectedCategory === null || app.category === selectedCategory;
  });

  const categories = [...new Set(filteredApps.map((app) => app.category))];

  // Get visible apps (not in collapsed categories)
  const visibleApps = filteredApps.filter(
    (app) => !collapsedCategories.has(app.category)
  );

  // Get favorite apps (filtered by search)
  const favoriteApps = filteredApps.filter((app) => favorites.has(app.id));

  // Get recent apps (filtered by search, maintaining order)
  const recentAppsList = recentApps
    .map((id) => filteredApps.find((app) => app.id === id))
    .filter((app): app is Application => app !== undefined);

  const toggleCategory = (category: string) => {
    setCollapsedCategories((prev) => {
      const next = new Set(prev);
      if (next.has(category)) {
        next.delete(category);
      } else {
        next.add(category);
      }
      return next;
    });
  };

  const toggleFavorite = (appId: string, e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setFavorites((prev) => {
      const next = new Set(prev);
      if (next.has(appId)) {
        next.delete(appId);
      } else {
        next.add(appId);
      }
      return next;
    });
  };

  const trackRecentApp = (appId: string) => {
    setRecentApps((prev) => {
      const filtered = prev.filter((id) => id !== appId);
      return [appId, ...filtered].slice(0, 5);
    });
  };

  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      // Cmd/Ctrl+K toggles command palette
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault();
        setIsCommandPaletteOpen((prev) => !prev);
        return;
      }

      if (isCommandPaletteOpen) return;
      if (visibleApps.length === 0) return;

      const columnsMap: Record<string, number> = {
        xl: 4,
        lg: 3,
        sm: 2,
        default: 1,
      };

      const getColumns = () => {
        if (window.innerWidth >= 1280) return columnsMap.xl;
        if (window.innerWidth >= 1024) return columnsMap.lg;
        if (window.innerWidth >= 640) return columnsMap.sm;
        return columnsMap.default;
      };

      const columns = getColumns();

      switch (e.key) {
        case 'ArrowRight':
          e.preventDefault();
          setShowKeyboardHint(true);
          setFocusedIndex((prev) =>
            prev < visibleApps.length - 1 ? prev + 1 : prev
          );
          break;
        case 'ArrowLeft':
          e.preventDefault();
          setShowKeyboardHint(true);
          setFocusedIndex((prev) => (prev > 0 ? prev - 1 : prev));
          break;
        case 'ArrowDown':
          e.preventDefault();
          setShowKeyboardHint(true);
          setFocusedIndex((prev) =>
            prev + columns < visibleApps.length ? prev + columns : prev
          );
          break;
        case 'ArrowUp':
          e.preventDefault();
          setShowKeyboardHint(true);
          setFocusedIndex((prev) => (prev - columns >= 0 ? prev - columns : prev));
          break;
        case 'Enter':
          if (focusedIndex >= 0 && focusedIndex < visibleApps.length) {
            e.preventDefault();
            const app = visibleApps[focusedIndex];
            trackRecentApp(app.id);
            if (app.launch_type === 'container' || app.launch_type === 'web_proxy') {
              // Both container and web_proxy apps use VNC streaming (browser sidecar for web_proxy)
              setSelectedContainerApp(app);
            } else {
              window.open(app.url, '_blank', 'noopener,noreferrer');
            }
          }
          break;
        case 'Escape':
          setFocusedIndex(-1);
          break;
      }
    },
    [visibleApps, focusedIndex, isCommandPaletteOpen]
  );

  useEffect(() => {
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [handleKeyDown]);

  useEffect(() => {
    if (focusedIndex >= 0 && appRefs.current[focusedIndex]) {
      appRefs.current[focusedIndex]?.focus();
    }
  }, [focusedIndex]);

  // Reset focus when category filter changes
  useEffect(() => {
    setFocusedIndex(-1);
  }, [selectedCategory]);

  const handleLogin = async (loggedInUser: User) => {
    setStoredUser(loggedInUser);
    setUser(loggedInUser);
    setShowRegister(false);
    setLoading(true); // Trigger app reload
    // Fetch enriched user from /api/auth/me (includes admin_categories)
    const enriched = await getCurrentUser();
    if (enriched) setUser(enriched);
  };

  const handleRegister = async (registeredUser: User) => {
    setStoredUser(registeredUser);
    setUser(registeredUser);
    setShowRegister(false);
    setLoading(true); // Trigger app reload
    const enriched = await getCurrentUser();
    if (enriched) setUser(enriched);
  };

  const isAdmin = user?.roles?.includes('admin') ?? false;
  const isCategoryAdmin = (user?.admin_categories?.length ?? 0) > 0;
  const canAccessAdmin = isAdmin || isCategoryAdmin;

  const handleLogout = async () => {
    await authLogout();
    setUser(null);
    setApps([]);
  };

  const handleAddApp = async (app: Application) => {
    const response = await fetchWithAuth('/api/apps', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(app),
    });
    if (!response.ok) {
      const errorData = await response.json().catch(() => ({}));
      throw new Error(errorData.error || 'Failed to add application');
    }
    const addedApp = await response.json();
    setApps((prev) => [...prev, addedApp]);
  };

  // Show loading spinner while validating auth
  if (authLoading) {
    return (
      <div className="min-h-screen bg-gray-50 dark:bg-gray-900 flex items-center justify-center">
        <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-brand-accent"></div>
      </div>
    );
  }

  // Show login or register screen if not authenticated
  if (!user) {
    if (showRegister) {
      return (
        <Register
          onRegister={handleRegister}
          onBackToLogin={() => setShowRegister(false)}
          darkMode={darkMode}
        />
      );
    }
    return (
      <Login
        onLogin={handleLogin}
        onShowRegister={() => setShowRegister(true)}
        allowRegistration={allowRegistration}
        ssoEnabled={ssoEnabled}
        darkMode={darkMode}
      />
    );
  }

  // Show full-page session view for container/web_proxy apps
  if (selectedContainerApp) {
    return (
      <SessionPage
        app={selectedContainerApp}
        onClose={() => {
          setSelectedContainerApp(null);
          setReconnectSessionId(null);
          setSessionShareInfo(null);
        }}
        darkMode={darkMode}
        sessionId={reconnectSessionId || undefined}
        viewOnly={sessionShareInfo?.viewOnly}
        ownerUsername={sessionShareInfo?.ownerUsername}
        sharePermission={sessionShareInfo?.sharePermission}
      />
    );
  }

  // Build a flat list for keyboard navigation while maintaining category order
  let appIndex = 0;

  return (
    <div className="min-h-screen bg-gradient-to-b from-gray-100 via-gray-50 to-gray-100 dark:from-gray-900 dark:via-gray-900 dark:to-gray-950 transition-colors flex flex-col">
      {/* Header */}
      <header className="bg-gradient-to-r from-brand-primary via-brand-secondary to-brand-primary-light text-white shadow-lg sticky top-0 z-30 border-b border-white/10">
        <div className="max-w-7xl mx-auto px-4 py-3 sm:px-6 lg:px-8">
          <div className="flex items-center justify-between">
            {/* Logo */}
            <div className="flex items-center gap-3">
              <img src={sortieIconWhite} alt="Sortie" className="w-9 h-9" />
              <h1 className="text-xl font-bold">Sortie</h1>
            </div>

            {/* Right side controls */}
            <div className="flex items-center gap-3">
              {/* Search hint — opens command palette */}
              <button
                onClick={() => setIsCommandPaletteOpen(true)}
                className="hidden sm:flex items-center gap-2 px-3 py-1.5 rounded-lg bg-white/10 border border-white/15 hover:bg-white/15 hover:border-white/25 transition-colors text-sm text-white/70 hover:text-white/90"
              >
                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
                </svg>
                <span>Search...</span>
                <kbd className="ml-1 px-1.5 py-0.5 text-[10px] font-medium bg-white/10 rounded border border-white/15">
                  {navigator.platform?.includes('Mac') ? '\u2318K' : 'Ctrl+K'}
                </kbd>
              </button>

              {/* Sessions button */}
              <button
                onClick={() => setShowSessionManager(true)}
                className="relative px-3 py-1.5 rounded-lg bg-white/10 border border-white/15 hover:bg-white/15 hover:border-white/25 transition-colors text-sm font-medium flex items-center gap-2"
                aria-label="Manage sessions"
              >
                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
                </svg>
                <span className="hidden sm:inline">Sessions</span>
                {activeSessionCount > 0 && (
                  <span className="absolute -top-1.5 -right-1.5 min-w-[18px] h-[18px] flex items-center justify-center rounded-full bg-green-500 text-white text-[10px] font-bold px-1 shadow-sm">
                    {activeSessionCount}
                  </span>
                )}
              </button>

              {/* Recordings button */}
              <button
                onClick={() => setShowRecordings(true)}
                className="px-3 py-1.5 rounded-lg bg-white/10 border border-white/15 hover:bg-white/15 hover:border-white/25 transition-colors text-sm font-medium flex items-center gap-2"
                aria-label="View recordings"
              >
                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
                </svg>
                <span className="hidden sm:inline">Recordings</span>
              </button>

              {/* Avatar / User Menu */}
              <UserMenu
                user={user}
                isAdmin={isAdmin}
                canAccessAdmin={canAccessAdmin}
                darkMode={darkMode}
                onToggleDarkMode={() => setDarkMode(!darkMode)}
                onOpenDocs={() => window.open('/docs/', '_blank', 'noopener,noreferrer')}
                onOpenAdmin={() => setShowAdmin(true)}
                onOpenAuditLog={() => setShowAuditLog(true)}
                onLogout={handleLogout}
              />
            </div>
          </div>
        </div>
      </header>

      {/* Command Palette */}
      <CommandPalette
        isOpen={isCommandPaletteOpen}
        onClose={() => setIsCommandPaletteOpen(false)}
        apps={apps}
        isAdmin={isAdmin}
        canAccessAdmin={canAccessAdmin}
        darkMode={darkMode}
        onLaunchApp={(app) => {
          trackRecentApp(app.id);
          if (app.launch_type === 'container' || app.launch_type === 'web_proxy') {
            setSelectedContainerApp(app);
          } else {
            window.open(app.url, '_blank', 'noopener,noreferrer');
          }
        }}
        onOpenTemplates={() => setIsTemplateBrowserOpen(true)}
        onToggleDarkMode={() => setDarkMode(!darkMode)}
        onOpenAdmin={() => setShowAdmin(true)}
        onOpenAuditLog={() => setShowAuditLog(true)}
      />

      {/* Main Content */}
      <main className="max-w-7xl mx-auto px-4 py-8 sm:px-6 lg:px-8 flex-grow w-full animate-fade-in-up">
        {/* Keyboard hint - shown after first arrow key press */}
        {showKeyboardHint && (
          <p className="text-xs text-gray-500 dark:text-gray-400 mb-4 animate-in fade-in">
            Use arrow keys to navigate, Enter to launch, Escape to clear focus
          </p>
        )}

        {/* Category filter tabs */}
        {allCategories.length > 1 && (
          <div className="mb-6">
            <div className="flex flex-wrap gap-2">
              <button
                onClick={() => setSelectedCategory(null)}
                className={`px-4 py-2 rounded-full text-sm font-medium transition-all ${
                  selectedCategory === null
                    ? 'bg-brand-accent text-white shadow-lg'
                    : 'bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300 hover:bg-gray-200 dark:hover:bg-gray-600 border border-gray-200 dark:border-gray-600'
                }`}
              >
                All
                <span className="ml-1.5 text-xs opacity-75">({apps.length})</span>
              </button>
              {allCategories.map((category) => {
                const count = apps.filter((app) => app.category === category).length;
                return (
                  <button
                    key={category}
                    onClick={() => setSelectedCategory(category)}
                    className={`px-4 py-2 rounded-full text-sm font-medium transition-all ${
                      selectedCategory === category
                        ? 'bg-brand-accent text-white shadow-lg'
                        : 'bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300 hover:bg-gray-200 dark:hover:bg-gray-600 border border-gray-200 dark:border-gray-600'
                    }`}
                  >
                    {category}
                    <span className="ml-1.5 text-xs opacity-75">({count})</span>
                  </button>
                );
              })}
            </div>
          </div>
        )}

        {loading ? (
          <div className="flex justify-center items-center h-64">
            <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-brand-accent"></div>
          </div>
        ) : filteredApps.length === 0 ? (
          <div className="text-center py-12">
            <svg
              className="mx-auto h-12 w-12 text-gray-400"
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
            <h3 className="mt-2 text-sm font-medium text-gray-900 dark:text-gray-100">No applications found</h3>
            <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
              Try adjusting your search terms.
            </p>
          </div>
        ) : (
          <div className="space-y-4">
            {/* Favorites Section - hidden when redundant with visible categories */}
            {favoriteApps.length > 0 && apps.length > 3 && (() => {
              const favCategories = new Set(favoriteApps.map(a => a.category));
              return favCategories.size > 1 || [...favCategories].some(c => collapsedCategories.has(c));
            })() && (
              <div className="bg-white dark:bg-gray-800/80 rounded-xl shadow-md border border-yellow-200/80 dark:border-yellow-700/50 overflow-hidden">
                <div className="flex items-center gap-2 px-4 py-3 bg-gradient-to-r from-yellow-50 to-yellow-50/50 dark:from-yellow-900/20 dark:to-yellow-900/10">
                  <svg className="w-5 h-5 text-yellow-500" fill="currentColor" viewBox="0 0 24 24">
                    <path d="M11.049 2.927c.3-.921 1.603-.921 1.902 0l1.519 4.674a1 1 0 00.95.69h4.915c.969 0 1.371 1.24.588 1.81l-3.976 2.888a1 1 0 00-.363 1.118l1.518 4.674c.3.922-.755 1.688-1.538 1.118l-3.976-2.888a1 1 0 00-1.176 0l-3.976 2.888c-.783.57-1.838-.197-1.538-1.118l1.518-4.674a1 1 0 00-.363-1.118l-3.976-2.888c-.784-.57-.38-1.81.588-1.81h4.914a1 1 0 00.951-.69l1.519-4.674z" />
                  </svg>
                  <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">Favorites</h2>
                  <span className="text-sm text-gray-500 dark:text-gray-400">({favoriteApps.length})</span>
                </div>
                <div className="p-4">
                  <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
                    {favoriteApps.map((app) => {
                      const isContainerApp = app.launch_type === 'container' || app.launch_type === 'web_proxy';
                      const cardClassName = "group bg-gradient-to-br from-gray-50 to-white dark:from-gray-700 dark:to-gray-700/80 rounded-xl border border-gray-200 dark:border-gray-600 hover:border-brand-accent p-4 hover:shadow-lg hover:-translate-y-0.5 transition-all duration-200 text-left w-full";
                      const cardContent = (
                        <div className="flex items-start gap-3">
                          <div className="flex-shrink-0 w-12 h-12 bg-white dark:bg-gray-600 rounded-xl flex items-center justify-center overflow-hidden relative">
                            <img src={app.icon} alt={`${app.name} icon`} className="w-8 h-8 object-contain" onError={(e) => { (e.target as HTMLImageElement).src = 'data:image/svg+xml,<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="%23636A51"><rect width="24" height="24" rx="4"/><text x="12" y="16" text-anchor="middle" fill="white" font-size="12">' + app.name.charAt(0) + '</text></svg>'; }} />
                          </div>
                          <div className="flex-1 min-w-0">
                            <h3 className="text-sm font-medium text-gray-900 dark:text-gray-100 group-hover:text-brand-accent line-clamp-2">{app.name}</h3>
                            <p className="mt-1 text-xs text-gray-500 dark:text-gray-400 line-clamp-2">{app.description}</p>
                            {isContainerApp && (
                              <span className={`mt-1.5 inline-flex items-center gap-1 text-[10px] font-medium px-1.5 py-0.5 rounded-full ${app.launch_type === 'web_proxy' ? 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300' : 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'}`}>
                                <svg className="w-2.5 h-2.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" /></svg>
                                {app.launch_type === 'web_proxy' ? 'Web App' : 'Container'}
                              </span>
                            )}
                          </div>
                          <div className="flex-shrink-0">
                            <button onClick={(e) => toggleFavorite(app.id, e)} className="p-1 rounded hover:bg-gray-200 dark:hover:bg-gray-600 transition-colors" aria-label="Remove from favorites" title="Remove from favorites">
                              <svg className="w-4 h-4 text-yellow-500 fill-yellow-500" fill="currentColor" stroke="currentColor" viewBox="0 0 24 24">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M11.049 2.927c.3-.921 1.603-.921 1.902 0l1.519 4.674a1 1 0 00.95.69h4.915c.969 0 1.371 1.24.588 1.81l-3.976 2.888a1 1 0 00-.363 1.118l1.518 4.674c.3.922-.755 1.688-1.538 1.118l-3.976-2.888a1 1 0 00-1.176 0l-3.976 2.888c-.783.57-1.838-.197-1.538-1.118l1.518-4.674a1 1 0 00-.363-1.118l-3.976-2.888c-.784-.57-.38-1.81.588-1.81h4.914a1 1 0 00.951-.69l1.519-4.674z" />
                              </svg>
                            </button>
                          </div>
                        </div>
                      );
                      if (app.launch_type === 'container' || app.launch_type === 'web_proxy') {
                        // Both container and web_proxy apps use VNC streaming
                        return <button key={app.id} onClick={() => { trackRecentApp(app.id); setSelectedContainerApp(app); }} className={cardClassName}>{cardContent}</button>;
                      }
                      return <a key={app.id} href={app.url} target="_blank" rel="noopener noreferrer" onClick={() => trackRecentApp(app.id)} className={cardClassName}>{cardContent}</a>;
                    })}
                  </div>
                </div>
              </div>
            )}

            {/* Recent Apps Section - hidden when redundant with visible categories */}
            {recentAppsList.length > 0 && apps.length > 3 && (() => {
              const recentCategories = new Set(recentAppsList.map(a => a.category));
              return recentCategories.size > 1 || [...recentCategories].some(c => collapsedCategories.has(c));
            })() && (
              <div className="bg-white dark:bg-gray-800/80 rounded-xl shadow-md border border-blue-200/80 dark:border-blue-700/50 overflow-hidden">
                <div className="flex items-center gap-2 px-4 py-3 bg-gradient-to-r from-blue-50 to-blue-50/50 dark:from-blue-900/20 dark:to-blue-900/10">
                  <svg className="w-5 h-5 text-blue-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
                  </svg>
                  <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">Recent</h2>
                  <span className="text-sm text-gray-500 dark:text-gray-400">({recentAppsList.length})</span>
                </div>
                <div className="p-4">
                  <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
                    {recentAppsList.map((app) => {
                      const isContainerApp = app.launch_type === 'container' || app.launch_type === 'web_proxy';
                      const isFavorited = favorites.has(app.id);
                      const cardClassName = "group bg-gradient-to-br from-gray-50 to-white dark:from-gray-700 dark:to-gray-700/80 rounded-xl border border-gray-200 dark:border-gray-600 hover:border-brand-accent p-4 hover:shadow-lg hover:-translate-y-0.5 transition-all duration-200 text-left w-full";
                      const cardContent = (
                        <div className="flex items-start gap-3">
                          <div className="flex-shrink-0 w-12 h-12 bg-white dark:bg-gray-600 rounded-xl flex items-center justify-center overflow-hidden relative">
                            <img src={app.icon} alt={`${app.name} icon`} className="w-8 h-8 object-contain" onError={(e) => { (e.target as HTMLImageElement).src = 'data:image/svg+xml,<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="%23636A51"><rect width="24" height="24" rx="4"/><text x="12" y="16" text-anchor="middle" fill="white" font-size="12">' + app.name.charAt(0) + '</text></svg>'; }} />
                          </div>
                          <div className="flex-1 min-w-0">
                            <h3 className="text-sm font-medium text-gray-900 dark:text-gray-100 group-hover:text-brand-accent line-clamp-2">{app.name}</h3>
                            <p className="mt-1 text-xs text-gray-500 dark:text-gray-400 line-clamp-2">{app.description}</p>
                            {isContainerApp && (
                              <span className={`mt-1.5 inline-flex items-center gap-1 text-[10px] font-medium px-1.5 py-0.5 rounded-full ${app.launch_type === 'web_proxy' ? 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300' : 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'}`}>
                                <svg className="w-2.5 h-2.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" /></svg>
                                {app.launch_type === 'web_proxy' ? 'Web App' : 'Container'}
                              </span>
                            )}
                          </div>
                          <div className="flex-shrink-0">
                            <button onClick={(e) => toggleFavorite(app.id, e)} className="p-1 rounded hover:bg-gray-200 dark:hover:bg-gray-600 transition-colors" aria-label={isFavorited ? 'Remove from favorites' : 'Add to favorites'} title={isFavorited ? 'Remove from favorites' : 'Add to favorites'}>
                              <svg className={`w-4 h-4 ${isFavorited ? 'text-yellow-500 fill-yellow-500' : 'text-gray-400 group-hover:text-gray-500'}`} fill={isFavorited ? 'currentColor' : 'none'} stroke="currentColor" viewBox="0 0 24 24">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M11.049 2.927c.3-.921 1.603-.921 1.902 0l1.519 4.674a1 1 0 00.95.69h4.915c.969 0 1.371 1.24.588 1.81l-3.976 2.888a1 1 0 00-.363 1.118l1.518 4.674c.3.922-.755 1.688-1.538 1.118l-3.976-2.888a1 1 0 00-1.176 0l-3.976 2.888c-.783.57-1.838-.197-1.538-1.118l1.518-4.674a1 1 0 00-.363-1.118l-3.976-2.888c-.784-.57-.38-1.81.588-1.81h4.914a1 1 0 00.951-.69l1.519-4.674z" />
                              </svg>
                            </button>
                          </div>
                        </div>
                      );
                      if (app.launch_type === 'container' || app.launch_type === 'web_proxy') {
                        // Both container and web_proxy apps use VNC streaming
                        return <button key={app.id} onClick={() => { trackRecentApp(app.id); setSelectedContainerApp(app); }} className={cardClassName}>{cardContent}</button>;
                      }
                      return <a key={app.id} href={app.url} target="_blank" rel="noopener noreferrer" onClick={() => trackRecentApp(app.id)} className={cardClassName}>{cardContent}</a>;
                    })}
                  </div>
                </div>
              </div>
            )}

            {categories.map((category) => {
              const categoryApps = filteredApps.filter((app) => app.category === category);
              const isCollapsed = collapsedCategories.has(category);

              return (
                <div key={category} className="bg-white dark:bg-gray-800/80 rounded-xl shadow-md border border-gray-200/80 dark:border-gray-700/50 overflow-hidden">
                  {/* Category header - clickable to collapse */}
                  <button
                    onClick={() => toggleCategory(category)}
                    className="w-full flex items-center justify-between px-4 py-3 bg-gradient-to-r from-gray-50 to-gray-100/50 dark:from-gray-700 dark:to-gray-700/80 hover:from-gray-100 hover:to-gray-100 dark:hover:from-gray-600 dark:hover:to-gray-600/80 transition-all"
                  >
                    <div className="flex items-center gap-2">
                      <svg
                        className={`w-4 h-4 text-gray-500 dark:text-gray-400 transition-transform ${isCollapsed ? '' : 'rotate-90'}`}
                        fill="none"
                        stroke="currentColor"
                        viewBox="0 0 24 24"
                      >
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
                      </svg>
                      <h2 className="text-sm font-semibold text-gray-900 dark:text-gray-100 uppercase tracking-wide">{category}</h2>
                      <span className="text-sm text-gray-500 dark:text-gray-400">({categoryApps.length})</span>
                    </div>
                  </button>

                  {/* Category apps */}
                  {!isCollapsed && (
                    <div className="px-4 py-3">
                      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-3">
                        {categoryApps.map((app) => {
                          const currentIndex = appIndex++;
                          const isContainerApp = app.launch_type === 'container' || app.launch_type === 'web_proxy';
                          const cardClassName = `group bg-gradient-to-br from-gray-50 to-white dark:from-gray-700 dark:to-gray-700/80 rounded-xl border p-4 hover:shadow-lg hover:-translate-y-0.5 transition-all duration-200 text-left w-full ${
                            focusedIndex === currentIndex
                              ? 'ring-2 ring-brand-accent border-brand-accent'
                              : 'border-gray-200 dark:border-gray-600 hover:border-brand-accent'
                          }`;

                          const isFavorited = favorites.has(app.id);
                          const cardContent = (
                            <div className="flex items-start gap-3">
                              <div className="flex-shrink-0 w-12 h-12 bg-white dark:bg-gray-600 rounded-xl flex items-center justify-center overflow-hidden relative shadow-sm ring-1 ring-gray-200/50 dark:ring-gray-500/30">
                                <img
                                  src={app.icon}
                                  alt={`${app.name} icon`}
                                  className="w-8 h-8 object-contain"
                                  onError={(e) => {
                                    (e.target as HTMLImageElement).src =
                                      'data:image/svg+xml,<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="%23636A51"><rect width="24" height="24" rx="4"/><text x="12" y="16" text-anchor="middle" fill="white" font-size="12">' +
                                      app.name.charAt(0) +
                                      '</text></svg>';
                                  }}
                                />
                              </div>
                              <div className="flex-1 min-w-0">
                                <h3 className="text-sm font-medium text-gray-900 dark:text-gray-100 group-hover:text-brand-accent line-clamp-2">
                                  {app.name}
                                </h3>
                                <p className="mt-1 text-xs text-gray-500 dark:text-gray-400 line-clamp-2">
                                  {app.description}
                                </p>
                                {isContainerApp && (
                                  <span className={`mt-1.5 inline-flex items-center gap-1 text-[10px] font-medium px-1.5 py-0.5 rounded-full ${app.launch_type === 'web_proxy' ? 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300' : 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'}`}>
                                    <svg className="w-2.5 h-2.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" /></svg>
                                    {app.launch_type === 'web_proxy' ? 'Web App' : 'Container'}
                                  </span>
                                )}
                              </div>
                              <div className="flex-shrink-0">
                                <button
                                  onClick={(e) => toggleFavorite(app.id, e)}
                                  className="p-1 rounded hover:bg-gray-200 dark:hover:bg-gray-600 transition-colors"
                                  aria-label={isFavorited ? 'Remove from favorites' : 'Add to favorites'}
                                  title={isFavorited ? 'Remove from favorites' : 'Add to favorites'}
                                >
                                  <svg
                                    className={`w-4 h-4 ${isFavorited ? 'text-yellow-500 fill-yellow-500' : 'text-gray-400 group-hover:text-gray-500'}`}
                                    fill={isFavorited ? 'currentColor' : 'none'}
                                    stroke="currentColor"
                                    viewBox="0 0 24 24"
                                  >
                                    <path
                                      strokeLinecap="round"
                                      strokeLinejoin="round"
                                      strokeWidth={2}
                                      d="M11.049 2.927c.3-.921 1.603-.921 1.902 0l1.519 4.674a1 1 0 00.95.69h4.915c.969 0 1.371 1.24.588 1.81l-3.976 2.888a1 1 0 00-.363 1.118l1.518 4.674c.3.922-.755 1.688-1.538 1.118l-3.976-2.888a1 1 0 00-1.176 0l-3.976 2.888c-.783.57-1.838-.197-1.538-1.118l1.518-4.674a1 1 0 00-.363-1.118l-3.976-2.888c-.784-.57-.38-1.81.588-1.81h4.914a1 1 0 00.951-.69l1.519-4.674z"
                                    />
                                  </svg>
                                </button>
                              </div>
                            </div>
                          );

                          if (app.launch_type === 'container' || app.launch_type === 'web_proxy') {
                            // Both container and web_proxy apps use VNC streaming (browser sidecar for web_proxy)
                            return (
                              <button
                                key={app.id}
                                ref={(el) => { appRefs.current[currentIndex] = el; }}
                                tabIndex={focusedIndex === currentIndex ? 0 : -1}
                                onClick={() => {
                                  setFocusedIndex(currentIndex);
                                  trackRecentApp(app.id);
                                  setSelectedContainerApp(app);
                                }}
                                onFocus={() => setFocusedIndex(currentIndex)}
                                className={cardClassName}
                              >
                                {cardContent}
                              </button>
                            );
                          }

                          return (
                            <a
                              key={app.id}
                              ref={(el) => { appRefs.current[currentIndex] = el; }}
                              href={app.url}
                              target="_blank"
                              rel="noopener noreferrer"
                              tabIndex={focusedIndex === currentIndex ? 0 : -1}
                              onClick={() => {
                                setFocusedIndex(currentIndex);
                                trackRecentApp(app.id);
                              }}
                              onFocus={() => setFocusedIndex(currentIndex)}
                              className={cardClassName}
                            >
                              {cardContent}
                            </a>
                          );
                        })}
                      </div>
                    </div>
                  )}
                </div>
              );
            })}

            {/* Getting started prompt for sparse dashboards */}
            {apps.length <= 1 && (
              <div className="bg-white dark:bg-gray-800 rounded-xl border-2 border-dashed border-gray-300 dark:border-gray-600 p-8 text-center">
                <svg className="mx-auto h-12 w-12 text-gray-400 dark:text-gray-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M12 4v16m8-8H4" />
                </svg>
                <h3 className="mt-3 text-sm font-semibold text-gray-900 dark:text-gray-100">Get started</h3>
                <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
                  Browse the Template Marketplace to add pre-configured applications
                </p>
                <button
                  onClick={() => setIsTemplateBrowserOpen(true)}
                  className="mt-4 inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-brand-accent text-white text-sm font-medium hover:bg-brand-accent/90 transition-colors shadow-md hover:shadow-lg"
                >
                  <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 5a1 1 0 011-1h14a1 1 0 011 1v2a1 1 0 01-1 1H5a1 1 0 01-1-1V5zM4 13a1 1 0 011-1h6a1 1 0 011 1v6a1 1 0 01-1 1H5a1 1 0 01-1-1v-6zM16 13a1 1 0 011-1h2a1 1 0 011 1v6a1 1 0 01-1 1h-2a1 1 0 01-1-1v-6z" />
                  </svg>
                  Browse Templates
                </button>
              </div>
            )}
          </div>
        )}
      </main>

      {/* Footer */}
      <footer className="bg-gradient-to-r from-gray-50 via-white to-gray-50 dark:from-gray-800 dark:via-gray-800 dark:to-gray-800 border-t border-gray-200 dark:border-gray-700 mt-auto">
        <div className="max-w-7xl mx-auto px-4 py-4 sm:px-6 lg:px-8">
          <p className="text-center text-sm text-gray-500 dark:text-gray-400">
            Sortie — Your centralized application launcher
          </p>
        </div>
      </footer>

      {/* Template Browser Modal */}
      <TemplateBrowser
        isOpen={isTemplateBrowserOpen}
        onClose={() => setIsTemplateBrowserOpen(false)}
        onAddApp={handleAddApp}
        darkMode={darkMode}
      />

      {/* Admin Panel */}
      {showAdmin && canAccessAdmin && (
        <Admin
          darkMode={darkMode}
          onClose={() => setShowAdmin(false)}
          isSystemAdmin={isAdmin}
          adminCategoryIds={user?.admin_categories ?? []}
        />
      )}

      {/* Audit Log */}
      {showAuditLog && isAdmin && (
        <AuditLog
          darkMode={darkMode}
          onClose={() => setShowAuditLog(false)}
        />
      )}

      {/* Recordings List */}
      <RecordingsList
        isOpen={showRecordings}
        onClose={() => setShowRecordings(false)}
        darkMode={darkMode}
      />

      {/* Session Manager */}
      <SessionManager
        isOpen={showSessionManager}
        onClose={() => setShowSessionManager(false)}
        onReconnect={(appId, sessionId) => {
          const app = apps.find((a) => a.id === appId);
          if (app) {
            setReconnectSessionId(sessionId);
            // Check if this is a shared session
            const sess = sessions.find((s) => s.id === sessionId);
            if (sess?.is_shared) {
              setSessionShareInfo({
                viewOnly: sess.share_permission === 'read_only',
                ownerUsername: sess.owner_username,
                sharePermission: sess.share_permission,
              });
            } else {
              setSessionShareInfo(null);
            }
            setSelectedContainerApp(app);
            setShowSessionManager(false);
          }
        }}
        darkMode={darkMode}
      />
    </div>
  );
}

export default App;
