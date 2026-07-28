import React from 'react';

/**
 * SecretPad 设计系统基础组件库。
 *
 * 本文件集中提供跨页面复用的原子组件：Button / Badge / Card / Modal /
 * Toast / ConfirmDialog。所有组件均：
 * - 基于 Tailwind 工具类实现，并内置 `dark:` 暗色适配；
 * - 以受控（controlled）为主，状态由调用方持有；
 * - 通过 `className` 透传支持自定义扩展。
 */

/**
 * Button 按钮。
 * @property variant 视觉风格：主按钮 / 描边 / 危险 / 幽灵 / 链接。
 * @property size 尺寸：小 / 中 / 大。
 * @property icon 前置图标（loading 时会被加载动画替换）。
 * @property loading 加载中：展示转圈动画并禁用交互。
 */
export interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: 'primary' | 'outline' | 'danger' | 'ghost' | 'link';
  size?: 'sm' | 'md' | 'lg';
  icon?: React.ReactNode;
  loading?: boolean;
}

export const Button: React.FC<ButtonProps> = ({
  children,
  variant = 'outline',
  size = 'md',
  icon,
  loading,
  disabled,
  className = '',
  ...props
}) => {
  const baseStyle = 'inline-flex items-center justify-center font-medium rounded-lg transition-all focus:outline-none disabled:opacity-50 disabled:cursor-not-allowed';
  
  const sizeStyles = {
    sm: 'px-2.5 py-1 text-xs gap-1.5',
    md: 'px-3.5 py-1.5 text-sm gap-2',
    lg: 'px-4 py-2 text-base gap-2',
  };

  const variantStyles = {
    primary: 'bg-blue-600 hover:bg-blue-500 text-white shadow-sm active:bg-blue-700',
    outline: 'border border-gray-200 dark:border-gray-700 hover:border-blue-500 text-gray-800 dark:text-gray-200 bg-white dark:bg-gray-800 hover:text-blue-600 dark:hover:text-blue-400',
    danger: 'bg-red-600 hover:bg-red-500 text-white shadow-sm',
    ghost: 'hover:bg-gray-100 dark:hover:bg-gray-800 text-gray-700 dark:text-gray-300',
    link: 'text-blue-600 hover:underline p-0 shadow-none',
  };

  return (
    <button
      className={`${baseStyle} ${sizeStyles[size]} ${variantStyles[variant]} ${className}`}
      disabled={disabled || loading}
      {...props}
    >
      {loading ? (
        <span className="w-4 h-4 border-2 border-current border-t-transparent rounded-full animate-spin mr-1" />
      ) : (
        icon
      )}
      {children}
    </button>
  );
};

// Badge Component
/**
 * Badge 状态徽标。
 * @property status 状态语义，决定颜色与圆点动画（processing 带脉冲）。
 */
export interface BadgeProps {
  status?: 'success' | 'processing' | 'warning' | 'error' | 'default';
  children: React.ReactNode;
  className?: string;
}

export const Badge: React.FC<BadgeProps> = ({ status = 'default', children, className = '' }) => {
  const statusStyles = {
    success: 'bg-emerald-50 dark:bg-emerald-950/40 text-emerald-600 dark:text-emerald-400 border-emerald-200 dark:border-emerald-800',
    processing: 'bg-blue-50 dark:bg-blue-950/40 text-blue-600 dark:text-blue-400 border-blue-200 dark:border-blue-800',
    warning: 'bg-amber-50 dark:bg-amber-950/40 text-amber-600 dark:text-amber-400 border-amber-200 dark:border-amber-800',
    error: 'bg-rose-50 dark:bg-rose-950/40 text-rose-600 dark:text-rose-400 border-rose-200 dark:border-rose-800',
    default: 'bg-gray-100 dark:bg-gray-800 text-gray-600 dark:text-gray-400 border-gray-200 dark:border-gray-700',
  };

  const dotStyles = {
    success: 'bg-emerald-500',
    processing: 'bg-blue-500 animate-pulse',
    warning: 'bg-amber-500',
    error: 'bg-rose-500',
    default: 'bg-gray-400',
  };

  return (
    <span className={`inline-flex items-center gap-1.5 px-2.5 py-0.5 text-xs font-medium border rounded-full ${statusStyles[status]} ${className}`}>
      <span className={`w-1.5 h-1.5 rounded-full ${dotStyles[status]}`} />
      {children}
    </span>
  );
};

