# Bonfire brand assets

This folder is the visual source of truth for Bonfire.

- **`Bonfire Brand Book.html`** — the full brand book. Open in a browser.
- **`Design Backlog.html`** — parked design questions for future iteration.
- **`canvas/`** — React/JSX source for embed components, tokens, avatars. The brand book renders from these; they double as implementation reference.
  - `tokens.jsx` — `BF` design token object (palette hex codes, type stack, state map)
  - `embeds_v2.jsx` — `HeroEmbed`, `CardEmbed`, `LineEmbed`, `AlertEmbed` — lift field structures and copy verbatim into Go templates
  - `avatars.jsx` — canonical campfire avatar component
  - `discord.jsx`, `book.jsx` — book chrome; ignore when porting
- **`avatar/`** — exported PNGs at 128/256/512/1024. Upload `bonfire-lit-512.png` to Discord as the bot profile.

The structured text reference is at the repo root: **`BRAND.md`**. Read that first.
