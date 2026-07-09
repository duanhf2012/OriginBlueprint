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
      shortcuts: 'Shortcuts',
      about: 'About OriginBlueprint'
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
    error: 'Error',
    warning: 'Warning',
    code: 'Code',
    nodes: 'Nodes',
    noNode: 'No linked node',
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
    functionTitlePlaceholder: 'Function display name',
    functionCategory: 'Type',
    functionCategoryPlaceholder: 'Select or enter a function type'
  },
  settings: {
    title: 'Project Settings',
    language: 'Language',
    uiScale: 'UI Font',
    moduleScale: 'Module Library Font',
    nodeScale: 'Node Font',
    imageExportScale: 'Image Export Scale',
    showGrid: 'Show grid background',
    autoCheckUpdates: 'Check for updates automatically',
    checkUpdatesNow: 'Check Updates',
    small: 'Small',
    normal: 'Normal',
    large: 'Large',
    revealActiveFile: 'Reveal active file automatically',
    validateBeforeSave: 'Validate before save',
    close: 'Close'
  },
  update: {
    title: 'Update Available',
    checking: 'Checking for updates...',
    available: 'OriginBlueprint {version} is available.',
    upToDate: 'You are using the latest version',
    currentVersion: 'Current Version',
    latestVersion: 'Latest Version',
    openRelease: 'Open GitHub Release',
    remindLater: 'Remind Me Later',
    noRelease: 'No GitHub release is available yet',
    checkFailed: 'Update check failed'
  },
  shortcuts: {
    title: 'Shortcuts',
    intro: 'These are the most common editing actions. Menus remain the source of truth for every command and shortcut.',
    fileTitle: 'File',
    fileBody: 'Ctrl+N creates a graph, Ctrl+O opens one, Ctrl+S saves, and Ctrl+Shift+S saves as.',
    canvasTitle: 'Canvas',
    canvasBody: 'Mouse wheel zooms, right or middle drag pans, and Home returns to the graph center.',
    selectionTitle: 'Selection',
    selectionBody: 'Left-drag selects nodes, Ctrl adds to selection, Ctrl+A selects all, and Delete removes selected items.',
    groupTitle: 'Groups',
    groupBody: 'Ctrl+G groups selected nodes. Select an existing group and press Ctrl+G again to ungroup.',
    validateTitle: 'Validation',
    validateBody: 'F5 checks graph structure and execution flow issues. Double-click a result to locate its node.',
    exportTitle: 'Export',
    exportBody: 'Ctrl+Alt+R exports selected nodes as an image. Ctrl+Shift+R exports the whole graph image.',
    close: 'Close'
  },
  about: {
    title: 'About OriginBlueprint',
    description: 'OriginBlueprint is a visual editor for creating, validating, and maintaining business blueprints.',
    version: 'Version',
    runtime: 'Runtime',
    checkUpdates: 'Check Version',
    close: 'Close'
  }
}
