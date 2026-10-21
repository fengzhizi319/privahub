/** i18n extension dictionary: platform (auth, guards, menus, header, account, node view). */
import type { Dictionary } from '../dictionaries';

export const zh: Dictionary = {
  display: {
    resultContent: '结果内容', filters: '筛选条件', clearFilters: '清空筛选', density: '行高', compact: '紧凑', comfortable: '舒适', columns: '列设置', width: '列宽', reset: '恢复默认', pageSort: '点击表头排序当前页', more: '更多',
    search: '搜索', statusFilter: '任务状态', allStatus: '全部状态', startTime: '开始时间', endTime: '结束时间', dateError: '结束时间不能早于开始时间',
    priorities: '优先处理', failedTasks: '查看失败任务', recentProjects: '最近项目', pendingItems: '待处理事项', overview: '资源概览', basics: '基本信息', data: '参与方与数据', tasks: '近期任务', issues: '待完善配置', validate: '检查配置', valid: '配置完整',
    status: {SUCCESS:'成功',ACTIVE:'可用',PUBLISHED:'已发布',FAILED:'失败',ERROR:'异常',REJECTED:'已拒绝',RUNNING:'运行中',PUBLISHING:'发布中',PENDING:'待处理',WAITING:'等待中',STAGING:'待运行',INIT:'初始化',STOPPED:'已停止',ARCHIVED:'已归档',OFFLINE:'已下线',UNKNOWN:'未知',UNAVAILABLE:'不可用',DISCARDED:'已废弃',AVAILABLE:'可用'},
  },

  projectBrowser: {
    view: '显示方式', listView: '紧凑列表', cardView: '分页卡片', allView: '全部卡片', actions: '操作',
    search: '搜索名称、ID、描述或参与节点', groupBy: '分组方式', mode: '计算模式', status: '项目状态', unknown: '未设置状态',
    count: '显示 {{count}} / {{total}} 个项目', page: '第 {{page}} / {{pages}} 页', previous: '上一页', next: '下一页',
    sceneSearch: '搜索场景、技术或用途', all: '全部场景',
  },
  canvasLibrary: {
    fit: '适应窗口', actualSize: '原尺寸', train: '模型训练', predict: '模型预测', evaluate: '模型评估', align: '隐私求交', split: '训练 / 测试集划分',
    title: '预定义画布', subtitle: '选择算法，查看独立流程。使用模板创建专属项目与画布，数据、参数和编辑记录分别保存。',
    openCustom: '打开自定义画布', search: '搜索算法或模板', list: '预定义画布列表', empty: '没有匹配的模板',
    nodes: '个节点', edges: '条连线', readOnly: '只读预览', preview: '独立画布预览', use: '使用此画布',
    independent: '每个预定义画布单独预览。使用模板后会生成独立副本，可在自定义画布中配置、编辑和运行，不会修改原始模板或其他画布。',
  },

  sidebar: {
    startSection: '工作入口', projectsSection: '项目与画布', dataSection: '数据资源', tasksSection: '任务与成果', networkSection: '节点与协作', platformSection: '平台管理', otherSection: '其他功能',
    predefinedCanvases: '预定义画布',
    nodeView: '节点视角',
    cooperativeNodes: '合作节点',
    allDataSources: '全部数据源',
    allDataTables: '全部数据表',
    myNodeView: '我的节点视角',
    instRegister: '节点注册',
    periodicTaskDetail: '周期任务详情',
    privacyScenes: '隐私场景',
  },
  header: {
    platform: {
      CENTER: '中心化平台',
      EDGE: 'Edge 节点',
      AUTONOMY: '自治模式',
      P2P: 'P2P 模式',
      TEST: '测试模式',
    },
    myInstitution: '我的机构',
    myNode: '我的节点',
    reopenGuide: '体验新手引导',
    community: 'c-life 社区',
    helpCenter: '帮助中心',
    messages: '消息中心',
    componentVersions: '组件版本',
    changePassword: '修改密码',
  },
  account: {
    policy: {
      hint: '新密码需 8–20 位，同时包含大写字母、小写字母和数字，且不能与当前密码相同。',
      length: '新密码长度需为 8–20 位',
      complexity: '新密码需同时包含大写字母、小写字母和数字',
      sameAsOld: '新密码与当前密码不能一致',
    },
  },
  nodeView: {
    title: '节点视角',
    mine: '（本方节点）',
    nodeInfo: '节点信息',
    backToNodes: '返回节点管理',
    tabs: {
      dataSources: '数据源',
      dataTables: '数据管理',
      cooperativeNodes: '合作节点',
      results: '结果管理',
    },
  },
};

