export interface MenuLocaleText {
  localeName: string
  menu: {
    file: {
      title: string
      newGraph: string
      newWindow: string
      open: string
      recent: string
      clearRecent: string
      openWorkspace: string
      refreshWorkspace: string
      refreshNodeLibrary: string
      collapseWorkspace: string
      revealActiveFile: string
      save: string
      saveAs: string
      saveAll: string
      exportSelectedImage: string
      exportGraphImage: string
      quit: string
    }
    edit: {
      title: string
      undo: string
      redo: string
      cut: string
      copy: string
      paste: string
      delete: string
      group: string
      ungroup: string
      selectAll: string
      deselectAll: string
    }
    align: {
      title: string
      verticalCenter: string
      horizontalCenter: string
      verticalDistribute: string
      horizontalDistribute: string
      left: string
      right: string
      top: string
      bottom: string
    }
    view: {
      title: string
      showTestResults: string
      showLeftSidebar: string
      showModuleLibrary: string
      language: string
      chinese: string
      english: string
      settings: string
    }
    blueprint: {
      title: string
      validate: string
    }
    help: {
      title: string
      shortcuts: string
      about: string
    }
  }
  toolbar: {
    test: string
    testTitle: string
  }
  canvas: {
    hint: string
  }
  validation: {
    title: string
    issueCount: string
    noIssues: string
    error: string
    warning: string
    code: string
    nodes: string
    noNode: string
    rerunTitle: string
    expandTitle: string
    collapseTitle: string
    closeTitle: string
  }
  module: {
    title: string
    searchPlaceholder: string
    functionCategory: string
    currentBlueprintFunctions: string
    workspaceFunctionLibrary: string
    noFunctionLibrary: string
  }
  detail: {
    functionTitle: string
    functionTitlePlaceholder: string
    functionCategory: string
    functionCategoryPlaceholder: string
  }
  settings: {
    title: string
    language: string
    uiScale: string
    moduleScale: string
    nodeScale: string
    imageExportScale: string
    showGrid: string
    autoCheckUpdates: string
    checkUpdatesNow: string
    small: string
    normal: string
    large: string
    revealActiveFile: string
    validateBeforeSave: string
    close: string
  }
  update: {
    title: string
    checking: string
    available: string
    upToDate: string
    currentVersion: string
    latestVersion: string
    openRelease: string
    remindLater: string
    noRelease: string
    checkFailed: string
  }
  shortcuts: {
    title: string
    intro: string
    fileTitle: string
    fileBody: string
    canvasTitle: string
    canvasBody: string
    selectionTitle: string
    selectionBody: string
    groupTitle: string
    groupBody: string
    validateTitle: string
    validateBody: string
    exportTitle: string
    exportBody: string
    close: string
  }
  about: {
    title: string
    description: string
    version: string
    runtime: string
    checkUpdates: string
    close: string
  }
}

