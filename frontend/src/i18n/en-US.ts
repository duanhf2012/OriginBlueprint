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
      openWorkspace: 'Open Workspace Folder',
      refreshWorkspace: 'Refresh Workspace',
      refreshNodeLibrary: 'Refresh Node Library',
      collapseWorkspace: 'Collapse Workspace Tree',
      revealActiveFile: 'Reveal Active File',
      save: 'Save',
      saveAs: 'Save As',
      saveAll: 'Save All',
      exportSelectedImage: 'Selected Nodes Image',
      exportGraphImage: 'Whole Graph Image',
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
      group: 'Group / Ungroup Nodes',
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
      bottom: 'Align Bottom'
    },
    view: {
      title: 'View',
      showTestResults: 'Show Test Results',
      showLeftSidebar: 'Show Left Sidebar',
      showModuleLibrary: 'Show Module Library',
      language: 'Language',
      chinese: '中文',
      english: 'English',
      settings: 'Project Settings...'
    },
    blueprint: {
      title: 'Blueprint',
      validate: 'Validate Blueprint'
    },
    help: {
      title: 'Help',
      about: 'Shortcuts and About'
    }
  },
  toolbar: {
    test: 'Test',
    testTitle: 'Validate blueprint (F5)'
  },
  canvas: {
    hint: 'Right drag: pan  Middle drag: pan  Ctrl: multi-select  Ctrl + right drag: cut connections  Connection: click + Delete'
  },
  validation: {
    title: 'Test Results',
    issueCount: '{count} issue(s)',
    noIssues: 'No issues',
    rerunTitle: 'Validate blueprint again',
    expandTitle: 'Expand Test Results',
    collapseTitle: 'Collapse Test Results',
    closeTitle: 'Close Test Results'
  },
  module: {
    title: 'Module Library',
    searchPlaceholder: 'Search modules...',
    functionCategory: 'Functions',
    currentBlueprintFunctions: 'Current Blueprint Functions',
    workspaceFunctionLibrary: 'Workspace Function Library',
    noFunctionLibrary: 'No workspace function library resources found'
  },
  detail: {
    functionTitle: 'Function Title',
    functionTitlePlaceholder: 'Function display name'
  },
  settings: {
    title: 'Project Settings',
    language: 'Language',
    uiScale: 'UI Font',
    nodeScale: 'Node Font',
    small: 'Small',
    normal: 'Normal',
    large: 'Large',
    revealActiveFile: 'Reveal active file automatically',
    validateBeforeSave: 'Validate before save',
    close: 'Close'
  }
}
