import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { expect, it } from 'vitest';
import type { Project } from '@secretpad/api-client';
import { I18nProvider } from '../../shared/lib/i18n';
import { ProjectGroup } from './project-group';

it('folds and pages projects, and keeps remaining projects visible after deletion',()=>{
 localStorage.setItem('secretpad-locale','zh-CN');
 const projects=Array.from({length:7},(_,i)=>({projectId:`p${i}`,projectName:`Project ${i}`})) as Project[];
 const child=(p:Project)=><div key={p.projectId}>{p.projectName}</div>;
 const {rerender}=render(<I18nProvider><ProjectGroup title="MPC" projects={projects}>{child}</ProjectGroup></I18nProvider>);
 expect(screen.queryByText('Project 6')).toBeNull();
 fireEvent.click(screen.getByRole('button',{name:'下一页'}));
 expect(screen.getByText('Project 6')).toBeTruthy();
 expect(screen.queryByText('Project 0')).toBeNull();
 fireEvent.click(screen.getByRole('button',{name:/MPC/}));
 expect(screen.getByRole('button',{name:/MPC/}).getAttribute('aria-expanded')).toBe('false');
 fireEvent.click(screen.getByRole('button',{name:/MPC/}));
 rerender(<I18nProvider><ProjectGroup title="MPC" projects={projects.slice(0,6)}>{child}</ProjectGroup></I18nProvider>);
 expect(screen.getByText('Project 0')).toBeTruthy();
});

it.each(['list','all'] as const)('shows every matching project without pagination in %s view',view=>{
 const projects=Array.from({length:27},(_,i)=>({projectId:`p${i}`,projectName:`Project ${i}`})) as Project[];
 render(<I18nProvider><ProjectGroup title="MPC" view={view} projects={projects}>{p=><div key={p.projectId}>{p.projectName}</div>}</ProjectGroup></I18nProvider>);
 expect(screen.getByText('Project 0')).toBeTruthy();
 expect(screen.getByText('Project 26')).toBeTruthy();
 expect(screen.queryByRole('button',{name:'下一页'})).toBeNull();
});
