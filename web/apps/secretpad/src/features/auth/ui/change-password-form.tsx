import React, { useState } from 'react';
import { useMutation } from '@tanstack/react-query';
import { Button } from '@secretpad/design-system';
import { updateUserPassword } from '@secretpad/api-client';
import { sha256, sm3, validatePasswordChange } from '@secretpad/utils';
import { useTranslation } from '../../../shared/lib/i18n';

const inputClass =
  'w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500';

/**
 * Self-service password change (user/updatePwd). The plaintext never leaves
 * the browser: SHA-256 (plus SM3 twins for legacy Java accounts) is sent and
 * the backend stores an SM3-KDF of it. Length/character rules are enforced
 * here; the backend additionally rejects weak-dictionary and reused passwords.
 * On success every session of the user is revoked server side.
 */
export const ChangePasswordForm: React.FC<{ userName?: string; onSuccess: () => void | Promise<void> }> = ({
  userName,
  onSuccess,
}) => {
  const { t } = useTranslation();
  const [error, setError] = useState<string | null>(null);
  const [oldPassword, setOldPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');

  const mutation = useMutation({
    mutationFn: async () => {
      const [oldPasswordHash, newPasswordHash, confirmPasswordHash] = await Promise.all([
        sha256(oldPassword),
        sha256(newPassword),
        sha256(confirmPassword),
      ]);
      return updateUserPassword({
        name: userName,
        oldPasswordHash,
        newPasswordHash,
        confirmPasswordHash,
        // SM3 twins let the backend verify accounts created by the legacy Java platform.
        oldPasswordHashSm3: sm3(oldPassword),
        newPasswordHashSm3: sm3(newPassword),
        confirmPasswordHashSm3: sm3(confirmPassword),
      });
    },
    onSuccess: async () => {
      setOldPassword('');
      setNewPassword('');
      setConfirmPassword('');
      await onSuccess();
    },
    onError: (e) => setError(e instanceof Error ? e.message : String(e)),
  });

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    const issue = validatePasswordChange({ oldPassword, newPassword, confirmPassword });
    if (issue) {
      setError(issue === 'mismatch' ? t('account.mismatch') : t(`account.policy.${issue}`));
      return;
    }
    mutation.mutate();
  };

  return (
    <>
      {error && (
        <div
          role="alert"
          className="text-xs text-red-500 bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900 rounded-lg px-4 py-2 mb-4"
        >
          {error}
        </div>
      )}
      <form onSubmit={handleSubmit} className="space-y-4 text-xs max-w-md">
        <div className="text-[11px] text-gray-500">{t('account.policy.hint')}</div>
        <div>
          <label htmlFor="acc-old" className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">
            {t('account.oldPassword')}
          </label>
          <input
            type="password"
            value={oldPassword}
            id="acc-old"
            autoComplete="current-password"
            onChange={(e) => setOldPassword(e.target.value)}
            className={inputClass}
            required
          />
        </div>
        <div>
          <label htmlFor="acc-new" className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">
            {t('account.newPassword')}
          </label>
          <input
            type="password"
            value={newPassword}
            id="acc-new"
            autoComplete="new-password"
            onChange={(e) => setNewPassword(e.target.value)}
            minLength={8}
            maxLength={20}
            className={inputClass}
            required
          />
        </div>
        <div>
          <label htmlFor="acc-confirm" className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">
            {t('account.confirmPassword')}
          </label>
          <input
            type="password"
            value={confirmPassword}
            id="acc-confirm"
            autoComplete="new-password"
            onChange={(e) => setConfirmPassword(e.target.value)}
            className={inputClass}
            required
          />
        </div>
        <Button variant="primary" type="submit" loading={mutation.isPending}>
          {t('account.submit')}
        </Button>
      </form>
    </>
  );
};
