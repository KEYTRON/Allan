#!/usr/bin/env python3
"""Generates internal/i18n/catalog.go from the Russian source strings found in
the TUI and the CLI. A string missing from TRANSLATIONS is reported, not
silently skipped, so a new Russian message cannot reach users untranslated."""
import re
import glob
import sys
from collections import OrderedDict

EN = {}
DE = {}


def add(ru, en, de):
    EN[ru] = en
    DE[ru] = de


# --- machine identity (allan id, connect) ---
add("  id машины %s\n", "  machine id %s\n", "  Maschinen-ID %s\n")
add("  адрес     %s (по нему сайт достучится до воркера)\n",
    "  address   %s (the site reaches the worker here)\n",
    "  Adresse   %s (hierüber erreicht die Seite den Worker)\n")
add("  адрес     не найден (нет Tailscale): укажите --address http://<ip>:8790",
    "  address   not found (no Tailscale): pass --address http://<ip>:8790",
    "  Adresse   nicht gefunden (kein Tailscale): --address http://<ip>:8790 angeben")
add("Эта машина", "This machine", "Diese Maschine")
add("  имя        %s\n", "  name       %s\n", "  Name       %s\n")
add("  id         %s\n", "  id         %s\n", "  ID         %s\n")
add("  система    %s\n", "  system     %s\n", "  System     %s\n")
add("  адрес      %s (Tailscale)\n", "  address    %s (Tailscale)\n", "  Adresse    %s (Tailscale)\n")
add("  адрес      Tailscale не найден", "  address    Tailscale not found", "  Adresse    Tailscale nicht gefunden")
add("  привязка   нет — allan connect", "  pairing    none — allan connect", "  Kopplung   keine — allan connect")
add("  привязка   %s как «%s»\n", "  pairing    %s as “%s”\n", "  Kopplung   %s als „%s“\n")
add("id хранится в %s и не меняется при обновлении и повторной привязке.",
    "The id is kept in %s and does not change on upgrade or re-pairing.",
    "Die ID liegt in %s und ändert sich weder beim Update noch bei erneuter Kopplung.")
add("\nЭта машина: %s · id %s  (подробнее: allan id)\n",
    "\nThis machine: %s · id %s  (details: allan id)\n",
    "\nDiese Maschine: %s · ID %s  (Details: allan id)\n")

# --- slash commands and help ---
add("Список команд", "List of commands", "Befehlsliste")
add("Выбрать модель из списка (↑↓, поиск, Enter) или /model <provider/model>",
    "Pick a model from the list (arrows, search, Enter) or /model <provider/model>",
    "Modell aus der Liste wählen (Pfeiltasten, Suche, Enter) oder /model <anbieter/modell>")
add("API-ключи и эндпоинты: /api add|key|endpoint|list|use|remove|refresh",
    "API keys and endpoints: /api add|key|endpoint|list|use|remove|refresh",
    "API-Schlüssel und Endpunkte: /api add|key|endpoint|list|use|remove|refresh")
add("Выбрать провайдера, затем его модель", "Pick a provider, then its model",
    "Anbieter wählen, dann dessen Modell")
add("Продолжить сессию: /resume <id> (без аргументов — список последних)",
    "Resume a session: /resume <id> (no arguments lists recent ones)",
    "Sitzung fortsetzen: /resume <id> (ohne Argument werden die letzten gelistet)")
add("Связать машину с KEYTRON Prime: /connect <код> (страница выдаёт код)",
    "Pair this machine with KEYTRON Prime: /connect <code> (the page issues a code)",
    "Diese Maschine mit KEYTRON Prime verbinden: /connect <Code> (die Seite gibt einen Code aus)")
add("Список доступных тулз", "List of available tools", "Liste der verfügbaren Werkzeuge")
add("Состояние памяти", "Memory state", "Speicherzustand")
add("Текущий план агента (scratchpad)", "Current agent plan (scratchpad)",
    "Aktueller Agentenplan (Scratchpad)")
add("Список навыков (или show/delete/export)", "List of skills (or show/delete/export)",
    "Liste der Fähigkeiten (oder show/delete/export)")
add("Сжать историю в сводку, чтобы экономить контекст и токены",
    "Compress the history into a summary to save context and tokens",
    "Verlauf zu einer Zusammenfassung verdichten, um Kontext und Tokens zu sparen")
add("Очистить историю", "Clear the history", "Verlauf leeren")
add("Показать текущий конфиг", "Show the current config", "Aktuelle Konfiguration anzeigen")
add("Выход", "Quit", "Beenden")
add("Команды\n", "Commands\n", "Befehle\n")
add("\nКлавиши\n", "\nKeys\n", "\nTasten\n")
add("отправить; в подсказке — выполнить выбранную команду",
    "send; with the popup open — run the highlighted command",
    "senden; bei offener Vorschlagsliste — gewählten Befehl ausführen")
add("дополнить команду; при запущенном shell — переключить фокус",
    "complete a command; with a running shell — switch focus",
    "Befehl vervollständigen; bei laufender Shell — Fokus wechseln")
add("выбор в подсказке или списке, иначе история ввода",
    "move in the popup or list, otherwise walk the input history",
    "in Vorschlagsliste oder Liste bewegen, sonst Eingabehistorie durchgehen")
add("закрыть подсказку или список; пока агент работает — остановить его",
    "close the popup or list; while the agent is working — stop it",
    "Vorschlagsliste oder Liste schließen; während der Agent läuft — ihn stoppen")
add("прокрутка, колесо мыши тоже работает", "scroll; the mouse wheel works too",
    "scrollen; auch das Mausrad funktioniert")
