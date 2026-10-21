import React, { useEffect, useState } from 'react';
import { useMutation } from '@tanstack/react-query';
import { Button, FormField, Input, Modal, toast } from '@secretpad/design-system';
import { resetRemoteUserPwdJava } from '@secretpad/api-client';
import { useTranslation } from '@/shared/lib/i18n';
import {
  buildResetPwdRequest,
  hasEdgePasswordErrors,
  validateEdgePassword,
  type EdgePasswordError,
  type EdgePasswordInput,
} from './password-validation';

export interface EdgeAccountPasswordDialogProps {
  open: boolean;
  onClose: () => void;
  /** Node whose edge account is changed. */
  nodeId: string;
  /** Account name (legacy: equals the node id). */
  accountName?: string;
  /**
   * @deprecated ignored. Java's frontend always calls user/remote/resetPassword
   * (user/node/resetPassword is inner-port only); the backend proxies it to the
   * center, or serves it locally when this platform is the center.
   */
  remote?: boolean;
}

const EMPTY: EdgePasswordInput = { oldPassword: '', newPassword: '', confirmPassword: '' };

/** Legacy my-node "中心平台账号 · 设置密码" dialog. */
export const EdgeAccountPasswordDialog: React.FC<EdgeAccountPasswordDialogProps> = ({
  open,
  onClose,
  nodeId,
  accountName,
}) => {
  const { t } = useTranslation();
  const [values, setValues] = useState<EdgePasswordInput>(EMPTY);
  const [touched, setTouched] = useState(false);
  const name = accountName || nodeId;

  useEffect(() => {
    if (open) {
      setValues(EMPTY);
      setTouched(false);
    }
  }, [open]);

  const errors = validateEdgePassword(values);
  const msg = (e?: EdgePasswordError) => (touched && e ? t(`edgeAccount.error.${e}`) : undefined);

  const mutation = useMutation({
    mutationFn: async () => {
      const body = await buildResetPwdRequest(nodeId, name, values.oldPassword, values.newPassword);
      return resetRemoteUserPwdJava(body);
    },
    onSuccess: () => {
      toast.success(t('edgeAccount.success'));
      onClose();
    },
    onError: (e) => {
      const m = e instanceof Error ? e.message : String(e);
      toast.error(/202011100/.test(m) ? t('edgeAccount.wrongOldPassword') : m);
    },
  });

  const submit = () => {
    setTouched(true);
    if (!hasEdgePasswordErrors(errors)) mutation.mutate();
  };

  const field = (key: keyof EdgePasswordInput, label: string) => (
    <FormField label={label} required error={msg(errors[key])}>
      <Input
        type="password"
        autoComplete="new-password"
        value={values[key]}
        placeholder={t('edgeAccount.placeholder')}
        onChange={(e) => setValues((v) => ({ ...v, [key]: e.target.value }))}
      />
    </FormField>
  );

  return (
    <Modal
      isOpen={open}
      onClose={onClose}
      title={t('edgeAccount.title')}
      footer={
        <>
          <Button variant="outline" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button variant="primary" loading={mutation.isPending} onClick={submit}>
            {t('common.confirm')}
          </Button>
        </>
      }
    >
      <div className="space-y-3">
        <div className="text-xs text-gray-600 dark:text-gray-300">
          {t('edgeAccount.accountName')}: <span className="font-mono">{name}</span>
        </div>
        {field('oldPassword', t('edgeAccount.oldPassword'))}
        {field('newPassword', t('edgeAccount.newPassword'))}
        {field('confirmPassword', t('edgeAccount.confirmPassword'))}
        <div className="text-[11px] text-gray-400">{t('edgeAccount.policy')}</div>
      </div>
    </Modal>
  );
};
