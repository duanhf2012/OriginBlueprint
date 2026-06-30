import type { MenuLocaleText } from './zh-CN'

export const enUS: MenuLocaleText = {
  localeName: 'English',
  menu: {
    file: {
      title: 'File',
      newGraph: 'New Graph',
      newWindow: 'New Window',
      open: 'Open',
      recent: 'Recent',
      clearRecent: 'Clear Recent Files',
      setWorkspace: 'Set Workspace Path',
      save: 'Save',
      saveAs: 'Save As',
      saveAll: 'Save All',
      quit: 'Quit'
    },
    edit: {
      title: 'Edit',
      undo: 'Undo',
      redo: 'Redo',
      cut: 'Cut',
      copy: 'Copy',
      paste: 'Paste',
      delete: 'Delete',
      group: 'Group Nodes',
      ungroup: 'Ungroup Nodes',
      selectAll: 'Select All',
      deselectAll: 'Deselect All'
    },
    align: {
      title: 'Align',
      verticalCenter: 'Align Vertical Center',
      horizontalCenter: 'Align Horizontal Center',
      verticalDistribute: 'Vertical Distribution',
      horizontalDistribute: 'Horizontal Distribution',
      left: 'Align Left',
      right: 'Align Right',
      top: 'Align Top',
      bottom: 'Align Bottom',
      straighten: 'Straighten Edge'
    },
    view: {
      title: 'View',
      showTestResults: 'Show Test Results',
      showLeftSidebar: 'Show Left Sidebar',
      showModuleLibrary: 'Show Module Library',
      language: 'Language',
      chinese: '中文',
      english: 'English'
    },
    render: {
      title: 'Render',
      selectedNodes: 'Render Selected Nodes',
      graph: 'Render Graph'
    },
    test: 'Test',
    help: 'Help'
  },
  toolbar: {
    test: 'Test',
    testTitle: 'Validate blueprint (F5)'
  },
  module: {
    functionCategory: 'Functions',
    currentBlueprintFunctions: 'Current Blueprint Functions',
    workspaceFunctionLibrary: 'Workspace Function Library',
    noFunctionLibrary: 'No workspace function library resources found'
  },
  detail: {
    functionTitle: 'Function Title',
    functionTitlePlaceholder: 'Function display name'
  }
}
