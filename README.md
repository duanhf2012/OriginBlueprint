# OriginBlueprint

A cross-platform blueprint editor built with Go, Wails v2, Vue 3, TypeScript, and Rete.js v2.

## Architecture

Go owns domain rules, persistence, validation, migration, compilation, and platform services. Vue/TypeScript owns Rete.js rendering and high-frequency canvas interaction.

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) before adding or moving application logic.

## Live Development

Run `wails dev` in this directory for desktop development with frontend hot reload.

## Building

Run `wails build` to build the Windows executable. Double-click `run.bat` to start an existing build or build it automatically when needed.
