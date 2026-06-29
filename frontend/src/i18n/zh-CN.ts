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
      setWorkspace: string
      save: string
      saveAs: string
      saveAll: string
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
    render: {
      title: string
      selectedNodes: string
      graph: string
    }
    test: string
    help: string
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
      setWorkspace: '设置工程目录',
      save: '保存',
      saveAs: '另存为',
      saveAll: '全部保存',
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
      group: '创建节点组',
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
    render: {
      title: '渲染',
      selectedNodes: '渲染选中节点',
      graph: '渲染整张蓝图'
    },
    test: '测试',
    help: '帮助'
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
  }
}