add("Shift+мышь", "Shift+mouse", "Shift+Maus")
add("выделить и скопировать текст", "select and copy text", "Text markieren und kopieren")
add("очистить ввод; дважды на пустом вводе — выход",
    "clear the input; twice on an empty input — quit",
    "Eingabe leeren; zweimal bei leerer Eingabe — beenden")
add("\nВне TUI: allan key set <provider>, allan connect, allan serve. Справка: allan --help",
    "\nOutside the TUI: allan key set <provider>, allan connect, allan serve. Help: allan --help",
    "\nAußerhalb der TUI: allan key set <anbieter>, allan connect, allan serve. Hilfe: allan --help")

# --- picker, rendering ---
add("Enter — выбрать, печатайте для поиска", "Enter — choose, type to search",
    "Enter — auswählen, tippen zum Suchen")
add("  ничего не найдено", "  nothing found", "  nichts gefunden")
add("%d/%d · ↑↓ выбор · Enter выбрать · Esc отмена · печатайте для поиска",
    "%d/%d · ↑↓ move · Enter choose · Esc cancel · type to search",
    "%d/%d · ↑↓ wählen · Enter übernehmen · Esc abbrechen · tippen zum Suchen")
add("(нет вывода)", "(no output)", "(keine Ausgabe)")
add("    … ещё %d строк", "    … %d more lines", "    … noch %d Zeilen")

# --- session, thinking, skills ---
add("/ — команды, или просто напишите задачу",
    "/ — commands, or just write your task",
    "/ — Befehle, oder schreiben Sie einfach Ihre Aufgabe")
add("История сжата. Модель дальше видит эту сводку:",
    "History compressed. The model now sees this summary:",
    "Verlauf verdichtet. Das Modell sieht ab jetzt diese Zusammenfassung:")
add("Нажмите Ctrl+C ещё раз, чтобы выйти", "Press Ctrl+C again to quit",
    "Nochmal Ctrl+C drücken, um zu beenden")
add("Остановлено", "Stopped", "Angehalten")
add("добавить API-ключ", "add an API key", "API-Schlüssel hinzufügen")
add("заменить ключ провайдера", "replace a provider key", "Anbieterschlüssel ersetzen")
add("свой эндпоинт (base_url)", "custom endpoint (base_url)", "eigener Endpunkt (base_url)")
add("показать сохранённые провайдеры", "show saved providers", "gespeicherte Anbieter anzeigen")
add("опросить модели всех провайдеров", "query the models of all providers",
    "Modelle aller Anbieter abfragen")
add("сделать провайдера активным", "make a provider active", "Anbieter aktivieren")
add("удалить провайдера", "remove a provider", "Anbieter entfernen")
add("сначала обновить список моделей", "refresh the model list first",
    "zuerst die Modellliste aktualisieren")
add("выбрать модель", "choose a model", "Modell wählen")
add("Повторяю последний запрос на %s/%s%s", "Repeating the last request on %s/%s%s",
    "Letzte Anfrage wird auf %s/%s wiederholt%s")
add("Allan думает", "Allan is thinking", "Allan denkt nach")
add("\nЭта облачная модель платная на вашем тарифе Ollama — выберите другую: /model",
    "\nThis cloud model costs money on your Ollama plan — pick another one: /model",
    "\nDieses Cloud-Modell ist in Ihrem Ollama-Tarif kostenpflichtig — wählen Sie ein anderes: /model")
add("Навык сохранён: %q (/skills чтобы посмотреть)",
    "Skill saved: %q (/skills to look at it)",
    "Fähigkeit gespeichert: %q (/skills zeigt sie an)")
add("(scratchpad пуст)", "(scratchpad is empty)", "(Scratchpad ist leer)")
add("Память отключена", "Memory is disabled", "Speicher ist deaktiviert")
add("Память включена\nsession=%s\ndb=%s\nchromadb=%s\ntool_calls=%d\ntokens_in=%d\ntokens_out=%d",
    "Memory is on\nsession=%s\ndb=%s\nchromadb=%s\ntool_calls=%d\ntokens_in=%d\ntokens_out=%d",
    "Speicher ist an\nsession=%s\ndb=%s\nchromadb=%s\ntool_calls=%d\ntokens_in=%d\ntokens_out=%d")
add("Смена бэкенда требует перезапуска. Установлено в конфиге: ",
    "Switching the backend needs a restart. Set in the config: ",
    "Ein Backend-Wechsel braucht einen Neustart. In der Konfiguration gesetzt: ")
add("Неизвестная команда: ", "Unknown command: ", "Unbekannter Befehl: ")
add("Память отключена, продолжать нечего", "Memory is disabled, nothing to resume",
    "Speicher ist deaktiviert, nichts fortzusetzen")
add("Нет завершённых сессий", "No finished sessions", "Keine abgeschlossenen Sitzungen")
add("Последние сессии (/resume <id>):\n", "Recent sessions (/resume <id>):\n",
    "Letzte Sitzungen (/resume <id>):\n")
add("  %d. %s  %s/%s  сообщений: %d\n", "  %d. %s  %s/%s  messages: %d\n",
    "  %d. %s  %s/%s  Nachrichten: %d\n")
add("Сессия не найдена или пуста: ", "Session not found or empty: ",
    "Sitzung nicht gefunden oder leer: ")
add("Сессия загружена, но не переоткрыта: ", "Session loaded but not reopened: ",
    "Sitzung geladen, aber nicht wieder geöffnet: ")

