# Bonfire brand assets

This folder is the visual source of truth for Bonfire.

- **`BRAND.md`** — structured text reference. Copy this to the repo root (or symlink). Claude Code reads it via `CLAUDE.md`.
- **`Bonfire Brand Book.html`** — the full brand book. Open in a browser.
- **`Design Backlog.html`** — parked design questions for future iteration.
- **`canvas/`** — React/JSX source for embed components, tokens, avatars. The brand book renders from these; they double as implementation reference.
  - `tokens.jsx` — `BF` design token object (palette hex codes, type stack, state map)
  - `embeds_v2.jsx` — `HeroEmbed`, `CardEmbed`, `LineEmbed`, `AlertEmbed` — lift field structures and copy verbatim into Go templates
  - `avatars.jsx` — canonical campfire avatar component
  - `discord.jsx`, `book.jsx` — book chrome; ignore when porting
- **`avatar/`** — exported PNGs at 128/256/512/1024. Upload `bonfire-lit-512.png` to Discord as the bot profile.

Read `BRAND.md` first — it's the distilled text reference covering voice, palette, state lexicon, command descriptions, the registration model, and *what ports to Discord vs. what's aspirational*. The HTML book is the visual complement.
