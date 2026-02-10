import { useState, type FormEvent } from 'react';
import type { User } from '../types';
import { register as authRegister } from '../services/auth';
import sortieIconFull from '../assets/sortie-icon-full.svg';

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

  const textColor = darkMode ? 'text-gray-100' : 'text-gray-900';
  const inputBg = darkMode ? 'bg-gray-700 border-gray-600' : 'bg-white border-gray-300';
  const inputText = darkMode ? 'text-gray-100 placeholder-gray-400' : 'text-gray-900 placeholder-gray-500';

  return (
    <div className="min-h-screen flex items-center justify-center bg-brand-primary px-4 relative overflow-hidden">
      {/* Aurora background ribbons */}
      <div
        className="absolute top-[-20%] left-[-25%] w-[900px] h-[350px] rounded-full bg-brand-accent/40 blur-[80px]"
        style={{ animation: 'aurora-1 25s ease-in-out infinite' }}
      />
      <div
        className="absolute top-[5%] right-[-20%] w-[800px] h-[300px] rounded-full bg-aurora-teal/50 blur-[70px]"
        style={{ animation: 'aurora-2 30s ease-in-out infinite' }}
      />
      <div
        className="absolute bottom-[-15%] left-[-15%] w-[1000px] h-[320px] rounded-full bg-aurora-sage/35 blur-[90px]"
        style={{ animation: 'aurora-3 28s ease-in-out infinite' }}
      />
      <div
        className="absolute top-[40%] left-[20%] w-[600px] h-[250px] rounded-full bg-brand-primary-light/50 blur-[60px]"
        style={{ animation: 'aurora-4 22s ease-in-out infinite' }}
      />
      <div
        className="absolute top-[15%] left-[40%] w-[500px] h-[200px] rounded-full bg-brand-accent-muted/40 blur-[70px]"
        style={{ animation: 'aurora-5 26s ease-in-out infinite' }}
      />

      <div className={`relative w-full max-w-md rounded-2xl shadow-2xl p-8 backdrop-blur-xl border border-white/15 ${darkMode ? 'bg-gray-800/50' : 'bg-white/50'}`}>
        {/* Logo */}
        <div className="flex justify-center mb-6">
          <img src={sortieIconFull} alt="Sortie" className="w-16 h-16" />
        </div>

        <h1 className={`text-2xl font-bold text-center mb-2 ${textColor}`}>
          Create Account
        </h1>
        <p className={`text-center mb-6 ${darkMode ? 'text-gray-400' : 'text-gray-600'}`}>
          Register to access Sortie
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
              className={`w-full px-4 py-2 rounded-lg border shadow-sm ${inputBg} ${inputText} focus:outline-none focus:ring-2 focus:ring-brand-accent`}
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
              className={`w-full px-4 py-2 rounded-lg border shadow-sm ${inputBg} ${inputText} focus:outline-none focus:ring-2 focus:ring-brand-accent`}
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
              className={`w-full px-4 py-2 rounded-lg border shadow-sm ${inputBg} ${inputText} focus:outline-none focus:ring-2 focus:ring-brand-accent`}
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
              className={`w-full px-4 py-2 rounded-lg border shadow-sm ${inputBg} ${inputText} focus:outline-none focus:ring-2 focus:ring-brand-accent`}
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
              className={`w-full px-4 py-2 rounded-lg border shadow-sm ${inputBg} ${inputText} focus:outline-none focus:ring-2 focus:ring-brand-accent`}
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
            className="w-full py-2 px-4 bg-brand-accent text-white font-medium rounded-lg hover:bg-brand-primary transition-colors shadow-md hover:shadow-lg disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {loading ? 'Creating account...' : 'Create Account'}
          </button>
        </form>

        <div className="mt-6 text-center">
          <button
            onClick={onBackToLogin}
            className={`text-sm ${darkMode ? 'text-gray-400 hover:text-gray-300' : 'text-gray-600 hover:text-gray-800'}`}
          >
            Already have an account? <span className="text-brand-accent font-medium">Sign in</span>
          </button>
        </div>
      </div>
    </div>
  );
}