# --- connect ---
add("\n\nВоркер для телефона и сайта запускается отдельно: allan serve",
    "\n\nThe worker for the phone and the site runs separately: allan serve",
    "\n\nDer Worker für Handy und Webseite läuft getrennt: allan serve")
add("Связь с KEYTRON Prime удалена, токен стёрт.",
    "Pairing with KEYTRON Prime removed, token deleted.",
    "Verbindung zu KEYTRON Prime gelöscht, Token entfernt.")
add("Код не принят: ", "Code rejected: ", "Code abgelehnt: ")
add("Машина связана с ", "Machine paired with ", "Maschine verbunden mit ")
add("\nтокен ", "\ntoken ", "\nToken ")
add(" в системном keyring\nзапустите воркер: allan serve",
    " in the system keyring\nstart the worker: allan serve",
    " im System-Keyring\nWorker starten: allan serve")
add("Skills engine не инициализирован", "Skill Engine is not initialised",
    "Skill-Engine ist nicht initialisiert")
add("Навыков пока нет.", "No skills yet.", "Noch keine Fähigkeiten.")
add("Навык удалён.", "Skill deleted.", "Fähigkeit gelöscht.")
add("Подкоманды: show, delete, export", "Subcommands: show, delete, export",
    "Unterbefehle: show, delete, export")

# --- /api ---
add("/api list — провайдеры, эндпоинты и где лежат ключи",
    "/api list — providers, endpoints and where the keys live",
    "/api list — Anbieter, Endpunkte und wo die Schlüssel liegen")
add("/api add <provider> <api_key> [base_url] — ключ провайдера (и свой base_url)",
    "/api add <provider> <api_key> [base_url] — provider key (and a custom base_url)",
    "/api add <anbieter> <api_key> [base_url] — Anbieterschlüssel (und eigener base_url)")
add("/api key <provider> <api_key> — заменить только ключ",
    "/api key <provider> <api_key> — replace only the key",
    "/api key <anbieter> <api_key> — nur den Schlüssel ersetzen")
add("/api endpoint <name> <base_url> [openai|anthropic] — свой эндпоинт без ключа или смена base_url",
    "/api endpoint <name> <base_url> [openai|anthropic] — custom endpoint without a key, or change base_url",
    "/api endpoint <name> <base_url> [openai|anthropic] — eigener Endpunkt ohne Schlüssel oder base_url wechseln")
add("/api remove <provider> — удалить провайдер вместе с ключом",
    "/api remove <provider> — remove the provider together with its key",
    "/api remove <anbieter> — Anbieter samt Schlüssel entfernen")
add("Ключ безопаснее вводить вне TUI: allan key set <provider> (скрытый ввод).",
    "Safer to enter a key outside the TUI: allan key set <provider> (hidden input).",
    "Schlüssel sicherer außerhalb der TUI eingeben: allan key set <anbieter> (verdeckte Eingabe).")
add("Ключи хранятся в ", "Keys are stored in ", "Schlüssel liegen in ")
add(", не в config.toml.", ", not in config.toml.", ", nicht in config.toml.")
add("Провайдеры: ", "Providers: ", "Anbieter: ")
add("; локальные: ollama, llamacpp, lmstudio.", "; local: ollama, llamacpp, lmstudio.",
    "; lokal: ollama, llamacpp, lmstudio.")
add("Ключ для %s сохранён в %s. Выполните /api refresh или /model, чтобы обновить список моделей.",
    "Key for %s saved in %s. Run /api refresh or /model to update the model list.",
    "Schlüssel für %s in %s gespeichert. Führen Sie /api refresh oder /model aus, um die Modellliste zu aktualisieren.")
add("Ключ для %s обновлён (%s).", "Key for %s updated (%s).", "Schlüssel für %s aktualisiert (%s).")
add("Тип эндпоинта: openai (OpenAI-совместимый) или anthropic",
    "Endpoint type: openai (OpenAI-compatible) or anthropic",
    "Endpunkt-Typ: openai (OpenAI-kompatibel) oder anthropic")
add("Эндпоинт %s → %s сохранён. Ключ при необходимости: /api key %s <api_key>",
    "Endpoint %s → %s saved. Key if needed: /api key %s <api_key>",
    "Endpunkt %s → %s gespeichert. Schlüssel falls nötig: /api key %s <api_key>")
add("ключ не удалён: ", "key not removed: ", "Schlüssel nicht entfernt: ")
add("Провайдер и его ключ удалены: ", "Provider and its key removed: ",
    "Anbieter und Schlüssel entfernt: ")
add("Подкоманды /api: list, add, key, endpoint, use, remove, refresh",
    "/api subcommands: list, add, key, endpoint, use, remove, refresh",
    "/api Unterbefehle: list, add, key, endpoint, use, remove, refresh")
add("пустое имя провайдера", "empty provider name", "leerer Anbietername")
add("%s не входит в список известных провайдеров — укажите base_url: /api add %s <api_key> <base_url>",
    "%s is not a known provider — give a base_url: /api add %s <api_key> <base_url>",
    "%s ist kein bekannter Anbieter — base_url angeben: /api add %s <api_key> <base_url>")
add("хранилище ключей не инициализировано", "key store is not initialised",
    "Schlüsselspeicher ist nicht initialisiert")
add("не удалось сохранить ключ в %s: %w", "could not save the key in %s: %w",
    "Schlüssel konnte nicht in %s gespeichert werden: %w")
add("не настроено", "not configured", "nicht eingerichtet")
add("Модель не найдена. Сначала выполните /model или /api refresh, затем /model <номер>.",
    "Model not found. Run /model or /api refresh first, then /model <number>.",
    "Modell nicht gefunden. Führen Sie zuerst /model oder /api refresh aus, dann /model <Nummer>.")
