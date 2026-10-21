import React from 'react';
import { beforeEach, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { I18nProvider } from '../../shared/lib/i18n';
import { PredefinedCanvasesPage } from './index';

vi.mock('@tanstack/react-router',()=>({Link:({to,children}: {to:string;children:React.ReactNode})=><a href={to}>{children}</a>}));
vi.mock('../../shared/lib/platform',()=>({usePlatform:()=>({isCenter:true,isCenterAdmin:true,isAutonomy:false,supportsMpc:true,supportsTee:false})}));
vi.mock('../../features/create-project',()=>({CreateProjectWizard:({isOpen,preset}: {isOpen:boolean;preset?:{templateKey:string}})=>isOpen?<div role="dialog">{preset?.templateKey}</div>:null}));
beforeEach(()=>localStorage.setItem('secretpad-locale','zh-CN'));

it('replaces the entire preview when selecting another predefined canvas',()=>{
 const {container}=render(<I18nProvider><PredefinedCanvasesPage /></I18nProvider>);
 expect(container.querySelectorAll('[data-template-node]')).toHaveLength(7);
 fireEvent.click(screen.getByRole('button',{name:/KMeans 聚类/}));
 const ids=[...container.querySelectorAll('[data-template-node]')].map(n=>n.getAttribute('data-template-node'));
 expect(ids).toHaveLength(6);
 expect(ids.every(id=>id?.startsWith('preview-kmeans-'))).toBe(true);
 expect(screen.queryByRole('img',{name:/逻辑回归/})).toBeNull();
 fireEvent.click(screen.getByRole('button',{name:'使用此画布'}));
 expect(screen.getByRole('dialog').textContent).toBe('kmeans');
});

it('searches templates without editing a saved graph',()=>{
 render(<I18nProvider><PredefinedCanvasesPage /></I18nProvider>);
 fireEvent.change(screen.getByRole('textbox',{name:'搜索算法或模板'}),{target:{value:'SecureBoost'}});
 expect(screen.getAllByRole('button',{name:/SecureBoost/})).toHaveLength(1);
 expect(screen.queryByRole('button',{name:/KMeans/})).toBeNull();
 expect(screen.getByRole('link',{name:/打开自定义画布/}).getAttribute('href')).toBe('/dag');
});

it('groups templates and folds a category without losing the selected preview',()=>{
 const {container}=render(<I18nProvider><PredefinedCanvasesPage /></I18nProvider>);
 const ml=screen.getByRole('button',{name:/机器学习/});
 expect(ml.getAttribute('aria-expanded')).toBe('true');
 expect(screen.getByRole('button',{name:/隐私组件/}).getAttribute('aria-expanded')).toBe('false');
 fireEvent.click(ml);
 expect(screen.queryByRole('button',{name:/KMeans/})).toBeNull();
 expect(container.querySelectorAll('[data-template-node]')).toHaveLength(7);
 fireEvent.click(ml);
 expect(screen.getByRole('button',{name:/KMeans/})).toBeTruthy();
});

it('search reveals matching collapsed categories and restores folding on clear',()=>{
 render(<I18nProvider><PredefinedCanvasesPage /></I18nProvider>);
 const group=screen.getByRole('button',{name:/机器学习/});
 fireEvent.click(group);
 const search=screen.getByRole('textbox',{name:'搜索算法或模板'});
 fireEvent.change(search,{target:{value:'KMeans'}});
 expect(screen.getByRole('button',{name:/KMeans/})).toBeTruthy();
 expect(group.getAttribute('aria-expanded')).toBe('true');
 expect(screen.queryByRole('button',{name:/隐私组件/})).toBeNull();
 fireEvent.change(search,{target:{value:''}});
 expect(group.getAttribute('aria-expanded')).toBe('false');
 expect(screen.queryByRole('button',{name:/KMeans/})).toBeNull();
});
