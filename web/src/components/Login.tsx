import { useState, type FormEvent } from 'react';
import type { User } from '../types';
import { login as authLogin } from '../services/auth';
import sortieIconFull from '../assets/sortie-icon-full.svg';

interface LoginProps {
  onLogin: (user: User) => void;
  onShowRegister?: () => void;
  allowRegistration?: boolean;
  darkMode: boolean;
}

export function Login({ onLogin, onShowRegister, allowRegistration, darkMode }: LoginProps) {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setError('');

    if (!username.trim()) {
      setError('Username is required');
      return;
    }

    if (!password) {
      setError('Password is required');
      return;
    }

    setLoading(true);

    try {
      const response = await authLogin(username.trim(), password);

      const user: User = {
        id: response.user.id,
        username: response.user.username,
        displayName: response.user.name || response.user.username,
        email: response.user.email,
        roles: response.user.roles,
      };

      onLogin(user);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Login failed';
      setError(message === 'Invalid credentials' ? 'Invalid username or password' : message);
    } finally {
      setLoading(false);
    }
  };

  const bgColor = darkMode ? 'bg-gray-900' : 'bg-gray-50';
  const cardBg = darkMode ? 'bg-gray-800' : 'bg-white';
  const textColor = darkMode ? 'text-gray-100' : 'text-gray-900';
  const inputBg = darkMode ? 'bg-gray-700 border-gray-600' : 'bg-white border-gray-300';
  const inputText = darkMode ? 'text-gray-100 placeholder-gray-400' : 'text-gray-900 placeholder-gray-500';

  return (
    <div className={`min-h-screen flex items-center justify-center ${bgColor} px-4`}>
      <div className={`w-full max-w-md ${cardBg} rounded-xl shadow-lg p-8`}>
        {/* Logo */}
        <div className="flex justify-center mb-6">
          <img src={sortieIconFull} alt="Sortie" className="w-16 h-16" />
        </div>

        <h1 className={`text-2xl font-bold text-center mb-2 ${textColor}`}>
          Sortie
        </h1>
        <p className={`text-center mb-8 ${darkMode ? 'text-gray-400' : 'text-gray-600'}`}>
          Sign in to access your applications
        </p>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label htmlFor="username" className={`block text-sm font-medium mb-1 ${textColor}`}>
              Username
            </label>
            <input
              id="username"
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              className={`w-full px-4 py-2 rounded-lg border ${inputBg} ${inputText} focus:outline-none focus:ring-2 focus:ring-brand-accent`}
              placeholder="Enter your username"
              autoComplete="username"
              autoFocus
            />
          </div>

          <div>
            <label htmlFor="password" className={`block text-sm font-medium mb-1 ${textColor}`}>
              Password
            </label>
            <input
              id="password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className={`w-full px-4 py-2 rounded-lg border ${inputBg} ${inputText} focus:outline-none focus:ring-2 focus:ring-brand-accent`}
              placeholder="Enter your password"
              autoComplete="current-password"
            />
          </div>

          {error && (
            <p className="text-red-500 text-sm">{error}</p>
          )}

          <button
            type="submit"
            disabled={loading}
            className="w-full py-2 px-4 bg-brand-accent text-white font-medium rounded-lg hover:bg-brand-primary transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {loading ? 'Signing in...' : 'Sign In'}
          </button>
        </form>

        {allowRegistration && onShowRegister && (
          <div className="mt-6 text-center">
            <button
              onClick={onShowRegister}
              className={`text-sm ${darkMode ? 'text-gray-400 hover:text-gray-300' : 'text-gray-600 hover:text-gray-800'}`}
            >
              Don't have an account? <span className="text-brand-accent font-medium">Register</span>
            </button>
          </div>
        )}
      </div>
    </div>
  );
}
