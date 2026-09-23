# Allan roadmap

Stage: 0.2.0

Stages go in order: `[x]` is done, `[ ]` is planned. The current stage is the first unfinished one.

## Agent core
- [x] ReAct loop with tool calls
- [x] Tools: shell with destructive-command checks, file read/write, web search, SSH
- [x] PTY mode: interactive commands and `sudo` inside the agent
- [x] Scratchpad compression in long sessions

## Terminal interface
- [x] Bubbletea TUI: slash commands with autocompletion, input history
- [x] Tool calls rendered as separate blocks with status
- [x] Session summary on exit

## Memory and skills
- [x] SQLite memory: sessions, conversations, facts with full-text search, `--resume`
- [x] Semantic memory in ChromaDB
- [x] Skill Engine: successful solutions are saved as skills and reused

## Models
- [x] Anthropic, OpenAI, Ollama, llama.cpp and LM Studio backends
- [x] Automatic selection of an available local backend
- [x] Provider keys right from the TUI (`/api`) and picking a model from a catalog in `/model`
- [x] Grok (xAI) as a provider

## Quality and releases
- [ ] CI: build and tests on every push
- [x] Prebuilt binaries in GitHub releases