export const zhCN: MenuLocaleText = {
  localeName: '中文',
  menu: {
    file: {
      title: '文件',
      newGraph: '新建蓝图',
      newWindow: '新建窗口',
      open: '打开',
      recent: '最近文件',
      clearRecent: '清空最近文件',
      openWorkspace: '打开工程目录',
      refreshWorkspace: '刷新工程',
      refreshNodeLibrary: '刷新节点库',
      collapseWorkspace: '折叠工程目录',
      revealActiveFile: '定位当前文件',
      save: '保存',
      saveAs: '另存为',
      saveAll: '全部保存',
      exportSelectedImage: '选中节点图片',
      exportGraphImage: '整张蓝图图片',
      quit: '退出'
    },
    edit: {
      title: '编辑',
      undo: '撤销',
      redo: '重做',
      cut: '剪切',
      copy: '复制',
      paste: '粘贴',
      delete: '删除',
      group: '节点组/取消节点组',
      ungroup: '取消节点组',
      selectAll: '全选',
      deselectAll: '取消选择'
    },
    align: {
      title: '对齐',
      verticalCenter: '垂直居中',
      horizontalCenter: '水平居中',
      verticalDistribute: '垂直分布',
      horizontalDistribute: '水平分布',
      left: '左对齐',
      right: '右对齐',
      top: '顶部对齐',
      bottom: '底部对齐'
    },
    view: {
      title: '视图',
      showTestResults: '显示测试结果',
      showLeftSidebar: '显示左侧栏',
      showModuleLibrary: '显示模块库',
      language: '语言',
      chinese: '中文',
      english: 'English',
      settings: '工程设置...'
    },
    blueprint: {
      title: '蓝图',
      validate: '检查蓝图'
    },
    help: {
      title: '帮助',
      shortcuts: '快捷操作',
      about: '关于 OriginBlueprint'
    }
  },
  toolbar: {
    test: '测试',
    testTitle: '检查蓝图 (F5)'
  },
  canvas: {
    hint: '右键拖拽：平移  中键拖拽：平移  Ctrl：多选  Ctrl + 右键拖拽：切断连线  连线：点击后按 Delete 删除'
  },
  validation: {
    title: '检查结果',
    issueCount: '{count} 条问题',
    noIssues: '无问题',
    error: '错误',
    warning: '警告',
    code: '代码',
    nodes: '节点',
    noNode: '无对应节点',
    rerunTitle: '重新检查蓝图',
    expandTitle: '展开检查结果',
    collapseTitle: '收起检查结果',
    closeTitle: '关闭检查结果'
  },
  module: {
    title: '模块库',
    searchPlaceholder: '搜索模块...',
    functionCategory: '函数',
    currentBlueprintFunctions: '当前蓝图函数',
    workspaceFunctionLibrary: '工程函数库',
    noFunctionLibrary: '未发现工程函数库资源'
  },
  detail: {
    functionTitle: '函数名',
    functionTitlePlaceholder: '函数显示名',
    functionCategory: '类型',
    functionCategoryPlaceholder: '选择或输入函数类型'
  },
  settings: {
    title: '工程设置',
    language: '语言',
    uiScale: '界面字体',
    moduleScale: '模块库字体',
    nodeScale: '节点字体',
    imageExportScale: '图片导出倍率',
    showGrid: '显示网格背景',
    autoCheckUpdates: '自动检查更新',
    checkUpdatesNow: '检查更新',
    small: '小',
    normal: '标准',
    large: '大',
    revealActiveFile: '自动定位当前文件',
    validateBeforeSave: '保存前自动检查',
    close: '关闭'
  },
  update: {
    title: '发现新版本',
    checking: '正在检查更新...',
    available: 'OriginBlueprint {version} 已可下载。',
    upToDate: '当前已是最新版本',
    currentVersion: '当前版本',
    latestVersion: '最新版本',
    openRelease: '打开 GitHub Release',
    remindLater: '稍后提醒',
    noRelease: 'GitHub 上暂无可用发布版本',
    checkFailed: '检查更新失败'
  },
  shortcuts: {
    title: '快捷操作',
    intro: '这里保留日常编辑最常用的操作，完整命令仍以菜单和快捷键显示为准。',
    fileTitle: '文件',
    fileBody: 'Ctrl+N 新建蓝图，Ctrl+O 打开，Ctrl+S 保存，Ctrl+Shift+S 另存为。',
    canvasTitle: '画布',
    canvasBody: '鼠标滚轮缩放，右键或中键拖拽平移，Home 回到图中心。',
    selectionTitle: '选择',
    selectionBody: '左键框选节点，Ctrl 多选，Ctrl+A 全选，Delete 删除选中内容。',
    groupTitle: '节点组',
    groupBody: 'Ctrl+G 对选中节点创建节点组；选中已有节点组再按 Ctrl+G 可取消分组。',
    validateTitle: '检查',
    validateBody: 'F5 检查蓝图结构和执行流问题，底部结果可双击定位节点。',
    exportTitle: '导出',
    exportBody: 'Ctrl+Alt+R 导出选中节点图片，Ctrl+Shift+R 导出整张蓝图图片。',
    close: '关闭'
  },
  about: {
    title: '关于 OriginBlueprint',
    description: 'OriginBlueprint 是用于编辑、校验和维护业务蓝图的可视化编辑器。',
    version: '版本',
    runtime: '技术栈',
    checkUpdates: '检查版本',
    close: '关闭'
  }
}
