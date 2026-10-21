import React, { useState } from 'react';
import { describe, expect, it } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { I18nProvider } from '@/shared/lib/i18n';
import { SchemaEditor } from './schema-editor';
import type { SchemaField } from './model';

const Harness: React.FC<{ initial: SchemaField[] }> = ({ initial }) => {
  const [v, setV] = useState(initial);
  return <SchemaEditor value={v} onChange={setV} showErrors />;
};

describe('SchemaEditor', () => {
  it('shows duplicate / pattern errors and SCQL warnings, and adds rows', () => {
    render(
      <I18nProvider>
        <Harness
          initial={[
            { featureName: 'id', featureType: 'str', featureDescription: '' },
            { featureName: 'id', featureType: 'int', featureDescription: '' },
            { featureName: 'select', featureType: 'int', featureDescription: '' },
          ]}
        />
      </I18nProvider>,
    );
    expect(screen.getByRole('alert')).toBeTruthy();
    const inputs = screen.getAllByRole('textbox');
    const before = inputs.length;
    fireEvent.change(inputs[2], { target: { value: 'id2' } });
    expect(screen.queryByRole('alert')).toBeNull();
    const addBtn = screen.getAllByRole('button').find((b) => b.textContent?.includes('＋'));
    fireEvent.click(addBtn!);
    expect(screen.getAllByRole('textbox').length).toBe(before + 2);
  });
});
