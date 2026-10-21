import { defineConfig, loadEnv, type Plugin } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'path';

// 开发期可通过 API_PROXY_URL 覆盖后端代理地址，例如：
// API_PROXY_URL=http://127.0.0.1:18083 pnpm dev
const apiProxyTarget = process.env.API_PROXY_URL || 'http://127.0.0.1:8080';

/**
 * 从构建期环境变量中收集需要放开 `connect-src` 的跨源上报地址（origin）。
 *
 * 目前覆盖两类“显式开启”的可观测性出口（均默认关闭、不产生网络请求）：
 * - `VITE_SENTRY_DSN`：Sentry 错误/性能上报（见 src/shared/lib/observability/sentry.ts）；
 * - `VITE_WEB_VITALS_ENDPOINT`：Web Vitals sendBeacon 上报端点
 *   （见 src/shared/lib/observability/web-vitals.ts，同样要求放开 connect-src）。
 *
 * 仅追加 origin（scheme + host + port），不追加完整 URL，避免把 query 中的
 * 采样参数等带入 CSP；非法 URL 被静默忽略，保持策略为保守默认值。
 *
 * @param env loadEnv 得到的构建期环境变量集合
 * @returns 去重后的 origin 列表；未配置任何出口时为空数组
 */
function extraConnectSrcOrigins(env: Record<string, string>): string[] {
  const origins = new Set<string>();
  for (const key of ['VITE_SENTRY_DSN', 'VITE_WEB_VITALS_ENDPOINT']) {
    const raw = env[key]?.trim();
    if (!raw) continue;
    try {
      origins.add(new URL(raw).origin);
    } catch {
      // 非法地址不进入 CSP，保持默认的同源限制。
    }
  }
  return [...origins];
}

/**
 * 生产环境内容安全策略（CSP）注入插件。
 *
 * 设计要点：
 * 1. `apply: 'build'` —— 仅在 `vite build` 生产构建时注入，开发服务器
 *    （依赖 @vitejs/plugin-react 注入的内联 react-refresh 引导脚本）不受影响。
 * 2. 生产构建下所有 JS 均打包为外部 hash chunk，配合下方
 *    `build.modulePreload.polyfill: false` 关闭内联 polyfill，
 *    因此可使用严格的 `script-src 'self'`（不允许内联脚本/eval）。
 * 3. `style-src` 保留 'unsafe-inline'：Tailwind 产物为外部 CSS，但部分
 *    组件/第三方库会动态插入 <style>，CSS 维度的内联风险极低，放行以兼容。
 * 4. `frame-ancestors`、`report-uri` 等指令在 <meta> 中被浏览器忽略，
 *    需由后端/反向代理以 HTTP 响应头形式下发（见注释说明）。
 * 5. `connect-src` 默认仅同源；构建期若配置了外部可观测性出口
 *    （Sentry DSN / Web Vitals 端点），则按 origin 追加放行，
 *    否则外部上报会被 CSP 静默拦截。
 *
 * @param extraConnectOrigins 需要追加到 `connect-src` 的跨源 origin 列表
 */
const contentSecurityPolicyPlugin = (extraConnectOrigins: string[]): Plugin => {
  // 以数组维护各指令，便于阅读与后续扩展；最终以 '; ' 拼接为完整策略串。
  const directives = [
    "default-src 'self'", // 默认仅允许同源资源，作为兜底白名单
    "script-src 'self'", // 仅同源外部脚本文件，禁止内联脚本与 eval（防 XSS）
    "style-src 'self' 'unsafe-inline'", // 外部样式 + 必要的动态内联样式
    "img-src 'self' data: blob:", // 图片允许同源、data URI 与 blob URL
    "font-src 'self' data:", // 字体允许同源与 data URI
    // XHR/fetch/sendBeacon 仅限同源（API 经同源代理），生产无需 WebSocket；
    // 配置了外部可观测性出口时按 origin 追加放行。
    ["connect-src 'self'", ...extraConnectOrigins].join(' '),
    "object-src 'none'", // 全面禁用 <object>/<embed> 等插件载体
    "base-uri 'self'", // 防止通过注入 <base> 标签劫持相对路径
    "form-action 'self'", // 限制表单仅可提交到同源
  ];
  const csp = directives.join('; ');

  return {
    name: 'secretpad-csp',
    apply: 'build',
    // 在 index.html 的 <head> 起始处插入 CSP meta 标签。
    transformIndexHtml(html) {
      return html.replace(
        '<head>',
        `<head>\n    <meta http-equiv="Content-Security-Policy" content="${csp}" />`
      );
    },
  };
};

export default defineConfig(({ mode }) => {
  // 读取构建期 VITE_* 环境变量（含 .env 文件），用于 CSP connect-src 放行决策。
  const env = loadEnv(mode, process.cwd(), '');
  return {
    plugins: [react(), contentSecurityPolicyPlugin(extraConnectSrcOrigins(env))],
    resolve: {
      alias: {
        '@': path.resolve(__dirname, './src'),
      },
    },
    server: {
      host: '127.0.0.1',
      port: 8000,
      proxy: {
        '/api': {
          target: apiProxyTarget,
          changeOrigin: true,
        },
      },
    },
    build: {
      // 关闭内联 module preload polyfill：现代浏览器原生支持 modulepreload，
      // 且内联 polyfill 会被严格 CSP 的 script-src 'self' 拦截。
      modulePreload: { polyfill: false },
      // 主包拆分：将体积较大的第三方依赖抽离为独立 chunk，
      // 利用浏览器长期缓存并消除 >500kB 告警。
      rollupOptions: {
        output: {
          manualChunks: (id) => {
            if (!id.includes('node_modules')) return undefined;
            // react 与 @tanstack 存在双向模块引用，合并为同一 framework chunk 以避免循环 chunk 告警。
            if (
              id.includes('react-dom') ||
              id.includes('/react/') ||
              id.includes('scheduler') ||
              id.includes('@tanstack')
            ) {
              return 'framework';
            }
            if (id.includes('/zod/')) {
              return 'zod-vendor';
            }
            // 其余依赖（zustand 等）与 react 存在双向引用，一并归入 framework 避免循环 chunk。
            return 'framework';
          },
        },
      },
    },
  };
});
