import { useState, useEffect } from 'react';
import {
  getAdminSettings,
  updateAdminSettings,
  listUsers,
  createUser,
  deleteUser,
  listTemplates,
  createTemplate,
  updateTemplate,
  deleteTemplate,
  listAdminSessions,
  terminateAdminSession,
  listApps,
  createApp,
  updateApp,
  deleteApp,
  type AdminUser,
  type AdminTemplate,
} from '../services/auth';
import type { Application, Session, SessionStatus } from '../types';
import { formatDuration } from '../utils/time';

interface AdminProps {
  darkMode: boolean;
  onClose: () => void;
}

const TEMPLATE_CATEGORIES = [
  'development',
  'productivity',
  'communication',
  'browsers',
  'monitoring',
  'databases',
  'creative',
] as const;

const LAUNCH_TYPES = ['url', 'container', 'web_proxy'] as const;
const OS_TYPES = ['linux', 'windows'] as const;

const emptyTemplate: Omit<AdminTemplate, 'id' | 'created_at' | 'updated_at'> = {
  template_id: '',
  template_version: '1.0.0',
  template_category: 'development',
  name: '',
  description: '',
  url: '',
  icon: '',
  category: 'Development',
  launch_type: 'container',
  os_type: 'linux',
  container_image: '',
  container_port: 8080,
  container_args: [],
  tags: [],
  maintainer: '',
  documentation_url: '',
  recommended_limits: {
    cpu_request: '',
    cpu_limit: '',
    memory_request: '',
    memory_limit: '',
  },
};

const emptyApp: Application = {
  id: '',
  name: '',
  description: '',
  url: '',
  icon: '',
  category: '',
  launch_type: 'url',
  os_type: 'linux',
  container_image: '',
  container_port: 8080,
  container_args: [],
  resource_limits: {
    cpu_request: '',
    cpu_limit: '',
    memory_request: '',
    memory_limit: '',
  },
};

