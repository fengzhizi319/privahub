/** i18n extension dictionary: authentication / forced password change. */
import type { Dictionary } from '../dictionaries';

export const zh: Dictionary = {
  account: {
    force: {
      title: '请先修改密码',
      subtitle: '当前密码为初始密码或已被管理员重置，修改后方可继续使用平台。',
      logout: '退出登录',
      relogin: '密码已修改，请使用新密码重新登录。',
    },
  },
};

export const en: Dictionary = {
  account: {
    force: {
      title: 'Change your password',
      subtitle: 'Your password is an initial or administrator-reset password. Change it to continue using the platform.',
      logout: 'Log out',
      relogin: 'Password changed. Please log in again with the new password.',
    },
  },
};