add("не удалось получить модели: %s", "could not fetch models: %s",
    "Modelle konnten nicht geladen werden: %s")
add("Модели не найдены. Для API-провайдера добавьте ключ: /api add <provider> <api_key>",
    "No models found. For an API provider add a key: /api add <provider> <api_key>",
    "Keine Modelle gefunden. Für einen API-Anbieter einen Schlüssel hinzufügen: /api add <anbieter> <api_key>")
add("Текущая модель: %s/%s\n\n", "Current model: %s/%s\n\n", "Aktuelles Modell: %s/%s\n\n")
add("\nВыбор: /model <номер> или /model <provider>/<model>",
    "\nChoose: /model <number> or /model <provider>/<model>",
    "\nAuswahl: /model <Nummer> oder /model <anbieter>/<modell>")
add("\nfeatherless: каталог ~20 тыс. моделей в список не выводится, выбор по id: /model featherless/<owner>/<model> (список: featherless.ai/models)",
    "\nfeatherless: the catalogue of ~20k models is not listed; choose by id: /model featherless/<owner>/<model> (list: featherless.ai/models)",
    "\nfeatherless: Der Katalog mit ~20 Tsd. Modellen wird nicht aufgelistet; Auswahl per ID: /model featherless/<owner>/<modell> (Liste: featherless.ai/models)")
add("API-провайдеры не настроены. Добавить: /api add <provider> <api_key> [base_url]\nКлючи хранятся в ",
    "No API providers configured. Add one: /api add <provider> <api_key> [base_url]\nKeys are stored in ",
    "Keine API-Anbieter eingerichtet. Hinzufügen: /api add <anbieter> <api_key> [base_url]\nSchlüssel liegen in ")
add("нет", "no", "nein")
add("есть", "yes", "ja")
add("%s%s: type=%s base_url=%s ключ=%s\n", "%s%s: type=%s base_url=%s key=%s\n",
    "%s%s: type=%s base_url=%s Schlüssel=%s\n")
add("\nКлючи хранятся в ", "\nKeys are stored in ", "\nSchlüssel liegen in ")
add("Модель сменена: %s/%s", "Model changed: %s/%s", "Modell gewechselt: %s/%s")

# --- status bar, welcome, pickers ---
add("%s… %s · %s · Esc — остановить", "%s… %s · %s · Esc — stop",
    "%s… %s · %s · Esc — anhalten")
add("Загрузка…", "Loading…", "Wird geladen…")
add("модель", "model", "Modell")
add("папка", "folder", "Ordner")
add("ключи API", "API keys", "API-Schlüssel")
add("выбрать модель: ↑↓, поиск, Enter", "pick a model: arrows, search, Enter",
    "Modell wählen: Pfeiltasten, Suche, Enter")
add("ключи провайдеров и свои эндпоинты", "provider keys and custom endpoints",
    "Anbieterschlüssel und eigene Endpunkte")
add("все команды", "all commands", "alle Befehle")
add("дополнить команду", "complete a command", "Befehl vervollständigen")
add("подсказки и история ввода", "suggestions and input history",
    "Vorschläge und Eingabehistorie")
add("остановить агента", "stop the agent", "Agenten stoppen")
add("выход", "quit", "beenden")
add("  %d из %d · ↑↓ выбор · Tab вставить", "  %d of %d · ↑↓ move · Tab insert",
    "  %d von %d · ↑↓ wählen · Tab einfügen")
add(" · Tab: фокус ", " · Tab: focus ", " · Tab: Fokus ")
add("инструменты %d · токены %s · /help ", "tools %d · tokens %s · /help ",
    "Werkzeuge %d · Tokens %s · /help ")
add("не для чата", "not for chat", "nicht für Chat")
add("облако Ollama", "Ollama cloud", "Ollama Cloud")
add("бесплатно", "free", "kostenlos")
add("Модели не найдены. Добавьте ключ: /api add <provider> <api_key>",
    "No models found. Add a key: /api add <provider> <api_key>",
    "Keine Modelle gefunden. Schlüssel hinzufügen: /api add <anbieter> <api_key>")
add("Выбор модели", "Model picker", "Modellauswahl")
add("локальный", "local", "lokal")
add(" · без ключа", " · no key", " · ohne Schlüssel")
add("Выбор провайдера", "Provider picker", "Anbieterauswahl")
add("Продолжена сессия %s — %d сообщений. Всё дальше дописывается в неё же.",
    "Resumed session %s — %d messages. Everything from now on is appended to it.",
    "Sitzung %s fortgesetzt — %d Nachrichten. Alles Weitere wird daran angehängt.")
add("Сжимаю историю", "Compressing the history", "Verlauf wird verdichtet")
add(" токенов", " tokens", " Tokens")

# --- CLI: connect ---
add("Запустите: allan connect", "Run: allan connect", "Ausführen: allan connect")
add("Запустить воркер:  allan serve", "Start the worker:  allan serve",
    "Worker starten:  allan serve")
add("не удалось сбросить: %v\n", "could not reset: %v\n", "Zurücksetzen fehlgeschlagen: %v\n")
add("Связь с %s удалена, токен воркера стёрт.\n",
    "Pairing with %s removed, worker token deleted.\n",
    "Verbindung zu %s gelöscht, Worker-Token entfernt.\n")
add("Нечего сбрасывать.", "Nothing to reset.", "Nichts zum Zurücksetzen.")
add("Код со страницы: ", "Code from the page: ", "Code von der Seite: ")
add("ввод кода: %v\n", "reading the code: %v\n", "Code-Eingabe: %v\n")
add("пустой код, ничего не подключено", "empty code, nothing was paired",
    "leerer Code, es wurde nichts verbunden")
