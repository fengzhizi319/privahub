import React from 'react';
import { useNavigate } from '@tanstack/react-router';
import { Card, toast } from '@secretpad/design-system';
import { useTranslation } from '../../shared/lib/i18n';
import { useAuthStore } from '../../features/auth/model/auth-store';
import { ChangePasswordForm } from '../../features/auth/ui/change-password-form';

export const AccountPage: React.FC = () => {
  const { t } = useTranslation();
  const { user, platform, logout } = useAuthStore();
  const navigate = useNavigate();

  const handleSuccess = async () => {
    toast.success(t('account.success'));
    // Every session was revoked by the backend: log in again with the new password.
    try {
      await logout();
    } finally {
      navigate({ to: '/login' });
    }
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

        <ChangePasswordForm userName={user?.name} onSuccess={handleSuccess} />
      </Card>
    </div>
  );
};
