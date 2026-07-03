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
      straighten: string
    }
    view: {
      title: string
      showTestResults: string
      showLeftSidebar: string
      showModuleLibrary: string
      language: string
      chinese: string
      english: string
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
  module: {
    functionCategory: string
    currentBlueprintFunctions: string
    workspaceFunctionLibrary: string
    noFunctionLibrary: string
  }
  detail: {
    functionTitle: string
    functionTitlePlaceholder: string
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
      bottom: '底部对齐',
      straighten: '拉直连线'
    },
    view: {
      title: '视图',
      showTestResults: '显示测试结果',
      showLeftSidebar: '显示左侧栏',
      showModuleLibrary: '显示模块库',
      language: '语言',
      chinese: '中文',
      english: 'English'
    },
    blueprint: {
      title: '蓝图',
      validate: '检查蓝图'
    },
    help: {
      title: '帮助',
      about: '快捷操作与关于'
    },
  },
  toolbar: {
    test: '测试',
    testTitle: '检查蓝图 (F5)'
  },
  module: {
    functionCategory: '函数',
    currentBlueprintFunctions: '当前蓝图函数',
    workspaceFunctionLibrary: '工程函数库',
    noFunctionLibrary: '未发现工程函数库资源'
  },
  detail: {
    functionTitle: '函数名',
    functionTitlePlaceholder: '函数显示名'
  }
}
