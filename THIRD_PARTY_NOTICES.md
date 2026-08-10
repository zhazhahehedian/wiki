# Third-Party Notices

## LiveAgent

- Project: LiveAgent
- Source: https://github.com/Stack-Cairn/LiveAgent
- Source commit: `d5a27420633390aa53b72c0fade011f69ad8be4b`
- License: MIT License
- Copyright: Copyright (c) 2026 Stack-Cairn

Migrated source mapping:

- `crates/agent-ui/src/components/chat/TaskProgressIndicator.tsx` -> `frontend/components/liveagent/task-progress-indicator.tsx` (adapted to the existing tool-call model and lucide icon set)
- `crates/agent-ui/src/components/chat/TaskProgressIndicator.tsx` -> `frontend/styles/liveagent.css` (extracted and renamed only the progress indicator tokens and surface styles used by this product)

No LiveAgent Gateway, Tauri, Git, terminal, desktop runtime, or protocol code is included.

MIT License

Copyright (c) 2026 Stack-Cairn

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
