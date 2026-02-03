import { useState, type FormEvent } from 'react';
import type { User } from '../types';
import { register as authRegister } from '../services/auth';

interface RegisterProps {
  onRegister: (user: User) => void;
  onBackToLogin: () => void;
  darkMode: boolean;
}

export function Register({ onRegister, onBackToLogin, darkMode }: RegisterProps) {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [email, setEmail] = useState('');
  const [displayName, setDisplayName] = useState('');
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

    if (password.length < 6) {
      setError('Password must be at least 6 characters');
      return;
    }

    if (password !== confirmPassword) {
      setError('Passwords do not match');
      return;
    }

    if (!email.trim()) {
      setError('Email is required');
      return;
    }

    setLoading(true);

    try {
      const response = await authRegister(
        username.trim(),
        password,
        email.trim() || undefined,
        displayName.trim() || undefined
      );

      const user: User = {
        id: response.user.id,
        username: response.user.username,
        displayName: response.user.name || response.user.username,
        email: response.user.email,
        roles: response.user.roles,
      };

      onRegister(user);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Registration failed';
      setError(message);
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
          <div className="w-16 h-16 bg-brand-primary rounded-xl flex items-center justify-center">
            <svg
              className="w-10 h-10 text-white"
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
        </div>

        <h1 className={`text-2xl font-bold text-center mb-2 ${textColor}`}>
          Create Account
        </h1>
        <p className={`text-center mb-6 ${darkMode ? 'text-gray-400' : 'text-gray-600'}`}>
          Register to access Launchpad
        </p>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label htmlFor="username" className={`block text-sm font-medium mb-1 ${textColor}`}>
              Username *
            </label>
            <input
              id="username"
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              className={`w-full px-4 py-2 rounded-lg border ${inputBg} ${inputText} focus:outline-none focus:ring-2 focus:ring-brand-primary`}
              placeholder="Choose a username"
              autoComplete="username"
              autoFocus
            />
          </div>

          <div>
            <label htmlFor="email" className={`block text-sm font-medium mb-1 ${textColor}`}>
              Email *
            </label>
            <input
              id="email"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className={`w-full px-4 py-2 rounded-lg border ${inputBg} ${inputText} focus:outline-none focus:ring-2 focus:ring-brand-primary`}
              placeholder="your@email.com"
              autoComplete="email"
            />
          </div>

          <div>
            <label htmlFor="displayName" className={`block text-sm font-medium mb-1 ${textColor}`}>
              Display Name
            </label>
            <input
              id="displayName"
              type="text"
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              className={`w-full px-4 py-2 rounded-lg border ${inputBg} ${inputText} focus:outline-none focus:ring-2 focus:ring-brand-primary`}
              placeholder="Your display name (optional)"
            />
          </div>

          <div>
            <label htmlFor="password" className={`block text-sm font-medium mb-1 ${textColor}`}>
              Password *
            </label>
            <input
              id="password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className={`w-full px-4 py-2 rounded-lg border ${inputBg} ${inputText} focus:outline-none focus:ring-2 focus:ring-brand-primary`}
              placeholder="At least 6 characters"
              autoComplete="new-password"
            />
          </div>

          <div>
            <label htmlFor="confirmPassword" className={`block text-sm font-medium mb-1 ${textColor}`}>
              Confirm Password *
            </label>
            <input
              id="confirmPassword"
              type="password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              className={`w-full px-4 py-2 rounded-lg border ${inputBg} ${inputText} focus:outline-none focus:ring-2 focus:ring-brand-primary`}
              placeholder="Confirm your password"
              autoComplete="new-password"
            />
          </div>

          {error && (
            <p className="text-red-500 text-sm">{error}</p>
          )}

          <button
            type="submit"
            disabled={loading}
            className="w-full py-2 px-4 bg-brand-primary text-white font-medium rounded-lg hover:bg-brand-secondary transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {loading ? 'Creating account...' : 'Create Account'}
          </button>
        </form>

        <div className="mt-6 text-center">
          <button
            onClick={onBackToLogin}
            className={`text-sm ${darkMode ? 'text-gray-400 hover:text-gray-300' : 'text-gray-600 hover:text-gray-800'}`}
          >
            Already have an account? <span className="text-brand-primary font-medium">Sign in</span>
          </button>
        </div>
      </div>
    </div>
  );
}