add("не получилось обменять код: %v\n", "could not exchange the code: %v\n",
    "Code konnte nicht eingetauscht werden: %v\n")
add("Проверьте, что код ещё действителен и вы вошли на %s\n",
    "Check that the code is still valid and that you are signed in to %s\n",
    "Prüfen Sie, ob der Code noch gültig ist und Sie bei %s angemeldet sind\n")
add("Машина связана с %s\n", "Machine paired with %s\n", "Maschine verbunden mit %s\n")
add("  имя       %s\n", "  name      %s\n", "  Name      %s\n")
add("  токен     %s (в системном keyring)\n", "  token     %s (in the system keyring)\n",
    "  Token     %s (im System-Keyring)\n")
add("Теперь запустите воркер, чтобы телефон и сайт до него достучались:",
    "Now start the worker so the phone and the site can reach it:",
    "Starten Sie jetzt den Worker, damit Handy und Webseite ihn erreichen:")

# --- CLI: key ---
add("%-14s type=%-9s base_url=%s ключ=%s\n", "%-14s type=%-9s base_url=%s key=%s\n",
    "%-14s type=%-9s base_url=%s Schlüssel=%s\n")
add("Провайдеры не настроены.", "No providers configured.", "Keine Anbieter eingerichtet.")
add("Хранилище ключей:", "Key store:", "Schlüsselspeicher:")
add("%s не входит в список известных провайдеров — укажите base_url: allan key set %s <base_url>\n",
    "%s is not a known provider — give a base_url: allan key set %s <base_url>\n",
    "%s ist kein bekannter Anbieter — base_url angeben: allan key set %s <base_url>\n")
add("API-ключ для %s: ", "API key for %s: ", "API-Schlüssel für %s: ")
add("Токен Hugging Face (hf_…, huggingface.co/settings/tokens): ",
    "Hugging Face token (hf_…, huggingface.co/settings/tokens): ",
    "Hugging-Face-Token (hf_…, huggingface.co/settings/tokens): ")
add("Ключ авторизации GigaChat (developers.sber.ru/studio → API-ключи; не Client Secret): ",
    "GigaChat authorisation key (developers.sber.ru/studio → API keys; not a Client Secret): ",
    "GigaChat-Autorisierungsschlüssel (developers.sber.ru/studio → API-Schlüssel; kein Client Secret): ")
add("ввод ключа: %v\n", "reading the key: %v\n", "Schlüssel-Eingabe: %v\n")
add("пустой ключ, ничего не сохранено", "empty key, nothing was saved",
    "leerer Schlüssel, es wurde nichts gespeichert")
add("не удалось сохранить ключ в %s: %v\n", "could not save the key in %s: %v\n",
    "Schlüssel konnte nicht in %s gespeichert werden: %v\n")
add("Ключ для %s сохранён (%s), base_url=%s\n", "Key for %s saved (%s), base_url=%s\n",
    "Schlüssel für %s gespeichert (%s), base_url=%s\n")
add("ключ не удалён: %v\n", "key not removed: %v\n", "Schlüssel nicht entfernt: %v\n")
add("Удалено:", "Removed:", "Entfernt:")

# --- CLI: main ---
add("провайдер", "provider", "Anbieter")
add("рабочая папка", "working folder", "Arbeitsordner")
add("продолжить сессию (пусто = последняя)", "resume a session (empty = the last one)",
    "Sitzung fortsetzen (leer = die letzte)")
add("версия", "version", "Version")
add("без памяти", "without memory", "ohne Speicher")
add("Создан дефолтный конфиг: %s\n", "Default config created: %s\n",
    "Standardkonfiguration erstellt: %s\n")
add("Продолжена сессия %s: сообщений %d, модель %s/%s\n",
    "Resumed session %s: %d messages, model %s/%s\n",
    "Sitzung %s fortgesetzt: %d Nachrichten, Modell %s/%s\n")
add("◆ allan · сессия завершена за %s\n", "◆ allan · session finished in %s\n",
    "◆ allan · Sitzung beendet in %s\n")
add("  модель       %s/%s\n", "  model        %s/%s\n", "  Modell       %s/%s\n")
add("  инструменты  %d (✓ %d  ✗ %d)\n", "  tools        %d (✓ %d  ✗ %d)\n",
    "  Werkzeuge    %d (✓ %d  ✗ %d)\n")
add("  токены       %d вход · %d выход\n", "  tokens       %d in · %d out\n",
    "  Token        %d Eingang · %d Ausgang\n")
add("  сессия       %s (продолжить: allan --resume %s)\n",
    "  session      %s (continue: allan --resume %s)\n",
    "  Sitzung      %s (fortsetzen: allan --resume %s)\n")

# --- CLI: serve ---
add("адрес слушания", "listen address", "Adresse zum Lauschen")
add("общий токен", "shared token", "gemeinsamer Token")
add("сколько сессий держать", "how many sessions to keep", "wie viele Sitzungen behalten")
add("слушать на всех интерфейсах", "listen on all interfaces", "auf allen Schnittstellen lauschen")
add("\nСначала свяжите машину с KEYTRON Prime: allan connect\n",
    "\nPair this machine with KEYTRON Prime first: allan connect\n",
    "\nBinden Sie die Maschine zuerst an KEYTRON Prime: allan connect\n")