export const en: Dictionary = {
  display: {
    resultContent: 'Result content', filters: 'Filters', clearFilters: 'Clear filters', density: 'Density', compact: 'Compact', comfortable: 'Comfortable', columns: 'Columns', width: 'Width', reset: 'Reset view', pageSort: 'Sort the current page by clicking a header', more: 'More',
    search: 'Search', statusFilter: 'Job status', allStatus: 'All statuses', startTime: 'From', endTime: 'Until', dateError: 'End time must not precede start time',
    priorities: 'Needs attention', failedTasks: 'View failed tasks', recentProjects: 'Recent projects', pendingItems: 'Pending requests', overview: 'Resource overview', basics: 'Overview', data: 'Participants & data', tasks: 'Recent jobs', issues: 'Configuration issues', validate: 'Check configuration', valid: 'Configuration complete',
    status: {SUCCESS:'Succeeded',ACTIVE:'Active',PUBLISHED:'Published',FAILED:'Failed',ERROR:'Error',REJECTED:'Rejected',RUNNING:'Running',PUBLISHING:'Publishing',PENDING:'Pending',WAITING:'Waiting',STAGING:'Queued',INIT:'Initialized',STOPPED:'Stopped',ARCHIVED:'Archived',OFFLINE:'Offline',UNKNOWN:'Unknown',UNAVAILABLE:'Unavailable',DISCARDED:'Discarded',AVAILABLE:'Available'},
  },

  projectBrowser: {
    view: 'View', listView: 'Compact list', cardView: 'Paged cards', allView: 'All cards', actions: 'Actions',
    search: 'Search name, ID, description or participant', groupBy: 'Group by', mode: 'Compute mode', status: 'Project status', unknown: 'Status not set',
    count: 'Showing {{count}} of {{total}} projects', page: 'Page {{page}} of {{pages}}', previous: 'Previous', next: 'Next',
    sceneSearch: 'Search scenes, technology or purpose', all: 'All scenes',
  },
  canvasLibrary: {
    fit: 'Fit preview', actualSize: 'Actual size', train: 'Model training', predict: 'Model prediction', evaluate: 'Model evaluation', align: 'Private set intersection', split: 'Train / test split',
    title: 'Predefined Canvases', subtitle: 'Choose an algorithm and preview its independent workflow. Create a separate project and canvas with its own data, parameters and editing history.',
    openCustom: 'Open custom canvas', search: 'Search algorithms or templates', list: 'Predefined canvas list', empty: 'No matching templates',
    nodes: 'nodes', edges: 'edges', readOnly: 'Read-only preview', preview: 'Independent canvas preview', use: 'Use this canvas',
    independent: 'Each template is previewed separately. Using it creates an independent copy to configure, edit and run in the custom canvas editor, without changing the original template or another canvas.',
  },

  sidebar: {
    startSection: 'Get started', projectsSection: 'Projects & canvases', dataSection: 'Data resources', tasksSection: 'Tasks & outputs', networkSection: 'Nodes & collaboration', platformSection: 'Platform management', otherSection: 'Other features',
    predefinedCanvases: 'Predefined Canvases',
    nodeView: 'Node View',
    cooperativeNodes: 'Cooperative Nodes',
    allDataSources: 'All Data Sources',
    allDataTables: 'All Data Tables',
    myNodeView: 'My Node View',
    instRegister: 'Register Node',
    periodicTaskDetail: 'Periodic Task Detail',
    privacyScenes: 'Privacy Scenes',
  },
  header: {
    platform: {
      CENTER: 'Center',
      EDGE: 'Edge Node',
      AUTONOMY: 'Autonomy',
      P2P: 'P2P',
      TEST: 'Test',
    },
    myInstitution: 'My Institution',
    myNode: 'My Node',
    reopenGuide: 'Beginner Guide',
    community: 'c-life Community',
    helpCenter: 'Help Center',
    messages: 'Message Center',
    componentVersions: 'Component Versions',
    changePassword: 'Change Password',
  },
  account: {
    policy: {
      hint: 'The new password must be 8–20 characters, contain upper-case, lower-case letters and digits, and differ from the current one.',
      length: 'The new password must be 8–20 characters long',
      complexity: 'The new password must contain upper-case, lower-case letters and digits',
      sameAsOld: 'The new password must differ from the current password',
    },
  },
  nodeView: {
    title: 'Node View',
    mine: '(own node)',
    nodeInfo: 'Node info',
    backToNodes: 'Back to nodes',
    tabs: {
      dataSources: 'Data Sources',
      dataTables: 'Data Tables',
      cooperativeNodes: 'Cooperative Nodes',
      results: 'Results',
    },
  },
};
