import React from 'react';
import { useNavigate } from '@tanstack/react-router';
import { Button, Card, toast } from '@secretpad/design-system';
import { useTranslation } from '../../shared/lib/i18n';
import { useAuthStore } from '../../features/auth/model/auth-store';
import { ChangePasswordForm } from '../../features/auth/ui/change-password-form';

/**
 * Forced password change (mustChangePassword): shown outside the app layout,
 * because the backend restricts such a session to user/get, user/updatePwd and
 * logout until the initial / administrator-reset password is replaced.
 */
export const ChangePasswordPage: React.FC = () => {
  const { t } = useTranslation();
  const { user, logout, clearMustChangePassword } = useAuthStore();
  const navigate = useNavigate();

  const toLogin = async () => {
    try {
      await logout();
    } finally {
      navigate({ to: '/login' });
    }
  };

  const handleSuccess = async () => {
    clearMustChangePassword();
    toast.success(t('account.force.relogin'));
    // The backend revoked every session of the user: log in again.
    await toLogin();
  };

  return (
    <div className="min-h-screen w-full flex items-center justify-center bg-gradient-to-br from-gray-950 via-slate-900 to-blue-950 p-4">
      <Card className="w-full max-w-md">
        <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('account.force.title')}</h2>
        <p className="text-xs text-gray-500 mt-1 mb-4">{t('account.force.subtitle')}</p>
        <ChangePasswordForm userName={user?.name} onSuccess={handleSuccess} />
        <div className="mt-4 text-right">
          <Button variant="outline" size="sm" onClick={toLogin}>
            {t('account.force.logout')}
          </Button>
        </div>
      </Card>
    </div>
  );
};