add("не удалось занять %s: %v\n", "could not bind %s: %v\n", "%s konnte nicht belegt werden: %v\n")
add("◆ allan serve %s слушает %s\n", "◆ allan serve %s is listening on %s\n",
    "◆ allan serve %s lauscht auf %s\n")
add("  токен      %s (%s)\n", "  token      %s (%s)\n", "  Token      %s (%s)\n")
add("  страница   %s/pair\n", "  page       %s/pair\n", "  Seite      %s/pair\n")
add("  остановить: Ctrl+C\n\n", "  stop: Ctrl+C\n\n", "  beenden: Strg+C\n\n")
add("воркер остановлен", "worker stopped", "Worker gestoppt")
add("нет токена воркера", "no worker token", "kein Worker-Token")
add("из --token", "from --token", "aus --token")
add("из ALLAN_WORKER_TOKEN", "from ALLAN_WORKER_TOKEN", "aus ALLAN_WORKER_TOKEN")
add("из allan connect", "from allan connect", "aus allan connect")


def unescape(s):
    """Turn Go source escapes into the real characters they denote."""
    return (s.replace("\\n", "\n").replace("\\t", "\t")
             .replace('\\"', '"').replace("\\\\", "\\"))


add("Язык интерфейса: %s\n", "Interface language: %s\n", "Sprache der Oberfläche: %s\n")
add("Язык не найден: %s. Доступны: ru, en, de.", "Language not found: %s. Available: ru, en, de.",
    "Sprache nicht gefunden: %s. Verfügbar: ru, en, de.")
add("Язык интерфейса: %s", "Interface language: %s", "Sprache der Oberfläche: %s")
add("  %s — /lang %s\n", "  %s — /lang %s\n", "  %s — /lang %s\n")
add("язык интерфейса: ru, en, de", "interface language: ru, en, de", "Sprache der Oberfläche: ru, en, de")
add("модель", "model", "Modell")
add("продолжить сессию: --resume [id] (без id — последняя)",
    "resume a session: --resume [id] (without an id — the last one)",
    "Sitzung fortsetzen: --resume [id] (ohne id — die letzte)")

# --- long help texts (whole blocks are one catalog entry each) ---
add('Allan %s — автономный агент в терминале\n\nИспользование:\n  allan [флаги]                 запустить TUI в текущей папке\n  allan key set <provider>      сохранить API-ключ (скрытый ввод, системный keyring)\n  allan key list | rm <provider>\n  allan connect [код]           связать эту машину с KEYTRON Prime (страница выдаёт код)\n  allan serve                   HTTP-воркер для телефона и сайта (обычно в фоне)\n  allan id                      имя, уникальный id и адрес этой машины\n  allan help                    эта справка\n\nФлаги:\n  --model <имя>          модель на этот запуск (например glm-5.2:cloud)\n  --backend <провайдер>  провайдер на этот запуск: ollama, openrouter, opencode, ...\n  --workspace <папка>    рабочая папка агента (по умолчанию текущая)\n  --resume [id]          продолжить последнюю сессию или конкретную по id\n  --no-memory            не читать и не писать память\n  --version              показать версию\n\nПровайдеры:\n  облачные   %s\n  локальные  ollama, llamacpp, lmstudio (без ключа, находятся автоматически)\n\nВнутри TUI: /help — команды, /model — выбрать модель, /api — ключи и эндпоинты,\n/connect — связать машину с KEYTRON Prime.\n\nФайлы:\n  ~/.allan/config.toml   настройки (без ключей)\n  ~/.allan/memory.db     память и сессии\n  ключи                  системный keyring, иначе ~/.allan/secrets.json (0600)\n\nПримеры:\n  allan key set openrouter\n  allan connect                       # печатает ссылку и ждёт код со страницы\n  allan serve --listen 127.0.0.1:8790\n  allan --backend opencode --model deepseek-v4-flash-free\n  allan --workspace ~/git/project --resume\n',
    'Allan %s — autonomous terminal agent\n\nUsage:\n  allan [flags]                  start the TUI in the current folder\n  allan key set <provider>       store an API key (hidden input, system keyring)\n  allan key list | rm <provider>\n  allan connect [code]            pair this machine with KEYTRON Prime (the page issues a code)\n  allan serve                    HTTP worker for the phone and the site (usually in the background)\n  allan id                       name, unique id and address of this machine\n  allan help                     this help\n\nFlags:\n  --model <name>           model for this run (for example glm-5.2:cloud)\n  --backend <provider>     provider for this run: ollama, openrouter, opencode, ...\n  --workspace <folder>     agent working folder (the current one by default)\n  --resume [id]            continue the last session, or a specific one by id\n  --lang <ru|en|de>        interface language\n  --no-memory              do not read or write memory\n  --version                show the version\n\nProviders:\n  cloud   %s\n  local   ollama, llamacpp, lmstudio (no key, found automatically)\n\nInside the TUI: /help — commands, /model — pick a model, /api — keys and endpoints,\n/connect — pair the machine with KEYTRON Prime, /lang — interface language.\n\nFiles:\n  ~/.allan/config.toml   settings (no keys)\n  ~/.allan/memory.db     memory and sessions\n  keys                   system keyring, otherwise ~/.allan/secrets.json (0600)\n\nExamples:\n  allan key set openrouter\n  allan connect                       # prints the link and waits for the code\n  allan serve --listen 127.0.0.1:8790\n  allan --backend opencode --model deepseek-v4-flash-free\n  allan --workspace ~/git/project --resume\n',
    'Allan %s — autonomer Agent im Terminal\n\nVerwendung:\n  allan [Flags]                  TUI im aktuellen Ordner starten\n  allan key set <anbieter>       API-Schlüssel speichern (verdeckte Eingabe, System-Keyring)\n  allan key list | rm <anbieter>\n  allan connect [Code]           Maschine mit KEYTRON Prime verbinden (die Seite gibt einen Code aus)\n  allan serve                    HTTP-Worker für Handy und Webseite (läuft meist im Hintergrund)\n  allan id                       Name, eindeutige ID und Adresse dieser Maschine\n  allan help                     diese Hilfe\n\nFlags:\n  --model <name>           Modell für diesen Lauf (zum Beispiel glm-5.2:cloud)\n  --backend <anbieter>     Anbieter für diesen Lauf: ollama, openrouter, opencode, ...\n  --workspace <ordner>     Arbeitsordner des Agenten (standardmäßig der aktuelle)\n  --resume [id]            letzte Sitzung oder eine bestimmte per id fortsetzen\n  --lang <ru|en|de>        Sprache der Oberfläche\n  --no-memory              Speicher nicht lesen und nicht schreiben\n  --version                Version anzeigen\n\nAnbieter:\n  Cloud   %s\n  lokal   ollama, llamacpp, lmstudio (ohne Schlüssel, automatisch erkannt)\n\nIn der TUI: /help — Befehle, /model — Modell wählen, /api — Schlüssel und Endpunkte,\n/connect — Maschine mit KEYTRON Prime verbinden, /lang — Sprache der Oberfläche.\n\nDateien:\n  ~/.allan/config.toml   Einstellungen (keine Schlüssel)\n  ~/.allan/memory.db     Speicher und Sitzungen\n  Schlüssel              System-Keyring, sonst ~/.allan/secrets.json (0600)\n\nBeispiele:\n  allan key set openrouter\n  allan connect                       # gibt den Link aus und wartet auf den Code\n  allan serve --listen 127.0.0.1:8790\n  allan --backend opencode --model deepseek-v4-flash-free\n  allan --workspace ~/git/project --resume\n')