// Card Component
/**
 * Card 容器卡片。
 * @property title 标题栏内容（与 extra 任一存在时渲染标题栏）。
 * @property extra 标题栏右侧操作区。
 * @property bodyClassName 内容区自定义类名。
 */
export interface CardProps extends Omit<React.HTMLAttributes<HTMLDivElement>, 'title'> {
  title?: React.ReactNode;
  extra?: React.ReactNode;
  children: React.ReactNode;
  className?: string;
  bodyClassName?: string;
}

export const Card: React.FC<CardProps> = ({
  title,
  extra,
  children,
  className = '',
  bodyClassName = '',
  onClick,
  ...rest
}) => {
  return (
    <div
      onClick={onClick}
      className={`bg-white dark:bg-gray-900 border border-gray-200 dark:border-gray-800 rounded-xl shadow-xs overflow-hidden ${className}`}
      {...rest}
    >
      {(title || extra) && (
        <div className="px-5 py-3.5 border-b border-gray-100 dark:border-gray-800 flex items-center justify-between">
          <div className="font-semibold text-gray-900 dark:text-gray-100 text-sm">{title}</div>
          {extra && <div>{extra}</div>}
        </div>
      )}
      <div className={`p-5 ${bodyClassName}`}>{children}</div>
    </div>
  );
};

// Modal Component
/**
 * Modal 模态对话框（受控）。
 * @property isOpen 是否可见，由调用方控制。
 * @property onClose 关闭回调（点击遮罩右上角 ✕）。
 * @property footer 底部操作区，传入则渲染。
 * @property width 宽度类名（默认 max-w-lg）。
 */
export interface ModalProps {
  isOpen: boolean;
  onClose: () => void;
  title?: React.ReactNode;
  children: React.ReactNode;
  footer?: React.ReactNode;
  width?: string;
}

export const Modal: React.FC<ModalProps> = ({
  isOpen,
  onClose,
  title,
  children,
  footer,
  width = 'max-w-lg'
}) => {
  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-xs animate-fadeIn">
      <div className={`w-full ${width} bg-white dark:bg-gray-900 border border-gray-200 dark:border-gray-800 rounded-2xl shadow-xl overflow-hidden`}>
        {title && (
          <div className="px-6 py-4 border-b border-gray-100 dark:border-gray-800 flex items-center justify-between">
            <h3 className="font-semibold text-lg text-gray-900 dark:text-gray-100">{title}</h3>
            <button
              onClick={onClose}
              className="text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 transition-colors"
            >
              ✕
            </button>
          </div>
        )}
        <div className="p-6">{children}</div>
        {footer && (
          <div className="px-6 py-4 bg-gray-50 dark:bg-gray-850 border-t border-gray-100 dark:border-gray-800 flex justify-end gap-3">
            {footer}
          </div>
        )}
      </div>
    </div>
  );
};

// Toast Component (global imperative API)
export type ToastType = 'success' | 'error' | 'info' | 'warning';

export interface ToastItem {
  id: number;
  type: ToastType;
  message: React.ReactNode;
  duration: number;
}

type ToastListener = (item: ToastItem) => void;

let toastId = 0;
const toastListeners = new Set<ToastListener>();

function emitToast(type: ToastType, message: React.ReactNode, duration = 3000) {
  const item: ToastItem = { id: ++toastId, type, message, duration };
  toastListeners.forEach((listener) => listener(item));
}

