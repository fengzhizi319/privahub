import React, { useState } from 'react';
import { useAuthStore } from '../../features/auth/model/auth-store';
import { Button, Card } from '@secretpad/design-system';
import { useTranslation } from '../../shared/lib/i18n';

export const LoginPage: React.FC<{ onLoginSuccess: () => void }> = ({ onLoginSuccess }) => {
  const { t } = useTranslation();
  const [username, setUsername] = useState('admin');
  const [password, setPassword] = useState('12345678');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const { login } = useAuthStore();

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError(null);
    try {
      await login(username, password);
      onLoginSuccess();
    } catch (err) {
      setError(err instanceof Error ? err.message : t('login.error'));
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen w-full flex items-center justify-center bg-gradient-to-br from-gray-950 via-slate-900 to-blue-950 p-4">
      <Card className="w-full max-w-md bg-gray-900/90 border-gray-800 backdrop-blur-md shadow-2xl">
        <div className="text-center mb-8">
          <div className="w-14 h-14 mx-auto mb-4 rounded-2xl bg-gradient-to-tr from-blue-500 to-purple-600 flex items-center justify-center text-white text-2xl font-bold shadow-lg shadow-blue-500/20">
            SP
          </div>
          <h2 className="text-2xl font-bold text-white tracking-tight">{t('login.title')}</h2>
          <p className="text-xs text-gray-400 mt-1.5">{t('login.subtitle')}</p>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label htmlFor="username" className="block text-xs font-semibold text-gray-300 mb-1.5 uppercase">{t('login.username')}</label>
            <input
              id="username"
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              className="w-full px-3.5 py-2.5 rounded-lg bg-gray-800 border border-gray-700 text-white text-sm focus:outline-none focus:border-blue-500 transition-colors"
              required
            />
          </div>

          <div>
            <label htmlFor="password" className="block text-xs font-semibold text-gray-300 mb-1.5 uppercase">{t('login.password')}</label>
            <input
              id="password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="w-full px-3.5 py-2.5 rounded-lg bg-gray-800 border border-gray-700 text-white text-sm focus:outline-none focus:border-blue-500 transition-colors"
              required
            />
          </div>

          {error && (
            <div className="text-xs text-red-400 bg-red-950/30 border border-red-900/50 rounded-lg px-3 py-2">
              {error}
            </div>
          )}

          <div className="pt-2">
            <Button variant="primary" size="lg" className="w-full shadow-lg shadow-blue-600/30" loading={loading}>
              {t('login.submit')}
            </Button>
          </div>
        </form>

        <div className="mt-6 pt-4 border-t border-gray-800/80 text-center text-xs text-gray-500">
          SecretFlow Ecosystem • {t('login.defaultHint')}
        </div>
      </Card>
    </div>
  );
};