add('allan key — API-ключи провайдеров (хранятся в системном keyring, не в config.toml)\n\n  allan key set <provider> [base_url]   ввести ключ скрытым вводом (или из stdin)\n  allan key rm <provider>               удалить провайдер и его ключ\n  allan key list                        провайдеры, эндпоинты и наличие ключей\n\nИзвестные провайдеры: %s\nДля своего OpenAI-совместимого эндпоинта укажите base_url.\n',
    'allan key — provider API keys (kept in the system keyring, not in config.toml)\n\n  allan key set <provider> [base_url]   enter a key with hidden input (or from stdin)\n  allan key rm <provider>               remove a provider and its key\n  allan key list                        providers, endpoints and whether a key is set\n\nKnown providers: %s\nFor your own OpenAI-compatible endpoint give a base_url.\n',
    'allan key — API-Schlüssel der Anbieter (liegen im System-Keyring, nicht in config.toml)\n\n  allan key set <anbieter> [base_url]  Schlüssel mit verdeckter Eingabe eingeben (oder aus stdin)\n  allan key rm <anbieter>              Anbieter und Schlüssel entfernen\n  allan key list                       Anbieter, Endpunkte und vorhandene Schlüssel\n\nBekannte Anbieter: %s\nFür einen eigenen OpenAI-kompatiblen Endpunkt base_url angeben.\n')
add('allan serve — HTTP-воркер: отдаёт агента телефону и сайту\n\n  allan serve [флаги]\n\nФлаги:\n  --listen <addr>     адрес слушания (по умолчанию 127.0.0.1:8790)\n  --token <токен>     общий секрет; иначе берётся из connect (keyring) или ALLAN_WORKER_TOKEN\n  --workspace <папка> рабочая папка агента\n  --max-sessions <n>  сколько сессий держать в памяти (по умолчанию 32)\n  --public            слушать на всех интерфейсах (осторожно: только за TLS-прокси)\n\nТокен хранится в системном keyring. Воркер сам никуда не ходит: он только\nотвечает на запросы, поэтому держать его на локальном адресе или на Tailscale\nбезопасно, а вот в интернет — только за nginx с токеном на телефоне.\n\nПримеры:\n  allan serve                                   # только localhost\n  allan serve --listen 100.118.140.15:8790      # Tailscale, для сервера-шлюза\n  ALLAN_WORKER_TOKEN=… allan serve --listen 0.0.0.0:8790   # за обратным прокси\n',
    'allan serve — HTTP worker: serves the agent to the phone and the site\n\n  allan serve [flags]\n\nFlags:\n  --listen <addr>     listen address (127.0.0.1:8790 by default)\n  --token <token>     shared secret; otherwise taken from connect (keyring) or ALLAN_WORKER_TOKEN\n  --workspace <dir>   agent working folder\n  --max-sessions <n>  how many sessions to keep in memory (32 by default)\n  --public            listen on all interfaces (careful: only behind a TLS proxy)\n\nThe token lives in the system keyring. The worker never dials out, it only answers\nrequests, so keeping it on a local address or on Tailscale is safe; putting it on\nthe internet is only safe behind nginx with the token on the phone.\n\nExamples:\n  allan serve                                   # localhost only\n  allan serve --listen 100.118.140.15:8790      # Tailscale, for the gateway server\n  ALLAN_WORKER_TOKEN=… allan serve --listen 0.0.0.0:8790   # behind a reverse proxy\n',
    'allan serve — HTTP-Worker: liefert den Agenten ans Handy und die Webseite\n\n  allan serve [Flags]\n\nFlags:\n  --listen <addr>     Adresse zum Lauschen (standardmäßig 127.0.0.1:8790)\n  --token <token>     gemeinsames Geheimnis; sonst aus connect (Keyring) oder ALLAN_WORKER_TOKEN\n  --workspace <dir>   Arbeitsordner des Agenten\n  --max-sessions <n>  wie viele Sitzungen im Speicher bleiben (standardmäßig 32)\n  --public            auf allen Schnittstellen lauschen (Vorsicht: nur hinter TLS-Proxy)\n\nDer Token liegt im System-Keyring. Der Worker ruft selbst niemanden an, er beantwortet\nnur Anfragen — auf einer lokalen Adresse oder über Tailscale ist das sicher; ins Internet\ndarf er nur hinter nginx, mit dem Token auf dem Handy.\n\nBeispiele:\n  allan serve                                   # nur localhost\n  allan serve --listen 100.118.140.15:8790      # Tailscale, für das Gateway\n  ALLAN_WORKER_TOKEN=… allan serve --listen 0.0.0.0:8790   # hinter Reverse-Proxy\n')
