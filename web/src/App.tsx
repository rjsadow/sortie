import { useState, useEffect, useRef, useCallback } from 'react';
import type { Application, User } from './types';
import { SessionModal } from './components/SessionModal';
import { Login } from './components/Login';
import { Register } from './components/Register';
import { Admin } from './components/Admin';
import { TemplateBrowser } from './components/templates/TemplateBrowser';
import {
  getStoredUser,
  setStoredUser,
  logout as authLogout,
  getCurrentUser,
  isAuthenticated,
  fetchWithAuth
} from './services/auth';

function App() {
  const [user, setUser] = useState<User | null>(() => getStoredUser());
  const [authLoading, setAuthLoading] = useState(true);
  const [apps, setApps] = useState<Application[]>([]);
  const [search, setSearch] = useState('');
  const [loading, setLoading] = useState(true);
  const [darkMode, setDarkMode] = useState(() => {
    const stored = localStorage.getItem('launchpad-theme');
    if (stored) return stored === 'dark';
    return window.matchMedia('(prefers-color-scheme: dark)').matches;
  });
  const [collapsedCategories, setCollapsedCategories] = useState<Set<string>>(() => {
    const stored = localStorage.getItem('launchpad-collapsed');
    return stored ? new Set(JSON.parse(stored)) : new Set();
  });
  const [selectedCategory, setSelectedCategory] = useState<string | null>(() => {
    const stored = localStorage.getItem('launchpad-category-filter');
    return stored && stored !== 'null' ? stored : null;
  });
  const [favorites, setFavorites] = useState<Set<string>>(() => {
    const stored = localStorage.getItem('launchpad-favorites');
    return stored ? new Set(JSON.parse(stored)) : new Set();
  });
  const [recentApps, setRecentApps] = useState<string[]>(() => {
    const stored = localStorage.getItem('launchpad-recents');
    return stored ? JSON.parse(stored) : [];
  });
  const [focusedIndex, setFocusedIndex] = useState(-1);
  const [selectedContainerApp, setSelectedContainerApp] = useState<Application | null>(null);
  const [isTemplateBrowserOpen, setIsTemplateBrowserOpen] = useState(false);
  const [showRegister, setShowRegister] = useState(false);
  const [showAdmin, setShowAdmin] = useState(false);
  const [allowRegistration, setAllowRegistration] = useState(false);
  const appRefs = useRef<(HTMLButtonElement | HTMLAnchorElement | null)[]>([]);

  // Validate token on app load and fetch config
  useEffect(() => {
    const validateAuth = async () => {
      // Fetch config for registration setting
      try {
        const configRes = await fetch('/api/config');
        if (configRes.ok) {
          const config = await configRes.json();
          setAllowRegistration(config.allow_registration === true);
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
    localStorage.setItem('launchpad-theme', darkMode ? 'dark' : 'light');
  }, [darkMode]);

  useEffect(() => {
    localStorage.setItem('launchpad-collapsed', JSON.stringify([...collapsedCategories]));
  }, [collapsedCategories]);

  useEffect(() => {
    localStorage.setItem('launchpad-category-filter', selectedCategory || 'null');
  }, [selectedCategory]);

  useEffect(() => {
    localStorage.setItem('launchpad-favorites', JSON.stringify([...favorites]));
  }, [favorites]);

  useEffect(() => {
    localStorage.setItem('launchpad-recents', JSON.stringify(recentApps));
  }, [recentApps]);

  // Get all unique categories from all apps (before search filtering)
  const allCategories = [...new Set(apps.map((app) => app.category))].sort();

  const filteredApps = apps.filter((app) => {
    // First apply search filter
    const matchesSearch =
      app.name.toLowerCase().includes(search.toLowerCase()) ||
      app.description.toLowerCase().includes(search.toLowerCase()) ||
      app.category.toLowerCase().includes(search.toLowerCase());

    // Then apply category filter
    const matchesCategory = selectedCategory === null || app.category === selectedCategory;

    return matchesSearch && matchesCategory;
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
          setFocusedIndex((prev) =>
            prev < visibleApps.length - 1 ? prev + 1 : prev
          );
          break;
        case 'ArrowLeft':
          e.preventDefault();
          setFocusedIndex((prev) => (prev > 0 ? prev - 1 : prev));
          break;
        case 'ArrowDown':
          e.preventDefault();
          setFocusedIndex((prev) =>
            prev + columns < visibleApps.length ? prev + columns : prev
          );
          break;
        case 'ArrowUp':
          e.preventDefault();
          setFocusedIndex((prev) => (prev - columns >= 0 ? prev - columns : prev));
          break;
        case 'Enter':
          if (focusedIndex >= 0 && focusedIndex < visibleApps.length) {
            e.preventDefault();
            const app = visibleApps[focusedIndex];
            trackRecentApp(app.id);
            if (app.launch_type === 'container') {
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
    [visibleApps, focusedIndex]
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

  // Reset focus when search changes
  useEffect(() => {
    setFocusedIndex(-1);
  }, [search]);

  const handleLogin = (loggedInUser: User) => {
    setStoredUser(loggedInUser);
    setUser(loggedInUser);
    setShowRegister(false);
    setLoading(true); // Trigger app reload
  };

  const handleRegister = (registeredUser: User) => {
    setStoredUser(registeredUser);
    setUser(registeredUser);
    setShowRegister(false);
    setLoading(true); // Trigger app reload
  };

  const isAdmin = user?.roles?.includes('admin') ?? false;

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
        <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-brand-primary"></div>
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
        darkMode={darkMode}
      />
    );
  }

  // Build a flat list for keyboard navigation while maintaining category order
  let appIndex = 0;

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900 transition-colors">
      {/* Header */}
      <header className="bg-brand-primary text-white shadow-lg">
        <div className="max-w-7xl mx-auto px-4 py-6 sm:px-6 lg:px-8">
          <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 bg-white rounded-lg flex items-center justify-center">
                <svg
                  className="w-6 h-6 text-brand-primary"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M4 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2V6zM14 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2V6zM4 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2v-2zM14 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2v-2z"
                  />
                </svg>
              </div>
              <h1 className="text-2xl font-bold">Launchpad</h1>
            </div>
            <div className="flex items-center gap-3">
              <div className="relative">
                <input
                  type="text"
                  placeholder="Search applications..."
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                  className="w-full sm:w-80 px-4 py-2 pl-10 rounded-lg text-gray-900 placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-brand-secondary"
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
              {/* Templates button */}
              <button
                onClick={() => setIsTemplateBrowserOpen(true)}
                className="px-3 py-2 rounded-lg bg-white/10 hover:bg-white/20 transition-colors text-sm font-medium flex items-center gap-2"
                aria-label="Browse templates"
              >
                <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M4 5a1 1 0 011-1h14a1 1 0 011 1v2a1 1 0 01-1 1H5a1 1 0 01-1-1V5zM4 13a1 1 0 011-1h6a1 1 0 011 1v6a1 1 0 01-1 1H5a1 1 0 01-1-1v-6zM16 13a1 1 0 011-1h2a1 1 0 011 1v6a1 1 0 01-1 1h-2a1 1 0 01-1-1v-6z"
                  />
                </svg>
                <span className="hidden sm:inline">Templates</span>
              </button>
              {/* Admin button (only for admins) */}
              {isAdmin && (
                <button
                  onClick={() => setShowAdmin(true)}
                  className="px-3 py-2 rounded-lg bg-white/10 hover:bg-white/20 transition-colors text-sm font-medium flex items-center gap-2"
                  aria-label="Admin settings"
                >
                  <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      strokeWidth={2}
                      d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"
                    />
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                  </svg>
                  <span className="hidden sm:inline">Admin</span>
                </button>
              )}
              {/* Dark mode toggle */}
              <button
                onClick={() => setDarkMode(!darkMode)}
                className="p-2 rounded-lg bg-white/10 hover:bg-white/20 transition-colors"
                aria-label={darkMode ? 'Switch to light mode' : 'Switch to dark mode'}
              >
                {darkMode ? (
                  <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      strokeWidth={2}
                      d="M12 3v1m0 16v1m9-9h-1M4 12H3m15.364 6.364l-.707-.707M6.343 6.343l-.707-.707m12.728 0l-.707.707M6.343 17.657l-.707.707M16 12a4 4 0 11-8 0 4 4 0 018 0z"
                    />
                  </svg>
                ) : (
                  <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      strokeWidth={2}
                      d="M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z"
                    />
                  </svg>
                )}
              </button>
              {/* User menu */}
              <div className="flex items-center gap-2 pl-2 border-l border-white/20">
                <span className="text-sm hidden sm:inline">{user.displayName || user.username}</span>
                <button
                  onClick={handleLogout}
                  className="p-2 rounded-lg bg-white/10 hover:bg-white/20 transition-colors"
                  aria-label="Sign out"
                  title="Sign out"
                >
                  <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      strokeWidth={2}
                      d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1"
                    />
                  </svg>
                </button>
              </div>
            </div>
          </div>
        </div>
      </header>

      {/* Main Content */}
      <main className="max-w-7xl mx-auto px-4 py-8 sm:px-6 lg:px-8">
        {/* Keyboard hint */}
        <p className="text-xs text-gray-500 dark:text-gray-400 mb-4">
          Use arrow keys to navigate, Enter to launch, Escape to clear focus
        </p>

        {/* Category filter tabs */}
        {allCategories.length > 1 && (
          <div className="mb-6">
            <div className="flex flex-wrap gap-2">
              <button
                onClick={() => setSelectedCategory(null)}
                className={`px-4 py-2 rounded-full text-sm font-medium transition-all ${
                  selectedCategory === null
                    ? 'bg-brand-primary text-white shadow-md'
                    : 'bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300 hover:bg-gray-200 dark:hover:bg-gray-600'
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
                        ? 'bg-brand-primary text-white shadow-md'
                        : 'bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300 hover:bg-gray-200 dark:hover:bg-gray-600'
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
            <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-brand-primary"></div>
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
          <div className="space-y-6">
            {/* Favorites Section */}
            {favoriteApps.length > 0 && (
              <div className="bg-white dark:bg-gray-800 rounded-lg shadow-sm border border-yellow-200 dark:border-yellow-700 overflow-hidden">
                <div className="flex items-center gap-2 px-4 py-3 bg-yellow-50 dark:bg-yellow-900/20">
                  <svg className="w-5 h-5 text-yellow-500" fill="currentColor" viewBox="0 0 24 24">
                    <path d="M11.049 2.927c.3-.921 1.603-.921 1.902 0l1.519 4.674a1 1 0 00.95.69h4.915c.969 0 1.371 1.24.588 1.81l-3.976 2.888a1 1 0 00-.363 1.118l1.518 4.674c.3.922-.755 1.688-1.538 1.118l-3.976-2.888a1 1 0 00-1.176 0l-3.976 2.888c-.783.57-1.838-.197-1.538-1.118l1.518-4.674a1 1 0 00-.363-1.118l-3.976-2.888c-.784-.57-.38-1.81.588-1.81h4.914a1 1 0 00.951-.69l1.519-4.674z" />
                  </svg>
                  <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">Favorites</h2>
                  <span className="text-sm text-gray-500 dark:text-gray-400">({favoriteApps.length})</span>
                </div>
                <div className="p-4">
                  <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
                    {favoriteApps.map((app) => {
                      const isContainerApp = app.launch_type === 'container';
                      const cardClassName = "group bg-gray-50 dark:bg-gray-700 rounded-lg border border-gray-200 dark:border-gray-600 hover:border-brand-secondary p-4 hover:shadow-md transition-all duration-200 text-left w-full";
                      const cardContent = (
                        <div className="flex items-start gap-3">
                          <div className="flex-shrink-0 w-12 h-12 bg-white dark:bg-gray-600 rounded-lg flex items-center justify-center overflow-hidden relative">
                            <img src={app.icon} alt={`${app.name} icon`} className="w-8 h-8 object-contain" onError={(e) => { (e.target as HTMLImageElement).src = 'data:image/svg+xml,<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="%23398D9B"><rect width="24" height="24" rx="4"/><text x="12" y="16" text-anchor="middle" fill="white" font-size="12">' + app.name.charAt(0) + '</text></svg>'; }} />
                            {isContainerApp && (
                              <div className="absolute -top-1 -right-1 w-4 h-4 bg-blue-500 rounded-full flex items-center justify-center" title="Container App">
                                <svg className="w-2.5 h-2.5 text-white" fill="currentColor" viewBox="0 0 24 24"><path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/></svg>
                              </div>
                            )}
                          </div>
                          <div className="flex-1 min-w-0">
                            <h3 className="text-sm font-medium text-gray-900 dark:text-gray-100 group-hover:text-brand-primary truncate">{app.name}</h3>
                            <p className="mt-1 text-xs text-gray-500 dark:text-gray-400 line-clamp-2">{app.description}</p>
                          </div>
                          <div className="flex flex-col gap-1 flex-shrink-0">
                            <button onClick={(e) => toggleFavorite(app.id, e)} className="p-1 rounded hover:bg-gray-200 dark:hover:bg-gray-600 transition-colors" aria-label="Remove from favorites" title="Remove from favorites">
                              <svg className="w-4 h-4 text-yellow-500 fill-yellow-500" fill="currentColor" stroke="currentColor" viewBox="0 0 24 24">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M11.049 2.927c.3-.921 1.603-.921 1.902 0l1.519 4.674a1 1 0 00.95.69h4.915c.969 0 1.371 1.24.588 1.81l-3.976 2.888a1 1 0 00-.363 1.118l1.518 4.674c.3.922-.755 1.688-1.538 1.118l-3.976-2.888a1 1 0 00-1.176 0l-3.976 2.888c-.783.57-1.838-.197-1.538-1.118l1.518-4.674a1 1 0 00-.363-1.118l-3.976-2.888c-.784-.57-.38-1.81.588-1.81h4.914a1 1 0 00.951-.69l1.519-4.674z" />
                              </svg>
                            </button>
                            <svg className="w-4 h-4 text-gray-400 group-hover:text-brand-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                              {isContainerApp ? (
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
                              ) : (
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
                              )}
                            </svg>
                          </div>
                        </div>
                      );
                      return isContainerApp ? (
                        <button key={app.id} onClick={() => { trackRecentApp(app.id); setSelectedContainerApp(app); }} className={cardClassName}>{cardContent}</button>
                      ) : (
                        <a key={app.id} href={app.url} target="_blank" rel="noopener noreferrer" onClick={() => trackRecentApp(app.id)} className={cardClassName}>{cardContent}</a>
                      );
                    })}
                  </div>
                </div>
              </div>
            )}

            {/* Recent Apps Section */}
            {recentAppsList.length > 0 && (
              <div className="bg-white dark:bg-gray-800 rounded-lg shadow-sm border border-blue-200 dark:border-blue-700 overflow-hidden">
                <div className="flex items-center gap-2 px-4 py-3 bg-blue-50 dark:bg-blue-900/20">
                  <svg className="w-5 h-5 text-blue-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
                  </svg>
                  <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">Recent</h2>
                  <span className="text-sm text-gray-500 dark:text-gray-400">({recentAppsList.length})</span>
                </div>
                <div className="p-4">
                  <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
                    {recentAppsList.map((app) => {
                      const isContainerApp = app.launch_type === 'container';
                      const isFavorited = favorites.has(app.id);
                      const cardClassName = "group bg-gray-50 dark:bg-gray-700 rounded-lg border border-gray-200 dark:border-gray-600 hover:border-brand-secondary p-4 hover:shadow-md transition-all duration-200 text-left w-full";
                      const cardContent = (
                        <div className="flex items-start gap-3">
                          <div className="flex-shrink-0 w-12 h-12 bg-white dark:bg-gray-600 rounded-lg flex items-center justify-center overflow-hidden relative">
                            <img src={app.icon} alt={`${app.name} icon`} className="w-8 h-8 object-contain" onError={(e) => { (e.target as HTMLImageElement).src = 'data:image/svg+xml,<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="%23398D9B"><rect width="24" height="24" rx="4"/><text x="12" y="16" text-anchor="middle" fill="white" font-size="12">' + app.name.charAt(0) + '</text></svg>'; }} />
                            {isContainerApp && (
                              <div className="absolute -top-1 -right-1 w-4 h-4 bg-blue-500 rounded-full flex items-center justify-center" title="Container App">
                                <svg className="w-2.5 h-2.5 text-white" fill="currentColor" viewBox="0 0 24 24"><path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/></svg>
                              </div>
                            )}
                          </div>
                          <div className="flex-1 min-w-0">
                            <h3 className="text-sm font-medium text-gray-900 dark:text-gray-100 group-hover:text-brand-primary truncate">{app.name}</h3>
                            <p className="mt-1 text-xs text-gray-500 dark:text-gray-400 line-clamp-2">{app.description}</p>
                          </div>
                          <div className="flex flex-col gap-1 flex-shrink-0">
                            <button onClick={(e) => toggleFavorite(app.id, e)} className="p-1 rounded hover:bg-gray-200 dark:hover:bg-gray-600 transition-colors" aria-label={isFavorited ? 'Remove from favorites' : 'Add to favorites'} title={isFavorited ? 'Remove from favorites' : 'Add to favorites'}>
                              <svg className={`w-4 h-4 ${isFavorited ? 'text-yellow-500 fill-yellow-500' : 'text-gray-400 group-hover:text-gray-500'}`} fill={isFavorited ? 'currentColor' : 'none'} stroke="currentColor" viewBox="0 0 24 24">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M11.049 2.927c.3-.921 1.603-.921 1.902 0l1.519 4.674a1 1 0 00.95.69h4.915c.969 0 1.371 1.24.588 1.81l-3.976 2.888a1 1 0 00-.363 1.118l1.518 4.674c.3.922-.755 1.688-1.538 1.118l-3.976-2.888a1 1 0 00-1.176 0l-3.976 2.888c-.783.57-1.838-.197-1.538-1.118l1.518-4.674a1 1 0 00-.363-1.118l-3.976-2.888c-.784-.57-.38-1.81.588-1.81h4.914a1 1 0 00.951-.69l1.519-4.674z" />
                              </svg>
                            </button>
                            <svg className="w-4 h-4 text-gray-400 group-hover:text-brand-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                              {isContainerApp ? (
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
                              ) : (
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
                              )}
                            </svg>
                          </div>
                        </div>
                      );
                      return isContainerApp ? (
                        <button key={app.id} onClick={() => { trackRecentApp(app.id); setSelectedContainerApp(app); }} className={cardClassName}>{cardContent}</button>
                      ) : (
                        <a key={app.id} href={app.url} target="_blank" rel="noopener noreferrer" onClick={() => trackRecentApp(app.id)} className={cardClassName}>{cardContent}</a>
                      );
                    })}
                  </div>
                </div>
              </div>
            )}

            {categories.map((category) => {
              const categoryApps = filteredApps.filter((app) => app.category === category);
              const isCollapsed = collapsedCategories.has(category);

              return (
                <div key={category} className="bg-white dark:bg-gray-800 rounded-lg shadow-sm border border-gray-200 dark:border-gray-700 overflow-hidden">
                  {/* Category header - clickable to collapse */}
                  <button
                    onClick={() => toggleCategory(category)}
                    className="w-full flex items-center justify-between px-4 py-3 bg-gray-50 dark:bg-gray-750 hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors"
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
                      <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">{category}</h2>
                      <span className="text-sm text-gray-500 dark:text-gray-400">({categoryApps.length})</span>
                    </div>
                  </button>

                  {/* Category apps */}
                  {!isCollapsed && (
                    <div className="p-4">
                      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
                        {categoryApps.map((app) => {
                          const currentIndex = appIndex++;
                          const isContainerApp = app.launch_type === 'container';
                          const cardClassName = `group bg-gray-50 dark:bg-gray-700 rounded-lg border p-4 hover:shadow-md transition-all duration-200 text-left w-full ${
                            focusedIndex === currentIndex
                              ? 'ring-2 ring-brand-primary border-brand-primary'
                              : 'border-gray-200 dark:border-gray-600 hover:border-brand-secondary'
                          }`;

                          const isFavorited = favorites.has(app.id);
                          const cardContent = (
                            <div className="flex items-start gap-3">
                              <div className="flex-shrink-0 w-12 h-12 bg-white dark:bg-gray-600 rounded-lg flex items-center justify-center overflow-hidden relative">
                                <img
                                  src={app.icon}
                                  alt={`${app.name} icon`}
                                  className="w-8 h-8 object-contain"
                                  onError={(e) => {
                                    (e.target as HTMLImageElement).src =
                                      'data:image/svg+xml,<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="%23398D9B"><rect width="24" height="24" rx="4"/><text x="12" y="16" text-anchor="middle" fill="white" font-size="12">' +
                                      app.name.charAt(0) +
                                      '</text></svg>';
                                  }}
                                />
                                {isContainerApp && (
                                  <div className="absolute -top-1 -right-1 w-4 h-4 bg-blue-500 rounded-full flex items-center justify-center" title="Container App">
                                    <svg className="w-2.5 h-2.5 text-white" fill="currentColor" viewBox="0 0 24 24">
                                      <path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/>
                                    </svg>
                                  </div>
                                )}
                              </div>
                              <div className="flex-1 min-w-0">
                                <h3 className="text-sm font-medium text-gray-900 dark:text-gray-100 group-hover:text-brand-primary truncate">
                                  {app.name}
                                </h3>
                                <p className="mt-1 text-xs text-gray-500 dark:text-gray-400 line-clamp-2">
                                  {app.description}
                                </p>
                              </div>
                              <div className="flex flex-col gap-1 flex-shrink-0">
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
                                <svg
                                  className="w-4 h-4 text-gray-400 group-hover:text-brand-primary"
                                  fill="none"
                                  stroke="currentColor"
                                  viewBox="0 0 24 24"
                                >
                                  {isContainerApp ? (
                                    <path
                                      strokeLinecap="round"
                                      strokeLinejoin="round"
                                      strokeWidth={2}
                                      d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"
                                    />
                                  ) : (
                                    <path
                                      strokeLinecap="round"
                                      strokeLinejoin="round"
                                      strokeWidth={2}
                                      d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"
                                    />
                                  )}
                                </svg>
                              </div>
                            </div>
                          );

                          if (isContainerApp) {
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
          </div>
        )}
      </main>

      {/* Footer */}
      <footer className="bg-white dark:bg-gray-800 border-t border-gray-200 dark:border-gray-700 mt-auto">
        <div className="max-w-7xl mx-auto px-4 py-4 sm:px-6 lg:px-8">
          <p className="text-center text-sm text-gray-500 dark:text-gray-400">
            Launchpad — Your centralized application launcher
          </p>
        </div>
      </footer>

      {/* Session Modal for container apps */}
      {selectedContainerApp && (
        <SessionModal
          app={selectedContainerApp}
          isOpen={!!selectedContainerApp}
          onClose={() => setSelectedContainerApp(null)}
          darkMode={darkMode}
        />
      )}

      {/* Template Browser Modal */}
      <TemplateBrowser
        isOpen={isTemplateBrowserOpen}
        onClose={() => setIsTemplateBrowserOpen(false)}
        onAddApp={handleAddApp}
        darkMode={darkMode}
      />

      {/* Admin Panel */}
      {showAdmin && isAdmin && (
        <Admin
          darkMode={darkMode}
          onClose={() => setShowAdmin(false)}
        />
      )}
    </div>
  );
}

export default App;
