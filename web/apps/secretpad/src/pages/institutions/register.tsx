import React, { useMemo, useState } from 'react';
import { useMutation } from '@tanstack/react-query';
import { Button, Card, FormField, Input, RadioGroup, Select, Textarea, toast } from '@secretpad/design-system';
import { registerInstNodeJava } from '@secretpad/api-client';
import { useTranslation } from '@/shared/lib/i18n';
import { errorText } from '@/features/cooperative-node/format';
import { buildRegisterJson, validateRegisterJson, type InstRegisterFields } from './register-json';

const FileInput: React.FC<{ file: File | null; onChange: (f: File | null) => void; accept?: string }> = ({ file, onChange, accept }) => (
  <div className="flex items-center gap-2 text-xs">
    <input
      type="file"
      accept={accept}
      className="text-xs file:mr-2 file:px-2.5 file:py-1 file:rounded-lg file:border file:border-gray-200 dark:file:border-gray-700 file:bg-white dark:file:bg-gray-800"
      onChange={(e) => onChange(e.target.files?.[0] || null)}
    />
    {file && <span className="text-gray-400">{Math.ceil(file.size / 1024)} KB</span>}
  </div>
);

/**
 * Institution node registration — multipart POST inst/node/register
 * (Java InstController.registerNode: json_data + certFile + keyFile + token).
 * Equivalent of deploy/common/utils.sh `post_kuscia_node`.
 */
export const InstRegisterPage: React.FC = () => {
  const { t } = useTranslation();
  const [mode, setMode] = useState<'form' | 'json'>('form');
  const [fields, setFields] = useState<InstRegisterFields>({
    domainId: '',
    nodeName: '',
    token: '',
    mode: 'p2p',
    port: '8083',
    protocol: 'notls',
    transPort: '1080',
    netAddress: '',
  });
  const [raw, setRaw] = useState('');
  const [certFile, setCertFile] = useState<File | null>(null);
  const [keyFile, setKeyFile] = useState<File | null>(null);
  const [tokenFile, setTokenFile] = useState<File | null>(null);
  const [touched, setTouched] = useState(false);
  const set = (k: keyof InstRegisterFields, v: string) => setFields((f) => ({ ...f, [k]: v }));

  const jsonData = useMemo(() => (mode === 'form' ? buildRegisterJson(fields) : raw), [mode, fields, raw]);
  const jsonErr = mode === 'json' ? validateRegisterJson(raw) : fields.domainId.trim() ? null : 'domainId';
  const errors = {
    json: jsonErr ? t(`institutions.register.jsonError.${jsonErr}`) : undefined,
    cert: certFile ? undefined : t('coop.required'),
    key: keyFile ? undefined : t('coop.required'),
  };
  const valid = !errors.json && !errors.cert && !errors.key;

  const mutation = useMutation({
    mutationFn: () => registerInstNodeJava(jsonData, { certFile, keyFile, token: tokenFile }),
    onSuccess: () => toast.success(t('institutions.register.success')),
    onError: (e) => toast.error(errorText(e)),
  });

  return (
    <div className="space-y-4 max-w-3xl">
      <div className="bg-white dark:bg-gray-900 p-4 rounded-xl border border-gray-200 dark:border-gray-800">
        <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('institutions.register.title')}</h2>
        <p className="text-xs text-gray-500">{t('institutions.register.subtitle')}</p>
      </div>
      <Card>
        <div className="space-y-4">
          <RadioGroup
            value={mode}
            onChange={(v) => {
              if (v === 'json' && !raw) setRaw(JSON.stringify(JSON.parse(buildRegisterJson(fields)), null, 2));
              setMode(v as 'form' | 'json');
            }}
            options={[
              { value: 'form', label: t('institutions.register.modeForm') },
              { value: 'json', label: t('institutions.register.modeJson') },
            ]}
          />
          {mode === 'form' ? (
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <FormField label={t('institutions.register.domainId')} required error={touched ? errors.json : undefined}>
                <Input value={fields.domainId} onChange={(e) => set('domainId', e.target.value)} />
              </FormField>
              <FormField label={t('nodes.nodeNameLabel')}>
                <Input value={fields.nodeName} onChange={(e) => set('nodeName', e.target.value)} />
              </FormField>
              <FormField label={t('institutions.register.kusciaToken')} help={t('institutions.register.kusciaTokenHelp')}>
                <Input value={fields.token} onChange={(e) => set('token', e.target.value)} />
              </FormField>
              <FormField label={t('institutions.register.mode')}>
                <Select
                  value={fields.mode}
                  onChange={(v) => set('mode', v)}
                  options={['p2p', 'lite', 'master', 'autonomy'].map((m) => ({ value: m, label: m }))}
                />
              </FormField>
              <FormField label={t('institutions.register.port')}>
                <Input value={fields.port} onChange={(e) => set('port', e.target.value)} />
              </FormField>
              <FormField label={t('institutions.register.transPort')}>
                <Input value={fields.transPort} onChange={(e) => set('transPort', e.target.value)} />
              </FormField>
              <FormField label={t('nodes.protocol')}>
                <Select
                  value={fields.protocol}
                  onChange={(v) => set('protocol', v)}
                  options={['notls', 'tls', 'mtls'].map((m) => ({ value: m, label: m }))}
                />
              </FormField>
              <FormField label={t('nodes.netAddress')}>
                <Input value={fields.netAddress} onChange={(e) => set('netAddress', e.target.value)} placeholder="http://host:port" />
              </FormField>
            </div>
          ) : (
            <FormField label="json_data" required error={touched ? errors.json : undefined}>
              <Textarea rows={8} className="font-mono text-xs" value={raw} onChange={(e) => setRaw(e.target.value)} />
            </FormField>
          )}
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            <FormField label={t('institutions.register.certFile')} required error={touched ? errors.cert : undefined} help="client.crt">
              <FileInput file={certFile} onChange={setCertFile} accept=".crt,.pem,.cer" />
            </FormField>
            <FormField label={t('institutions.register.keyFile')} required error={touched ? errors.key : undefined} help="client.pem">
              <FileInput file={keyFile} onChange={setKeyFile} accept=".pem,.key" />
            </FormField>
            <FormField label={t('institutions.register.tokenFile')} help={t('institutions.register.tokenFileHelp')}>
              <FileInput file={tokenFile} onChange={setTokenFile} />
            </FormField>
          </div>
          {mode === 'form' && (
            <pre className="p-2 rounded bg-gray-50 dark:bg-gray-800 text-[10px] font-mono break-all whitespace-pre-wrap">json_data = {jsonData}</pre>
          )}
          <div className="flex justify-end">
            <Button
              variant="primary"
              loading={mutation.isPending}
              onClick={() => {
                setTouched(true);
                if (valid) mutation.mutate();
              }}
            >
              {t('institutions.register.submit')}
            </Button>
          </div>
        </div>
      </Card>
    </div>
  );
};