add('allan connect — связать эту машину с KEYTRON Prime\n\n  allan connect              показать страницу и принять код оттуда\n  allan connect <код>        сразу обменять код на токен (для скриптов)\n  allan connect status       что сейчас подключено\n  allan connect url          только ссылка на страницу привязки\n  allan connect reset        забыть сервер и удалить токен\n\nФлаги:\n  --server <url>    адрес KEYTRON Prime (по умолчанию %s, или ALLAN_SERVER_URL)\n  --name <имя>      как подписать этот ПК в приложении (по умолчанию имя хоста)\n  --address <url>   где сайт достучится до воркера (по умолчанию Tailscale-адрес\n                    этой машины и порт 8790)\n\nКак это работает: страница выдаёт одноразовый код после входа на сайт,\nэта команда обменивает код на общий токен и кладёт его в системный keyring.\nДальше телефон и сайт обращаются к агенту через сервер с этим токеном.\n',
    'allan connect — pair this machine with KEYTRON Prime\n\n  allan connect              show the page and take the code from it\n  allan connect <code>       exchange the code for a token right away (for scripts)\n  allan connect status       what is connected right now\n  allan connect url          just the link to the pairing page\n  Allan connect reset        forget the server and delete the token\n\nFlags:\n  --server <url>    KEYTRON Prime address (%s by default, or ALLAN_SERVER_URL)\n  --name <name>     how this PC is labelled in the app (hostname by default)\n  --address <url>   where the site reaches the worker (Tailscale address of this\n                    machine and port 8790 by default)\n\nHow it works: the page issues a one-time code after you sign in, this command\nexchanges the code for a shared token and keeps it in the system keyring. The\nphone and the site then reach the agent through the server with that token.\n',
    'allan connect — diese Maschine mit KEYTRON Prime verbinden\n\n  allan connect              Seite anzeigen und den Code von dort übernehmen\n  allan connect <Code>       Code sofort gegen einen Token tauschen (für Skripte)\n  allan connect status       was gerade verbunden ist\n  allan connect url          nur den Link zur Kopplungsseite\n  allan connect reset        Server vergessen und Token löschen\n\nFlags:\n  --server <url>    Adresse von KEYTRON Prime (standardmäßig %s, oder ALLAN_SERVER_URL)\n  --name <Name>     Bezeichnung dieses PCs in der App (standardmäßig der Hostname)\n  --address <url>   unter dieser Adresse erreicht die Seite den Worker (standardmäßig\n                    die Tailscale-Adresse dieser Maschine, Port 8790)\n\nSo funktioniert es: die Seite gibt nach der Anmeldung einen einmaligen Code aus,\ndieser Befehl tauscht ihn gegen einen gemeinsamen Token und legt ihn im System-Keyring\nab. Danach erreichen Handy und Webseite den Agenten über den Server mit diesem Token.\n')

def go_string(s):
    return '"' + s.replace("\\", "\\\\").replace('"', '\\"').replace("\n", "\\n").replace("\t", "\\t") + '"'


def main():
    files = sorted(glob.glob("internal/tui/*.go")) + sorted(glob.glob("cmd/allan/*.go"))
    sources = {}
    for f in files:
        if f.endswith("_test.go"):
            continue
        src = open(f, encoding="utf-8").read()
        for m in re.finditer(r'"((?:[^"\\\n]|\\.)*[А-Яа-яЁё](?:[^"\\\n]|\\.)*)"', src):
            key = unescape(m.group(1))
            sources.setdefault(key, f)

    missing = [k for k in sources if k not in EN]
    extra = [k for k in EN if k not in sources]
    for k in missing:
        print("MISSING TRANSLATION:", sources[k], "|", repr(k), file=sys.stderr)
    for k in extra:
        print("UNUSED CATALOG ENTRY:", repr(k), file=sys.stderr)

    if missing:
        print(f"\n{len(missing)} untranslated string(s)", file=sys.stderr)
        return 1

    out = []
    out.append("// Code generated by internal/i18n/gen.py; edit the translations there.\n")
    out.append("package i18n\n")
    out.append("// messages holds every user-visible string, keyed by its Russian source text.")
    out.append("var messages = map[string]Msg{")
    for ru in sorted(EN):
        out.append("\t%s: {En: %s, De: %s}," % (go_string(ru), go_string(EN[ru]), go_string(DE[ru])))
    out.append("}\n")
    open("internal/i18n/catalog.go", "w", encoding="utf-8").write("\n".join(out))
    print(f"catalog.go written: {len(EN)} messages")
    return 0


if __name__ == "__main__":
    sys.exit(main())
