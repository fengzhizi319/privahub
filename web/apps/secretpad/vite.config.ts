import { defineConfig, type Plugin } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'path';

// 开发期可通过 API_PROXY_URL 覆盖后端代理地址，例如：
// API_PROXY_URL=http://127.0.0.1:18083 pnpm dev
const apiProxyTarget = process.env.API_PROXY_URL || 'http://127.0.0.1:8080';

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
 */
const contentSecurityPolicyPlugin = (): Plugin => {
  // 以数组维护各指令，便于阅读与后续扩展；最终以 '; ' 拼接为完整策略串。
  const directives = [
    "default-src 'self'", // 默认仅允许同源资源，作为兜底白名单
    "script-src 'self'", // 仅同源外部脚本文件，禁止内联脚本与 eval（防 XSS）
    "style-src 'self' 'unsafe-inline'", // 外部样式 + 必要的动态内联样式
    "img-src 'self' data: blob:", // 图片允许同源、data URI 与 blob URL
    "font-src 'self' data:", // 字体允许同源与 data URI
    "connect-src 'self'", // XHR/fetch 仅限同源（API 经同源代理），生产无需 WebSocket
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

export default defineConfig({
  plugins: [react(), contentSecurityPolicyPlugin()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
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
});
