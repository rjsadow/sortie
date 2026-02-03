import { useState, useEffect } from 'react';
import {
  getAdminSettings,
  updateAdminSettings,
  listUsers,
  createUser,
  deleteUser,
  type AdminUser,
} from '../services/auth';

interface AdminProps {
  darkMode: boolean;
  onClose: () => void;
}

export function Admin({ darkMode, onClose }: AdminProps) {
  const [activeTab, setActiveTab] = useState<'settings' | 'users'>('settings');
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

  useEffect(() => {
    loadData();
  }, []);

  const loadData = async () => {
    setLoading(true);
    setError('');
    try {
      const [settings, userList] = await Promise.all([
        getAdminSettings(),
        listUsers(),
      ]);
      setAllowRegistration(settings.allow_registration === true || settings.allow_registration === 'true');
      setUsers(userList);
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
              ? 'border-b-2 border-brand-primary text-brand-primary'
              : mutedText}`}
          >
            Settings
          </button>
          <button
            onClick={() => setActiveTab('users')}
            className={`pb-2 px-1 ${activeTab === 'users'
              ? 'border-b-2 border-brand-primary text-brand-primary'
              : mutedText}`}
          >
            Users
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
            <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-brand-primary"></div>
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
                      className="w-5 h-5 rounded border-gray-500 text-brand-primary focus:ring-brand-primary"
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
                    className="px-4 py-2 bg-brand-primary text-white rounded-lg hover:bg-brand-secondary transition-colors"
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
                    className="px-4 py-2 bg-brand-primary text-white rounded-lg hover:bg-brand-secondary transition-colors"
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
          </>
        )}
      </div>
    </div>
  );
}