export function Admin({ darkMode, onClose }: AdminProps) {
  const [activeTab, setActiveTab] = useState<'settings' | 'users' | 'apps' | 'templates' | 'sessions'>('settings');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');

  // Settings state
  const [allowRegistration, setAllowRegistration] = useState(false);

  // Users state
  const [users, setUsers] = useState<AdminUser[]>([]);
  const [showCreateUser, setShowCreateUser] = useState(false);
  const [newUser, setNewUser] = useState({
    username: '',
    password: '',
    email: '',
    display_name: '',
    isAdmin: false,
  });

  // Sessions state
  const [adminSessions, setAdminSessions] = useState<Session[]>([]);

  // Apps state
  const [apps, setApps] = useState<Application[]>([]);
  const [showAppForm, setShowAppForm] = useState(false);
  const [editingApp, setEditingApp] = useState<Application | null>(null);
  const [appForm, setAppForm] = useState<Application>(emptyApp);
  const [appContainerArgsInput, setAppContainerArgsInput] = useState('');
  const [appSearch, setAppSearch] = useState('');

  // Templates state
  const [templates, setTemplates] = useState<AdminTemplate[]>([]);
  const [showTemplateForm, setShowTemplateForm] = useState(false);
  const [editingTemplate, setEditingTemplate] = useState<AdminTemplate | null>(null);
  const [templateForm, setTemplateForm] = useState<Omit<AdminTemplate, 'id' | 'created_at' | 'updated_at'>>(emptyTemplate);
  const [tagsInput, setTagsInput] = useState('');
  const [containerArgsInput, setContainerArgsInput] = useState('');

  useEffect(() => {
    loadData();
  }, []);

  const loadData = async () => {
    setLoading(true);
    setError('');
    try {
      const [settings, userList, appList, templateList, sessionList] = await Promise.all([
        getAdminSettings(),
        listUsers(),
        listApps(),
        listTemplates(),
        listAdminSessions(),
      ]);
      setAllowRegistration(settings.allow_registration === true || settings.allow_registration === 'true');
      setUsers(userList);
      setApps(appList);
      setTemplates(templateList);
      setAdminSessions(sessionList);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load data');
    } finally {
      setLoading(false);
    }
  };

  const handleSaveSettings = async () => {
    setError('');
    setSuccess('');
    try {
      await updateAdminSettings({
        allow_registration: allowRegistration.toString(),
      });
      setSuccess('Settings saved successfully');
      setTimeout(() => setSuccess(''), 3000);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save settings');
    }
  };

  const handleCreateUser = async () => {
    setError('');
    if (!newUser.username || !newUser.password) {
      setError('Username and password are required');
      return;
    }
    try {
      await createUser({
        username: newUser.username,
        password: newUser.password,
        email: newUser.email || undefined,
        display_name: newUser.display_name || undefined,
        roles: newUser.isAdmin ? ['admin', 'user'] : ['user'],
      });
      setNewUser({ username: '', password: '', email: '', display_name: '', isAdmin: false });
      setShowCreateUser(false);
      await loadData();
      setSuccess('User created successfully');
      setTimeout(() => setSuccess(''), 3000);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create user');
    }
  };

  const handleDeleteUser = async (user: AdminUser) => {
    if (!confirm(`Are you sure you want to delete user "${user.username}"?`)) {
      return;
    }
    setError('');
    try {
      await deleteUser(user.id);
      await loadData();
      setSuccess('User deleted successfully');
      setTimeout(() => setSuccess(''), 3000);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete user');
    }
  };

  // App CRUD handlers
  const handleOpenAppForm = (app?: Application) => {
    if (app) {
      setEditingApp(app);
      setAppForm({ ...app });
      setAppContainerArgsInput((app.container_args || []).join(', '));
    } else {
      setEditingApp(null);
      setAppForm({ ...emptyApp });
      setAppContainerArgsInput('');
    }
    setShowAppForm(true);
  };

  const handleCloseAppForm = () => {
    setShowAppForm(false);
    setEditingApp(null);
    setAppForm({ ...emptyApp });
    setAppContainerArgsInput('');
  };

  const handleSaveApp = async () => {
    setError('');

    if (!appForm.id) {
      setError('App ID is required');
      return;
    }
    if (!appForm.name) {
      setError('App name is required');
      return;
    }
    if (appForm.launch_type === 'url' && !appForm.url) {
      setError('URL is required for URL apps');
      return;
    }
    if ((appForm.launch_type === 'container' || appForm.launch_type === 'web_proxy') && !appForm.container_image) {
      setError('Container image is required for container/web proxy apps');
      return;
    }

    const containerArgs = appContainerArgsInput.split(',').map(a => a.trim()).filter(a => a);

    const appData: Application = {
      ...appForm,
      container_args: containerArgs,
    };

    try {
      if (editingApp) {
        await updateApp(editingApp.id, appData);
        setSuccess('App updated successfully');
      } else {
        await createApp(appData);
        setSuccess('App created successfully');
      }
      handleCloseAppForm();
      await loadData();
      setTimeout(() => setSuccess(''), 3000);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save app');
    }
  };

  const handleDeleteApp = async (app: Application) => {
    if (!confirm(`Are you sure you want to delete app "${app.name}"?`)) {
      return;
    }
    setError('');
    try {
      await deleteApp(app.id);
      await loadData();
      setSuccess('App deleted successfully');
      setTimeout(() => setSuccess(''), 3000);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete app');
    }
  };

  const filteredApps = apps.filter((app) => {
    const q = appSearch.toLowerCase();
    return !q || app.name.toLowerCase().includes(q) || app.id.toLowerCase().includes(q) || app.category.toLowerCase().includes(q);
  });

  const handleOpenTemplateForm = (template?: AdminTemplate) => {
    if (template) {
      setEditingTemplate(template);
      setTemplateForm({
        template_id: template.template_id,
        template_version: template.template_version,
        template_category: template.template_category,
        name: template.name,
        description: template.description,
        url: template.url || '',
        icon: template.icon || '',
        category: template.category,
        launch_type: template.launch_type,
        container_image: template.container_image || '',
        container_port: template.container_port || 8080,
        container_args: template.container_args || [],
        tags: template.tags || [],
        maintainer: template.maintainer || '',
        documentation_url: template.documentation_url || '',
        recommended_limits: template.recommended_limits || {
          cpu_request: '',
          cpu_limit: '',
          memory_request: '',
          memory_limit: '',
        },
      });
      setTagsInput((template.tags || []).join(', '));
      setContainerArgsInput((template.container_args || []).join(', '));
    } else {
      setEditingTemplate(null);
      setTemplateForm(emptyTemplate);
      setTagsInput('');
      setContainerArgsInput('');
    }
    setShowTemplateForm(true);
  };

  const handleCloseTemplateForm = () => {
    setShowTemplateForm(false);
    setEditingTemplate(null);
    setTemplateForm(emptyTemplate);
    setTagsInput('');
    setContainerArgsInput('');
  };

  const handleSaveTemplate = async () => {
    setError('');
    if (!templateForm.template_id || !templateForm.name) {
      setError('Template ID and Name are required');
      return;
    }
    if (!templateForm.template_category || !templateForm.category) {
      setError('Template Category and Display Category are required');
      return;
    }

    // Parse tags and container args from comma-separated strings
    const tags = tagsInput.split(',').map(t => t.trim()).filter(t => t);
    const containerArgs = containerArgsInput.split(',').map(a => a.trim()).filter(a => a);

    const templateData = {
      ...templateForm,
      tags,
      container_args: containerArgs,
    };

    try {
      if (editingTemplate) {
        await updateTemplate(editingTemplate.template_id, templateData);
        setSuccess('Template updated successfully');
      } else {
        await createTemplate(templateData);
        setSuccess('Template created successfully');
      }
      handleCloseTemplateForm();
      await loadData();
      setTimeout(() => setSuccess(''), 3000);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save template');
    }
  };

  const handleDeleteTemplate = async (template: AdminTemplate) => {
    if (!confirm(`Are you sure you want to delete template "${template.name}"?`)) {
      return;
    }
    setError('');
    try {
      await deleteTemplate(template.template_id);
      await loadData();
      setSuccess('Template deleted successfully');
      setTimeout(() => setSuccess(''), 3000);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete template');
    }
  };

  const handleTerminateSession = async (session: Session) => {
    if (!confirm(`Are you sure you want to terminate session "${session.id}"?`)) {
      return;
    }
    setError('');
    try {
      await terminateAdminSession(session.id);
      await loadData();
      setSuccess('Session terminated successfully');
      setTimeout(() => setSuccess(''), 3000);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to terminate session');
    }
  };

  const STATUS_COLORS: Record<SessionStatus, { bg: string; text: string; pulse?: boolean }> = {
    creating: { bg: 'bg-blue-500', text: 'text-blue-500', pulse: true },
    running: { bg: 'bg-green-500', text: 'text-green-500', pulse: true },
    failed: { bg: 'bg-red-500', text: 'text-red-500' },
    stopped: { bg: 'bg-gray-400', text: 'text-gray-400' },
    expired: { bg: 'bg-gray-400', text: 'text-gray-400' },
  };

  const bgColor = darkMode ? 'bg-gray-900' : 'bg-gray-100';
  const cardBg = darkMode ? 'bg-gray-800' : 'bg-white';
  const textColor = darkMode ? 'text-gray-100' : 'text-gray-900';
  const mutedText = darkMode ? 'text-gray-400' : 'text-gray-600';
  const inputBg = darkMode ? 'bg-gray-700 border-gray-600' : 'bg-white border-gray-300';
  const inputText = darkMode ? 'text-gray-100' : 'text-gray-900';

  return (
    <div className={`fixed inset-0 ${bgColor} z-50 overflow-auto`}>
      <div className="max-w-4xl mx-auto px-4 py-8">
        {/* Header */}
        <div className="flex items-center justify-between mb-8">
          <h1 className={`text-2xl font-bold ${textColor}`}>Admin Settings</h1>
          <button
            onClick={onClose}
            className={`p-2 rounded-lg ${darkMode ? 'hover:bg-gray-700' : 'hover:bg-gray-200'}`}
          >
            <svg className={`w-6 h-6 ${textColor}`} fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        {/* Tabs */}
        <div className="flex space-x-4 mb-6 border-b border-gray-700">
          <button
            onClick={() => setActiveTab('settings')}
            className={`pb-2 px-1 ${activeTab === 'settings'
              ? 'border-b-2 border-brand-accent text-brand-accent'
              : mutedText}`}
          >
            Settings
          </button>
          <button
            onClick={() => setActiveTab('users')}
            className={`pb-2 px-1 ${activeTab === 'users'
              ? 'border-b-2 border-brand-accent text-brand-accent'
              : mutedText}`}
          >
            Users
          </button>
          <button
            onClick={() => setActiveTab('apps')}
            className={`pb-2 px-1 ${activeTab === 'apps'
              ? 'border-b-2 border-brand-accent text-brand-accent'
              : mutedText}`}
          >
            Apps
          </button>
          <button
            onClick={() => setActiveTab('templates')}
            className={`pb-2 px-1 ${activeTab === 'templates'
              ? 'border-b-2 border-brand-accent text-brand-accent'
              : mutedText}`}
          >
            Templates
          </button>
          <button
            onClick={() => setActiveTab('sessions')}
            className={`pb-2 px-1 ${activeTab === 'sessions'
              ? 'border-b-2 border-brand-accent text-brand-accent'
              : mutedText}`}
          >
            Sessions
          </button>
        </div>

        {/* Error/Success messages */}
        {error && (
          <div className="mb-4 p-3 bg-red-500/20 border border-red-500 rounded-lg text-red-500">
            {error}
          </div>
        )}
        {success && (
          <div className="mb-4 p-3 bg-green-500/20 border border-green-500 rounded-lg text-green-500">
            {success}
          </div>
        )}

        {loading ? (
          <div className="flex justify-center py-12">
            <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-brand-accent"></div>
          </div>
        ) : (
          <>
            {/* Settings Tab */}
            {activeTab === 'settings' && (
              <div className={`${cardBg} rounded-lg p-6`}>
                <h2 className={`text-lg font-semibold mb-4 ${textColor}`}>Registration Settings</h2>

                <div className="space-y-4">
                  <label className="flex items-center space-x-3">
                    <input
                      type="checkbox"
                      checked={allowRegistration}
                      onChange={(e) => setAllowRegistration(e.target.checked)}
                      className="w-5 h-5 rounded border-gray-500 text-brand-accent focus:ring-brand-accent"
                    />
                    <div>
                      <span className={textColor}>Allow user self-registration</span>
                      <p className={`text-sm ${mutedText}`}>
                        When enabled, new users can create their own accounts
                      </p>
                    </div>
                  </label>
                </div>

                <div className="mt-6">
                  <button
                    onClick={handleSaveSettings}
                    className="px-4 py-2 bg-brand-accent text-white rounded-lg hover:bg-brand-primary transition-colors"
                  >
                    Save Settings
                  </button>
                </div>
              </div>
            )}

            {/* Users Tab */}
            {activeTab === 'users' && (
              <div className={`${cardBg} rounded-lg p-6`}>
                <div className="flex justify-between items-center mb-4">
                  <h2 className={`text-lg font-semibold ${textColor}`}>User Management</h2>
                  <button
                    onClick={() => setShowCreateUser(!showCreateUser)}
                    className="px-4 py-2 bg-brand-accent text-white rounded-lg hover:bg-brand-primary transition-colors"
                  >
                    {showCreateUser ? 'Cancel' : 'Create User'}
                  </button>
                </div>

                {/* Create User Form */}
                {showCreateUser && (
                  <div className={`mb-6 p-4 rounded-lg ${darkMode ? 'bg-gray-700' : 'bg-gray-100'}`}>
                    <h3 className={`font-medium mb-3 ${textColor}`}>Create New User</h3>
                    <div className="grid grid-cols-2 gap-4">
                      <input
                        type="text"
                        placeholder="Username *"
                        value={newUser.username}
                        onChange={(e) => setNewUser({ ...newUser, username: e.target.value })}
                        className={`px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                      />
                      <input
                        type="password"
                        placeholder="Password *"
                        value={newUser.password}
                        onChange={(e) => setNewUser({ ...newUser, password: e.target.value })}
                        className={`px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                      />
                      <input
                        type="email"
                        placeholder="Email (optional)"
                        value={newUser.email}
                        onChange={(e) => setNewUser({ ...newUser, email: e.target.value })}
                        className={`px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                      />
                      <input
                        type="text"
                        placeholder="Display Name (optional)"
                        value={newUser.display_name}
                        onChange={(e) => setNewUser({ ...newUser, display_name: e.target.value })}
                        className={`px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                      />
                    </div>
                    <label className="flex items-center space-x-2 mt-3">
                      <input
                        type="checkbox"
                        checked={newUser.isAdmin}
                        onChange={(e) => setNewUser({ ...newUser, isAdmin: e.target.checked })}
                        className="rounded border-gray-500"
                      />
                      <span className={textColor}>Admin user</span>
                    </label>
                    <button
                      onClick={handleCreateUser}
                      className="mt-4 px-4 py-2 bg-green-600 text-white rounded-lg hover:bg-green-700 transition-colors"
                    >
                      Create User
                    </button>
                  </div>
                )}

                {/* Users List */}
                <div className="overflow-x-auto">
                  <table className="w-full">
                    <thead>
                      <tr className={`border-b ${darkMode ? 'border-gray-700' : 'border-gray-200'}`}>
                        <th className={`text-left py-2 ${mutedText}`}>Username</th>
                        <th className={`text-left py-2 ${mutedText}`}>Email</th>
                        <th className={`text-left py-2 ${mutedText}`}>Roles</th>
                        <th className={`text-left py-2 ${mutedText}`}>Created</th>
                        <th className={`text-right py-2 ${mutedText}`}>Actions</th>
                      </tr>
                    </thead>
                    <tbody>
                      {users.map((user) => (
                        <tr
                          key={user.id}
                          className={`border-b ${darkMode ? 'border-gray-700' : 'border-gray-200'}`}
                        >
                          <td className={`py-3 ${textColor}`}>
                            {user.username}
                            {user.display_name && (
                              <span className={`ml-2 text-sm ${mutedText}`}>({user.display_name})</span>
                            )}
                          </td>
                          <td className={`py-3 ${mutedText}`}>{user.email || '-'}</td>
                          <td className="py-3">
                            {user.roles.map((role) => (
                              <span
                                key={role}
                                className={`inline-block px-2 py-0.5 mr-1 text-xs rounded ${
                                  role === 'admin'
                                    ? 'bg-purple-500/20 text-purple-400'
                                    : 'bg-blue-500/20 text-blue-400'
                                }`}
                              >
                                {role}
                              </span>
                            ))}
                          </td>
                          <td className={`py-3 ${mutedText}`}>
                            {new Date(user.created_at).toLocaleDateString()}
                          </td>
                          <td className="py-3 text-right">
                            <button
                              onClick={() => handleDeleteUser(user)}
                              className="text-red-500 hover:text-red-400 text-sm"
                              title="Delete user"
                            >
                              Delete
                            </button>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                  {users.length === 0 && (
                    <p className={`text-center py-8 ${mutedText}`}>No users found</p>
                  )}
                </div>
              </div>
            )}

            {/* Apps Tab */}
            {activeTab === 'apps' && (
              <div className={`${cardBg} rounded-lg p-6`}>
                <div className="flex justify-between items-center mb-4">
                  <h2 className={`text-lg font-semibold ${textColor}`}>App Catalog</h2>
                  <button
                    onClick={() => handleOpenAppForm()}
                    className="px-4 py-2 bg-brand-accent text-white rounded-lg hover:bg-brand-primary transition-colors"
                  >
                    Create App
                  </button>
                </div>

                {/* Search */}
                <div className="mb-4">
                  <input
                    type="text"
                    placeholder="Search apps by name, ID, or category..."
                    value={appSearch}
                    onChange={(e) => setAppSearch(e.target.value)}
                    className={`w-full px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                  />
                </div>

                {/* App Form Modal */}
                {showAppForm && (
                  <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
                    <div className={`${cardBg} rounded-lg p-6 max-w-3xl w-full mx-4 max-h-[90vh] overflow-y-auto`}>
                      <div className="flex justify-between items-center mb-4">
                        <h3 className={`text-lg font-semibold ${textColor}`}>
                          {editingApp ? 'Edit App' : 'Create New App'}
                        </h3>
                        <button
                          onClick={handleCloseAppForm}
                          className={`p-1 rounded ${darkMode ? 'hover:bg-gray-700' : 'hover:bg-gray-200'}`}
                        >
                          <svg className={`w-5 h-5 ${textColor}`} fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                          </svg>
                        </button>
                      </div>

                      <div className="grid grid-cols-2 gap-4">
                        {/* App ID - readonly when editing */}
                        <div>
                          <label className={`block text-sm mb-1 ${mutedText}`}>App ID *</label>
                          <input
                            type="text"
                            value={appForm.id}
                            onChange={(e) => setAppForm({ ...appForm, id: e.target.value })}
                            disabled={!!editingApp}
                            placeholder="e.g., my-app"
                            className={`w-full px-3 py-2 rounded-lg border ${inputBg} ${inputText} ${editingApp ? 'opacity-50' : ''}`}
                          />
                        </div>

                        <div>
                          <label className={`block text-sm mb-1 ${mutedText}`}>Name *</label>
                          <input
                            type="text"
                            value={appForm.name}
                            onChange={(e) => setAppForm({ ...appForm, name: e.target.value })}
                            placeholder="e.g., My Application"
                            className={`w-full px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                          />
                        </div>

                        <div className="col-span-2">
                          <label className={`block text-sm mb-1 ${mutedText}`}>Description</label>
                          <textarea
                            value={appForm.description}
                            onChange={(e) => setAppForm({ ...appForm, description: e.target.value })}
                            placeholder="Brief description of the application"
                            rows={2}
                            className={`w-full px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                          />
                        </div>

                        <div>
                          <label className={`block text-sm mb-1 ${mutedText}`}>Category *</label>
                          <input
                            type="text"
                            value={appForm.category}
                            onChange={(e) => setAppForm({ ...appForm, category: e.target.value })}
                            placeholder="e.g., Development"
                            className={`w-full px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                          />
                        </div>

                        <div>
                          <label className={`block text-sm mb-1 ${mutedText}`}>Launch Type *</label>
                          <select
                            value={appForm.launch_type}
                            onChange={(e) => setAppForm({ ...appForm, launch_type: e.target.value as Application['launch_type'] })}
                            className={`w-full px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                          >
                            {LAUNCH_TYPES.map((type) => (
                              <option key={type} value={type}>
                                {type === 'url' ? 'URL' : type === 'container' ? 'Container (VNC)' : 'Web Proxy'}
                              </option>
                            ))}
                          </select>
                        </div>

                        {/* URL for url launch type */}
                        {appForm.launch_type === 'url' && (
                          <div className="col-span-2">
                            <label className={`block text-sm mb-1 ${mutedText}`}>URL *</label>
                            <input
                              type="text"
                              value={appForm.url}
                              onChange={(e) => setAppForm({ ...appForm, url: e.target.value })}
                              placeholder="https://example.com"
                              className={`w-full px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                            />
                          </div>
                        )}

                        {/* OS Type - only for container apps */}
                        {appForm.launch_type === 'container' && (
                          <div>
                            <label className={`block text-sm mb-1 ${mutedText}`}>OS Type</label>
                            <select
                              value={appForm.os_type || 'linux'}
                              onChange={(e) => setAppForm({ ...appForm, os_type: e.target.value as Application['os_type'] })}
                              className={`w-full px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                            >
                              {OS_TYPES.map((type) => (
                                <option key={type} value={type}>
                                  {type === 'linux' ? 'Linux (VNC)' : 'Windows (RDP)'}
                                </option>
                              ))}
                            </select>
                          </div>
                        )}

                        {/* Container-specific fields */}
                        {(appForm.launch_type === 'container' || appForm.launch_type === 'web_proxy') && (
                          <>
                            <div>
                              <label className={`block text-sm mb-1 ${mutedText}`}>Container Image *</label>
                              <input
                                type="text"
                                value={appForm.container_image || ''}
                                onChange={(e) => setAppForm({ ...appForm, container_image: e.target.value })}
                                placeholder="e.g., nginx:latest"
                                className={`w-full px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                              />
                            </div>

                            <div>
                              <label className={`block text-sm mb-1 ${mutedText}`}>Container Port</label>
                              <input
                                type="number"
                                value={appForm.container_port || 8080}
                                onChange={(e) => setAppForm({ ...appForm, container_port: parseInt(e.target.value) || 8080 })}
                                placeholder="8080"
                                className={`w-full px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                              />
                            </div>

                            <div className="col-span-2">
                              <label className={`block text-sm mb-1 ${mutedText}`}>Container Args (comma-separated)</label>
                              <input
                                type="text"
                                value={appContainerArgsInput}
                                onChange={(e) => setAppContainerArgsInput(e.target.value)}
                                placeholder="e.g., --auth, none, --bind-addr, 0.0.0.0:8080"
                                className={`w-full px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                              />
                            </div>
                          </>
                        )}

                        <div>
                          <label className={`block text-sm mb-1 ${mutedText}`}>Icon URL</label>
                          <input
                            type="text"
                            value={appForm.icon}
                            onChange={(e) => setAppForm({ ...appForm, icon: e.target.value })}
                            placeholder="https://example.com/icon.png"
                            className={`w-full px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                          />
                        </div>

                        {/* Resource Limits */}
                        {(appForm.launch_type === 'container' || appForm.launch_type === 'web_proxy') && (
                          <div className="col-span-2">
                            <label className={`block text-sm mb-2 ${mutedText}`}>Resource Limits</label>
                            <div className="grid grid-cols-2 gap-4">
                              <div>
                                <label className={`block text-xs mb-1 ${mutedText}`}>CPU Request</label>
                                <input
                                  type="text"
                                  value={appForm.resource_limits?.cpu_request || ''}
                                  onChange={(e) => setAppForm({
                                    ...appForm,
                                    resource_limits: {
                                      ...appForm.resource_limits,
                                      cpu_request: e.target.value,
                                    },
                                  })}
                                  placeholder="e.g., 100m"
                                  className={`w-full px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                                />
                              </div>
                              <div>
                                <label className={`block text-xs mb-1 ${mutedText}`}>CPU Limit</label>
                                <input
                                  type="text"
                                  value={appForm.resource_limits?.cpu_limit || ''}
                                  onChange={(e) => setAppForm({
                                    ...appForm,
                                    resource_limits: {
                                      ...appForm.resource_limits,
                                      cpu_limit: e.target.value,
                                    },
                                  })}
                                  placeholder="e.g., 1"
                                  className={`w-full px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                                />
                              </div>
                              <div>
                                <label className={`block text-xs mb-1 ${mutedText}`}>Memory Request</label>
                                <input
                                  type="text"
                                  value={appForm.resource_limits?.memory_request || ''}
                                  onChange={(e) => setAppForm({
                                    ...appForm,
                                    resource_limits: {
                                      ...appForm.resource_limits,
                                      memory_request: e.target.value,
                                    },
                                  })}
                                  placeholder="e.g., 256Mi"
                                  className={`w-full px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                                />
                              </div>
                              <div>
                                <label className={`block text-xs mb-1 ${mutedText}`}>Memory Limit</label>
                                <input
                                  type="text"
                                  value={appForm.resource_limits?.memory_limit || ''}
                                  onChange={(e) => setAppForm({
                                    ...appForm,
                                    resource_limits: {
                                      ...appForm.resource_limits,
                                      memory_limit: e.target.value,
                                    },
                                  })}
                                  placeholder="e.g., 1Gi"
                                  className={`w-full px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                                />
                              </div>
                            </div>
                          </div>
                        )}
                      </div>

                      <div className="flex justify-end gap-3 mt-6">
                        <button
                          onClick={handleCloseAppForm}
                          className={`px-4 py-2 rounded-lg ${darkMode ? 'bg-gray-700 hover:bg-gray-600' : 'bg-gray-200 hover:bg-gray-300'} ${textColor}`}
                        >
                          Cancel
                        </button>
                        <button
                          onClick={handleSaveApp}
                          className="px-4 py-2 bg-brand-accent text-white rounded-lg hover:bg-brand-primary transition-colors"
                        >
                          {editingApp ? 'Update App' : 'Create App'}
                        </button>
                      </div>
                    </div>
                  </div>
                )}

                {/* Apps List */}
                <div className="overflow-x-auto">
                  <table className="w-full">
                    <thead>
                      <tr className={`border-b ${darkMode ? 'border-gray-700' : 'border-gray-200'}`}>
                        <th className={`text-left py-2 ${mutedText}`}>Name</th>
                        <th className={`text-left py-2 ${mutedText}`}>Category</th>
                        <th className={`text-left py-2 ${mutedText}`}>Launch Type</th>
                        <th className={`text-left py-2 ${mutedText}`}>URL / Image</th>
                        <th className={`text-right py-2 ${mutedText}`}>Actions</th>
                      </tr>
                    </thead>
                    <tbody>
                      {filteredApps.map((app) => (
                        <tr
                          key={app.id}
                          className={`border-b ${darkMode ? 'border-gray-700' : 'border-gray-200'}`}
                        >
                          <td className={`py-3 ${textColor}`}>
                            <div className="flex items-center gap-2">
                              {app.icon && (
                                <img
                                  src={app.icon}
                                  alt=""
                                  className="w-6 h-6 rounded"
                                  onError={(e) => { (e.target as HTMLImageElement).style.display = 'none'; }}
                                />
                              )}
                              <div>
                                <div className="font-medium">{app.name}</div>
                                <div className={`text-xs ${mutedText}`}>{app.id}</div>
                              </div>
                            </div>
                          </td>
                          <td className={`py-3 ${mutedText}`}>
                            <span className="inline-block px-2 py-0.5 text-xs rounded bg-blue-500/20 text-blue-400">
                              {app.category}
                            </span>
                          </td>
                          <td className={`py-3 ${mutedText}`}>
                            <span className={`inline-block px-2 py-0.5 text-xs rounded ${
                              app.launch_type === 'url'
                                ? 'bg-green-500/20 text-green-400'
                                : app.launch_type === 'web_proxy'
                                ? 'bg-purple-500/20 text-purple-400'
                                : 'bg-orange-500/20 text-orange-400'
                            }`}>
                              {app.launch_type}
                            </span>
                          </td>
                          <td className={`py-3 ${mutedText} text-sm truncate max-w-[200px]`}>
                            {app.launch_type === 'url' ? app.url : app.container_image}
                          </td>
                          <td className="py-3 text-right">
                            <button
                              onClick={() => handleOpenAppForm(app)}
                              className="text-blue-500 hover:text-blue-400 text-sm mr-3"
                              title="Edit app"
                            >
                              Edit
                            </button>
                            <button
                              onClick={() => handleDeleteApp(app)}
                              className="text-red-500 hover:text-red-400 text-sm"
                              title="Delete app"
                            >
                              Delete
                            </button>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                  {filteredApps.length === 0 && (
                    <p className={`text-center py-8 ${mutedText}`}>
                      {appSearch ? 'No apps match your search.' : 'No apps found. Click "Create App" to add one.'}
                    </p>
                  )}
                </div>
              </div>
            )}

            {/* Templates Tab */}
            {activeTab === 'templates' && (
              <div className={`${cardBg} rounded-lg p-6`}>
                <div className="flex justify-between items-center mb-4">
                  <h2 className={`text-lg font-semibold ${textColor}`}>Template Management</h2>
                  <button
                    onClick={() => handleOpenTemplateForm()}
                    className="px-4 py-2 bg-brand-accent text-white rounded-lg hover:bg-brand-primary transition-colors"
                  >
                    Create Template
                  </button>
                </div>

                {/* Template Form Modal */}
                {showTemplateForm && (
                  <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
                    <div className={`${cardBg} rounded-lg p-6 max-w-3xl w-full mx-4 max-h-[90vh] overflow-y-auto`}>
                      <div className="flex justify-between items-center mb-4">
                        <h3 className={`text-lg font-semibold ${textColor}`}>
                          {editingTemplate ? 'Edit Template' : 'Create New Template'}
                        </h3>
                        <button
                          onClick={handleCloseTemplateForm}
                          className={`p-1 rounded ${darkMode ? 'hover:bg-gray-700' : 'hover:bg-gray-200'}`}
                        >
                          <svg className={`w-5 h-5 ${textColor}`} fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                          </svg>
                        </button>
                      </div>

                      <div className="grid grid-cols-2 gap-4">
                        {/* Template ID - readonly when editing */}
                        <div>
                          <label className={`block text-sm mb-1 ${mutedText}`}>Template ID *</label>
                          <input
                            type="text"
                            value={templateForm.template_id}
                            onChange={(e) => setTemplateForm({ ...templateForm, template_id: e.target.value })}
                            disabled={!!editingTemplate}
                            placeholder="e.g., my-app"
                            className={`w-full px-3 py-2 rounded-lg border ${inputBg} ${inputText} ${editingTemplate ? 'opacity-50' : ''}`}
                          />
                        </div>

                        <div>
                          <label className={`block text-sm mb-1 ${mutedText}`}>Name *</label>
                          <input
                            type="text"
                            value={templateForm.name}
                            onChange={(e) => setTemplateForm({ ...templateForm, name: e.target.value })}
                            placeholder="e.g., My Application"
                            className={`w-full px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                          />
                        </div>

                        <div className="col-span-2">
                          <label className={`block text-sm mb-1 ${mutedText}`}>Description *</label>
                          <textarea
                            value={templateForm.description}
                            onChange={(e) => setTemplateForm({ ...templateForm, description: e.target.value })}
                            placeholder="Brief description of the application"
                            rows={2}
                            className={`w-full px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                          />
                        </div>

                        <div>
                          <label className={`block text-sm mb-1 ${mutedText}`}>Template Category *</label>
                          <select
                            value={templateForm.template_category}
                            onChange={(e) => setTemplateForm({ ...templateForm, template_category: e.target.value })}
                            className={`w-full px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                          >
                            {TEMPLATE_CATEGORIES.map((cat) => (
                              <option key={cat} value={cat}>
                                {cat.charAt(0).toUpperCase() + cat.slice(1)}
                              </option>
                            ))}
                          </select>
                        </div>

                        <div>
                          <label className={`block text-sm mb-1 ${mutedText}`}>Display Category *</label>
                          <input
                            type="text"
                            value={templateForm.category}
                            onChange={(e) => setTemplateForm({ ...templateForm, category: e.target.value })}
                            placeholder="e.g., Development"
                            className={`w-full px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                          />
                        </div>

                        <div>
                          <label className={`block text-sm mb-1 ${mutedText}`}>Launch Type *</label>
                          <select
                            value={templateForm.launch_type}
                            onChange={(e) => setTemplateForm({ ...templateForm, launch_type: e.target.value })}
                            className={`w-full px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                          >
                            {LAUNCH_TYPES.map((type) => (
                              <option key={type} value={type}>
                                {type === 'url' ? 'URL' : type === 'container' ? 'Container (VNC)' : 'Web Proxy'}
                              </option>
                            ))}
                          </select>
                        </div>

                        {/* OS Type - only shown for container apps */}
                        {templateForm.launch_type === 'container' && (
                          <div>
                            <label className={`block text-sm mb-1 ${mutedText}`}>OS Type</label>
                            <select
                              value={templateForm.os_type || 'linux'}
                              onChange={(e) => setTemplateForm({ ...templateForm, os_type: e.target.value })}
                              className={`w-full px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                            >
                              {OS_TYPES.map((type) => (
                                <option key={type} value={type}>
                                  {type === 'linux' ? 'Linux (VNC)' : 'Windows (RDP)'}
                                </option>
                              ))}
                            </select>
                          </div>
                        )}

                        <div>
                          <label className={`block text-sm mb-1 ${mutedText}`}>Version</label>
                          <input
                            type="text"
                            value={templateForm.template_version}
                            onChange={(e) => setTemplateForm({ ...templateForm, template_version: e.target.value })}
                            placeholder="1.0.0"
                            className={`w-full px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                          />
                        </div>

                        {/* Container-specific fields */}
                        {(templateForm.launch_type === 'container' || templateForm.launch_type === 'web_proxy') && (
                          <>
                            <div>
                              <label className={`block text-sm mb-1 ${mutedText}`}>Container Image *</label>
                              <input
                                type="text"
                                value={templateForm.container_image}
                                onChange={(e) => setTemplateForm({ ...templateForm, container_image: e.target.value })}
                                placeholder="e.g., nginx:latest"
                                className={`w-full px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                              />
                            </div>

                            <div>
                              <label className={`block text-sm mb-1 ${mutedText}`}>Container Port</label>
                              <input
                                type="number"
                                value={templateForm.container_port}
                                onChange={(e) => setTemplateForm({ ...templateForm, container_port: parseInt(e.target.value) || 8080 })}
                                placeholder="8080"
                                className={`w-full px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                              />
                            </div>

                            <div className="col-span-2">
                              <label className={`block text-sm mb-1 ${mutedText}`}>Container Args (comma-separated)</label>
                              <input
                                type="text"
                                value={containerArgsInput}
                                onChange={(e) => setContainerArgsInput(e.target.value)}
                                placeholder="e.g., --auth, none, --bind-addr, 0.0.0.0:8080"
                                className={`w-full px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                              />
                            </div>
                          </>
                        )}

                        {/* URL for url launch type */}
                        {templateForm.launch_type === 'url' && (
                          <div className="col-span-2">
                            <label className={`block text-sm mb-1 ${mutedText}`}>URL *</label>
                            <input
                              type="text"
                              value={templateForm.url}
                              onChange={(e) => setTemplateForm({ ...templateForm, url: e.target.value })}
                              placeholder="https://example.com"
                              className={`w-full px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                            />
                          </div>
                        )}

                        <div>
                          <label className={`block text-sm mb-1 ${mutedText}`}>Icon URL</label>
                          <input
                            type="text"
                            value={templateForm.icon}
                            onChange={(e) => setTemplateForm({ ...templateForm, icon: e.target.value })}
                            placeholder="https://example.com/icon.png"
                            className={`w-full px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                          />
                        </div>

                        <div>
                          <label className={`block text-sm mb-1 ${mutedText}`}>Maintainer</label>
                          <input
                            type="text"
                            value={templateForm.maintainer}
                            onChange={(e) => setTemplateForm({ ...templateForm, maintainer: e.target.value })}
                            placeholder="e.g., My Organization"
                            className={`w-full px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                          />
                        </div>

                        <div className="col-span-2">
                          <label className={`block text-sm mb-1 ${mutedText}`}>Documentation URL</label>
                          <input
                            type="text"
                            value={templateForm.documentation_url}
                            onChange={(e) => setTemplateForm({ ...templateForm, documentation_url: e.target.value })}
                            placeholder="https://docs.example.com"
                            className={`w-full px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                          />
                        </div>

                        <div className="col-span-2">
                          <label className={`block text-sm mb-1 ${mutedText}`}>Tags (comma-separated)</label>
                          <input
                            type="text"
                            value={tagsInput}
                            onChange={(e) => setTagsInput(e.target.value)}
                            placeholder="e.g., web, development, tools"
                            className={`w-full px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                          />
                        </div>

                        {/* Resource Limits */}
                        <div className="col-span-2">
                          <label className={`block text-sm mb-2 ${mutedText}`}>Resource Limits</label>
                          <div className="grid grid-cols-2 gap-4">
                            <div>
                              <label className={`block text-xs mb-1 ${mutedText}`}>CPU Request</label>
                              <input
                                type="text"
                                value={templateForm.recommended_limits?.cpu_request || ''}
                                onChange={(e) => setTemplateForm({
                                  ...templateForm,
                                  recommended_limits: {
                                    ...templateForm.recommended_limits,
                                    cpu_request: e.target.value,
                                  },
                                })}
                                placeholder="e.g., 100m"
                                className={`w-full px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                              />
                            </div>
                            <div>
                              <label className={`block text-xs mb-1 ${mutedText}`}>CPU Limit</label>
                              <input
                                type="text"
                                value={templateForm.recommended_limits?.cpu_limit || ''}
                                onChange={(e) => setTemplateForm({
                                  ...templateForm,
                                  recommended_limits: {
                                    ...templateForm.recommended_limits,
                                    cpu_limit: e.target.value,
                                  },
                                })}
                                placeholder="e.g., 1"
                                className={`w-full px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                              />
                            </div>
                            <div>
                              <label className={`block text-xs mb-1 ${mutedText}`}>Memory Request</label>
                              <input
                                type="text"
                                value={templateForm.recommended_limits?.memory_request || ''}
                                onChange={(e) => setTemplateForm({
                                  ...templateForm,
                                  recommended_limits: {
                                    ...templateForm.recommended_limits,
                                    memory_request: e.target.value,
                                  },
                                })}
                                placeholder="e.g., 256Mi"
                                className={`w-full px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                              />
                            </div>
                            <div>
                              <label className={`block text-xs mb-1 ${mutedText}`}>Memory Limit</label>
                              <input
                                type="text"
                                value={templateForm.recommended_limits?.memory_limit || ''}
                                onChange={(e) => setTemplateForm({
                                  ...templateForm,
                                  recommended_limits: {
                                    ...templateForm.recommended_limits,
                                    memory_limit: e.target.value,
                                  },
                                })}
                                placeholder="e.g., 1Gi"
                                className={`w-full px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                              />
                            </div>
                          </div>
                        </div>
                      </div>

                      <div className="flex justify-end gap-3 mt-6">
                        <button
                          onClick={handleCloseTemplateForm}
                          className={`px-4 py-2 rounded-lg ${darkMode ? 'bg-gray-700 hover:bg-gray-600' : 'bg-gray-200 hover:bg-gray-300'} ${textColor}`}
                        >
                          Cancel
                        </button>
                        <button
                          onClick={handleSaveTemplate}
                          className="px-4 py-2 bg-brand-accent text-white rounded-lg hover:bg-brand-primary transition-colors"
                        >
                          {editingTemplate ? 'Update Template' : 'Create Template'}
                        </button>
                      </div>
                    </div>
                  </div>
                )}

                {/* Templates List */}
                <div className="overflow-x-auto">
                  <table className="w-full">
                    <thead>
                      <tr className={`border-b ${darkMode ? 'border-gray-700' : 'border-gray-200'}`}>
                        <th className={`text-left py-2 ${mutedText}`}>Name</th>
                        <th className={`text-left py-2 ${mutedText}`}>Category</th>
                        <th className={`text-left py-2 ${mutedText}`}>Launch Type</th>
                        <th className={`text-left py-2 ${mutedText}`}>Tags</th>
                        <th className={`text-right py-2 ${mutedText}`}>Actions</th>
                      </tr>
                    </thead>
                    <tbody>
                      {templates.map((template) => (
                        <tr
                          key={template.template_id}
                          className={`border-b ${darkMode ? 'border-gray-700' : 'border-gray-200'}`}
                        >
                          <td className={`py-3 ${textColor}`}>
                            <div className="flex items-center gap-2">
                              {template.icon && (
                                <img
                                  src={template.icon}
                                  alt=""
                                  className="w-6 h-6 rounded"
                                  onError={(e) => { (e.target as HTMLImageElement).style.display = 'none'; }}
                                />
                              )}
                              <div>
                                <div className="font-medium">{template.name}</div>
                                <div className={`text-xs ${mutedText}`}>{template.template_id}</div>
                              </div>
                            </div>
                          </td>
                          <td className={`py-3 ${mutedText}`}>
                            <span className="inline-block px-2 py-0.5 text-xs rounded bg-blue-500/20 text-blue-400">
                              {template.template_category}
                            </span>
                          </td>
                          <td className={`py-3 ${mutedText}`}>
                            <span className={`inline-block px-2 py-0.5 text-xs rounded ${
                              template.launch_type === 'url'
                                ? 'bg-green-500/20 text-green-400'
                                : template.launch_type === 'web_proxy'
                                ? 'bg-purple-500/20 text-purple-400'
                                : 'bg-orange-500/20 text-orange-400'
                            }`}>
                              {template.launch_type}
                            </span>
                          </td>
                          <td className="py-3">
                            <div className="flex flex-wrap gap-1">
                              {(template.tags || []).slice(0, 3).map((tag) => (
                                <span
                                  key={tag}
                                  className={`inline-block px-2 py-0.5 text-xs rounded ${darkMode ? 'bg-gray-700' : 'bg-gray-200'} ${mutedText}`}
                                >
                                  {tag}
                                </span>
                              ))}
                              {(template.tags || []).length > 3 && (
                                <span className={`text-xs ${mutedText}`}>
                                  +{template.tags.length - 3}
                                </span>
                              )}
                            </div>
                          </td>
                          <td className="py-3 text-right">
                            <button
                              onClick={() => handleOpenTemplateForm(template)}
                              className="text-blue-500 hover:text-blue-400 text-sm mr-3"
                              title="Edit template"
                            >
                              Edit
                            </button>
                            <button
                              onClick={() => handleDeleteTemplate(template)}
                              className="text-red-500 hover:text-red-400 text-sm"
                              title="Delete template"
                            >
                              Delete
                            </button>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                  {templates.length === 0 && (
                    <p className={`text-center py-8 ${mutedText}`}>No templates found. Click "Create Template" to add one.</p>
                  )}
                </div>
              </div>
            )}

            {/* Sessions Tab */}
            {activeTab === 'sessions' && (
              <div className={`${cardBg} rounded-lg p-6`}>
                <div className="flex justify-between items-center mb-4">
                  <h2 className={`text-lg font-semibold ${textColor}`}>Session Management</h2>
                  <button
                    onClick={loadData}
                    className="px-4 py-2 bg-brand-accent text-white rounded-lg hover:bg-brand-primary transition-colors"
                  >
                    Refresh
                  </button>
                </div>

                <div className="overflow-x-auto">
                  <table className="w-full">
                    <thead>
                      <tr className={`border-b ${darkMode ? 'border-gray-700' : 'border-gray-200'}`}>
                        <th className={`text-left py-2 ${mutedText}`}>Status</th>
                        <th className={`text-left py-2 ${mutedText}`}>App Name</th>
                        <th className={`text-left py-2 ${mutedText}`}>User ID</th>
                        <th className={`text-left py-2 ${mutedText}`}>Started</th>
                        <th className={`text-right py-2 ${mutedText}`}>Actions</th>
                      </tr>
                    </thead>
                    <tbody>
                      {adminSessions.map((session) => {
                        const statusConfig = STATUS_COLORS[session.status];
                        return (
                          <tr
                            key={session.id}
                            className={`border-b ${darkMode ? 'border-gray-700' : 'border-gray-200'}`}
                          >
                            <td className="py-3">
                              <div className="flex items-center gap-2">
                                <span className={`inline-block w-2 h-2 rounded-full ${statusConfig.bg} ${statusConfig.pulse ? 'animate-pulse' : ''}`} />
                                <span className={`text-sm ${statusConfig.text}`}>{session.status}</span>
                              </div>
                            </td>
                            <td className={`py-3 ${textColor}`}>{session.app_name || session.app_id}</td>
                            <td className={`py-3 ${mutedText}`}>{session.user_id}</td>
                            <td className={`py-3 ${mutedText}`}>{formatDuration(session.created_at)}</td>
                            <td className="py-3 text-right">
                              {(session.status === 'running' || session.status === 'creating') && (
                                <button
                                  onClick={() => handleTerminateSession(session)}
                                  className="text-red-500 hover:text-red-400 text-sm"
                                  title="Terminate session"
                                >
                                  Terminate
                                </button>
                              )}
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                  {adminSessions.length === 0 && (
                    <p className={`text-center py-8 ${mutedText}`}>No sessions found</p>
                  )}
                </div>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
}
