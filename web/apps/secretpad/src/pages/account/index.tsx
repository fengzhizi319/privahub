import React, { useState } from 'react';
import { useMutation } from '@tanstack/react-query';
import { Card, Button, toast } from '@secretpad/design-system';
import { apiClient } from '@secretpad/api-client';
import { sha256 } from '@secretpad/utils';
import { useTranslation } from '../../shared/lib/i18n';
import { useAuthStore } from '../../features/auth/model/auth-store';

export const AccountPage: React.FC = () => {
  const { t } = useTranslation();
  const { user, platform } = useAuthStore();

  const [error, setError] = useState<string | null>(null);
  const [oldPassword, setOldPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');

  const mutation = useMutation({
    mutationFn: async () => {
      const oldPasswordHash = await sha256(oldPassword);
      const newPasswordHash = await sha256(newPassword);
      const confirmPasswordHash = await sha256(confirmPassword);
      return apiClient.updatePassword({
        name: user?.name,
        oldPasswordHash,
        newPasswordHash,
        confirmPasswordHash,
      });
    },
    onSuccess: () => {
      setOldPassword('');
      setNewPassword('');
      setConfirmPassword('');
      toast.success(t('account.success'));
    },
    onError: (e) => setError(e instanceof Error ? e.message : String(e)),
  });

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    if (newPassword !== confirmPassword) {
      setError(t('account.mismatch'));
      return;
    }
    mutation.mutate();
  };

  const infoRow = (label: string, value?: string) => (
    <div>
      <div className="text-gray-400 mb-1">{label}</div>
      <div className="font-semibold text-gray-800 dark:text-gray-200 font-mono">{value || '-'}</div>
    </div>
  );

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between bg-white dark:bg-gray-900 p-4 rounded-xl border border-gray-200 dark:border-gray-800">
        <div>
          <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('account.title')}</h2>
          <p className="text-xs text-gray-500">{t('account.subtitle')}</p>
        </div>
      </div>

      {/* Account Info */}
      <Card>
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 text-xs">
          {infoRow(t('account.username'), user?.name)}
          {infoRow(t('account.platformType'), user?.platformType || platform.platformType)}
          {infoRow(t('account.nodeId'), user?.platformNodeId || platform.nodeId)}
          {infoRow(t('account.ownerType'), user?.ownerType)}
        </div>
      </Card>

      {/* Change Password */}
      <Card>
        <div className="text-sm font-semibold text-gray-700 dark:text-gray-200 mb-4">{t('account.changePassword')}</div>

        {error && (
          <div className="text-xs text-red-500 bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900 rounded-lg px-4 py-2 mb-4">
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit} className="space-y-4 text-xs max-w-md">
          <div>
            <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('account.oldPassword')}</label>
            <input
              type="password"
              value={oldPassword}
              onChange={(e) => setOldPassword(e.target.value)}
              className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
              required
            />
          </div>
          <div>
            <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('account.newPassword')}</label>
            <input
              type="password"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
              required
            />
          </div>
          <div>
            <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('account.confirmPassword')}</label>
            <input
              type="password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
              required
            />
          </div>
          <Button variant="primary" onClick={handleSubmit} loading={mutation.isPending}>{t('account.submit')}</Button>
        </form>
      </Card>
    </div>
  );
};
