# Allan roadmap

Stage: 0.4.0

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
- [x] Reworked UI: single background, welcome card, framed input, spinner, Markdown replies, collapsed tool output, mouse-wheel scrolling, Russian `allan --help`
- [x] Keyboard navigation: model and provider pickers (arrows, search, Enter), Enter runs the highlighted suggestion, Esc stops the agent, double Ctrl+C to quit

## Memory and skills
- [x] SQLite memory: sessions, conversations, facts with full-text search, `--resume`
- [x] Semantic memory in ChromaDB
- [x] Skill Engine: successful solutions are saved as skills and reused

## Models
- [x] Anthropic, OpenAI, Ollama, llama.cpp and LM Studio backends
- [x] Automatic selection of an available local backend
- [x] Provider keys right from the TUI (`/api`) and picking a model from a catalog in `/model`
- [x] Grok (xAI) as a provider
- [x] Cloud providers: OpenRouter, OpenCode Zen, Ollama Cloud (API key, separate from local Ollama), Hugging Face Inference Providers, Featherless.ai, Z.ai
- [x] Keys in the system keyring (0600 file fallback), custom endpoints, `allan key` with hidden input
- [x] GigaChat (Sber): OAuth with the authorization key, bundled Russian Trusted Root CA, function calling
- [x] Model caption under replies, `/compact` to summarize history
- [x] Interface languages: ru (source), en, de — `/lang`, `--lang`, `tui.lang` in the config
- [x] `allan serve`: headless HTTP/SSE worker for the phone app and the site, token auth, per-client sessions
- [x] `allan connect` and `/connect`: pair the machine with KEYTRON Prime (one-time code from the site, token in the keyring)
- [x] `--resume [id]` really resumes: history is loaded and shown; a specific session id can be given
- [x] Retry instead of retyping: after `/model` an interrupted request repeats on the new model (`/retry` too)
- [x] Skill Engine no longer saves skills for greetings and one-tool turns (cooldown, duplicate check, `skill_enabled`)
- [ ] MCP support: connect MCP servers (stdio and HTTP) as agent tools
- [ ] Test the agent on free models (OpenRouter free, Ollama Cloud Free): tool calls, long sessions
- [ ] Per-user API keys on the worker, so a site user can bring their own model access

## Quality and releases
- [ ] CI: build and tests on every push
- [x] Prebuilt binaries in GitHub releases
