/// <reference types="node" />

import fs from 'node:fs'
import path from 'node:path'
import {defineConfig, type Plugin} from 'vite'
import vue from '@vitejs/plugin-vue'

function collectNodeJsonFiles(nodesDir: string) {
  const result: string[] = []
  const walk = (dir: string, prefix = '') => {
    if (!fs.existsSync(dir)) return
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
      const relative = prefix ? `${prefix}/${entry.name}` : entry.name
      const fullPath = path.join(dir, entry.name)
      if (entry.isDirectory()) walk(fullPath, relative)
      else if (entry.isFile() && entry.name.toLowerCase().endsWith('.json')) result.push(relative)
    }
  }
  walk(nodesDir)
  return result.sort()
}

function nodeLibraryPlugin(): Plugin {
  const nodesDir = path.resolve(__dirname, '..', 'nodes')
  const nodePath = (relative: string) => path.resolve(nodesDir, relative)
  const isInsideNodes = (target: string) => target === nodesDir || target.startsWith(`${nodesDir}${path.sep}`)

  return {
    name: 'origin-node-library',
    configureServer(server) {
      server.middlewares.use('/nodes', (request, response, next) => {
        const url = decodeURIComponent(request.url ?? '/')
        if (url === '/manifest.json') {
          response.setHeader('Content-Type', 'application/json; charset=utf-8')
          response.end(JSON.stringify(collectNodeJsonFiles(nodesDir)))
          return
        }

        const relative = url.replace(/^\/+/, '')
        const filePath = nodePath(relative)
        if (!isInsideNodes(filePath) || !filePath.toLowerCase().endsWith('.json') || !fs.existsSync(filePath)) {
          next()
          return
        }

        response.setHeader('Content-Type', 'application/json; charset=utf-8')
        fs.createReadStream(filePath).pipe(response)
      })
    },
    generateBundle() {
      const files = collectNodeJsonFiles(nodesDir)
      this.emitFile({ type: 'asset', fileName: 'nodes/manifest.json', source: JSON.stringify(files) })
      for (const file of files) {
        this.emitFile({ type: 'asset', fileName: `nodes/${file}`, source: fs.readFileSync(nodePath(file)) })
      }
    }
  }
}

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [vue(), nodeLibraryPlugin()]
})
