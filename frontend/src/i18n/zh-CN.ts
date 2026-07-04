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
  emptyStart: {
    title: string
    body: string
    openWorkspace: string
    openSample: string
    newGraph: string
  }
  validation: {
    title: string
    issueCount: string
    noIssues: string
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
  }
  settings: {
    title: string
    language: string
    uiScale: string
    nodeScale: string
    small: string
    normal: string
    large: string
    revealActiveFile: string
    validateBeforeSave: string
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
      about: '快捷操作与关于'
    }
  },
  toolbar: {
    test: '测试',
    testTitle: '检查蓝图 (F5)'
  },
  canvas: {
    hint: '右键拖拽：平移  中键拖拽：平移  Ctrl：多选  Ctrl + 右键拖拽：切断连线  连线：点击后按 Delete 删除'
  },
  emptyStart: {
    title: '开始编辑蓝图',
    body: '打开工程目录以加载蓝图文件和节点库，或先查看示例工程。',
    openWorkspace: '打开工程目录',
    openSample: '打开示例工程',
    newGraph: '新建空白蓝图'
  },
  validation: {
    title: '检查结果',
    issueCount: '{count} 条问题',
    noIssues: '无问题',
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
    functionTitlePlaceholder: '函数显示名'
  },
  settings: {
    title: '工程设置',
    language: '语言',
    uiScale: '界面字体',
    nodeScale: '节点字体',
    small: '小',
    normal: '标准',
    large: '大',
    revealActiveFile: '自动定位当前文件',
    validateBeforeSave: '保存前自动检查',
    close: '关闭'
  }
}
