import React from 'react';
import { describe, it, expect, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { App } from './App';
import { I18nProvider } from '../shared/lib/i18n';

const mockUser = {
  ownerId: 'kuscia-system',
  name: 'admin',
  token: 'test-token',
  platformType: 'CENTER',
  platformNodeId: 'kuscia-system',
  ownerType: 'CENTER',
};

const renderWithI18n = (ui: React.ReactNode) => render(<I18nProvider>{ui}</I18nProvider>);

describe('App', () => {
  beforeEach(() => {
    localStorage.clear();
    localStorage.setItem('secretpad-token', 'test-token');
    localStorage.setItem('secretpad-user', JSON.stringify(mockUser));
  });

  it('renders the dashboard by default', async () => {
    renderWithI18n(<App />);
    expect(await screen.findByText('Console Dashboard')).toBeTruthy();
  });

  it('shows header and sidebar navigation', async () => {
    renderWithI18n(<App />);
    expect(await screen.findByText('SecretPad')).toBeTruthy();
    expect(await screen.findByText('Dashboard')).toBeTruthy();
  });
});
