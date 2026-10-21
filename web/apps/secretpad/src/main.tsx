import React from 'react';
import ReactDOM from 'react-dom/client';
import { App } from './app/App';
import { I18nProvider } from './shared/lib/i18n';
import { initSentry, reportWebVitals } from './shared/lib/observability';
import { initTheme } from './shared/lib/theme';
import './styles/index.css';

// 主题初始化：必须在 React 渲染之前同步执行，
// 以便首屏即应用持久化/系统偏好主题，避免「先亮后暗」的闪烁（FOUC）。
initTheme();

// 可观测性初始化：
// 1. Sentry 必须在 React 渲染之前初始化，才能捕获首屏渲染异常；
//    未配置 DSN 时自动降级为 no-op，不影响本地开发与私有化部署。
// 2. Web Vitals 采集器注册后会在指标最终确定时自动回调上报。
initSentry();
reportWebVitals();

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <I18nProvider>
      <App />
    </I18nProvider>
  </React.StrictMode>
);