/** Imperative global toast API. Requires a mounted <ToastContainer />. */
export const toast = {
  success: (message: React.ReactNode, duration?: number) => emitToast('success', message, duration),
  error: (message: React.ReactNode, duration?: number) => emitToast('error', message, duration),
  info: (message: React.ReactNode, duration?: number) => emitToast('info', message, duration),
  warning: (message: React.ReactNode, duration?: number) => emitToast('warning', message, duration),
};

const TOAST_TYPE_STYLES: Record<ToastType, string> = {
  success: 'border-emerald-200 dark:border-emerald-800 text-emerald-700 dark:text-emerald-300',
  error: 'border-rose-200 dark:border-rose-800 text-rose-700 dark:text-rose-300',
  info: 'border-blue-200 dark:border-blue-800 text-blue-700 dark:text-blue-300',
  warning: 'border-amber-200 dark:border-amber-800 text-amber-700 dark:text-amber-300',
};

const TOAST_TYPE_ICONS: Record<ToastType, string> = {
  success: '✓',
  error: '✕',
  info: 'ℹ',
  warning: '⚠',
};

/** Mount once near the app root to render toasts emitted via the `toast` API. */
export const ToastContainer: React.FC = () => {
  const [items, setItems] = React.useState<ToastItem[]>([]);

  React.useEffect(() => {
    const listener: ToastListener = (item) => {
      setItems((prev) => [...prev, item]);
      window.setTimeout(() => {
        setItems((prev) => prev.filter((t) => t.id !== item.id));
      }, item.duration);
    };
    toastListeners.add(listener);
    return () => {
      toastListeners.delete(listener);
    };
  }, []);

  if (items.length === 0) return null;

  return (
    <div className="fixed top-4 right-4 z-[100] flex flex-col gap-2 max-w-sm">
      {items.map((item) => (
        <div
          key={item.id}
          className={`flex items-start gap-2.5 px-4 py-3 rounded-lg border shadow-lg bg-white dark:bg-gray-900 text-sm animate-fadeIn ${TOAST_TYPE_STYLES[item.type]}`}
          role="alert"
        >
          <span className="font-bold leading-5">{TOAST_TYPE_ICONS[item.type]}</span>
          <span className="leading-5 break-all">{item.message}</span>
          <button
            onClick={() => setItems((prev) => prev.filter((t) => t.id !== item.id))}
            className="ml-auto text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 transition-colors leading-5"
            aria-label="close"
          >
            ✕
          </button>
        </div>
      ))}
    </div>
  );
};

// ConfirmDialog Component (based on Modal)
/**
 * ConfirmDialog 确认对话框（基于 Modal 封装）。
 * @property danger 危险操作：确认按钮使用 danger 风格。
 * @property loading 确认中：禁用取消并令确认按钮加载。
 */
export interface ConfirmDialogProps {
  isOpen: boolean;
  title?: React.ReactNode;
  message: React.ReactNode;
  confirmText?: React.ReactNode;
  cancelText?: React.ReactNode;
  danger?: boolean;
  loading?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

export const ConfirmDialog: React.FC<ConfirmDialogProps> = ({
  isOpen,
  title,
  message,
  confirmText = 'Confirm',
  cancelText = 'Cancel',
  danger = false,
  loading = false,
  onConfirm,
  onCancel,
}) => {
  return (
    <Modal
      isOpen={isOpen}
      onClose={onCancel}
      title={title}
      width="max-w-md"
      footer={
        <>
          <Button variant="outline" onClick={onCancel} disabled={loading}>
            {cancelText}
          </Button>
          <Button variant={danger ? 'danger' : 'primary'} onClick={onConfirm} loading={loading}>
            {confirmText}
          </Button>
        </>
      }
    >
      <div className="text-sm text-gray-600 dark:text-gray-300 leading-6">{message}</div>
    </Modal>
  );
};
