// Shared Discord chrome + state vocabulary for Bonfire bot messages.
// All new work uses this instead of the old brand.jsx FlameGlyph primitives.

// ── State vocabulary ────────────────────────────────────────────────────────
// Consistent color + copy for every server state the bot can be in.
const STATES = {
  running:      { label: 'burning',     color: BF.ember, glow: true,  pulse: false, icon: '●' },
  starting:     { label: 'lighting',    color: BF.spark, glow: true,  pulse: true,  icon: '●' },
  stopping:     { label: 'dying down',  color: BF.ice,   glow: false, pulse: true,  icon: '●' },
  stopped:      { label: 'out',         color: BF.ash,   glow: false, pulse: false, icon: '○' },
  error:        { label: 'trouble',     color: BF.danger,glow: false, pulse: false, icon: '!' },
  unauthorized: { label: 'not yours',   color: BF.ash,   glow: false, pulse: false, icon: '○' },
  not_found:    { label: 'no such fire',color: BF.ash,   glow: false, pulse: false, icon: '○' },
};

// ── Bot avatar (uses the canonical Av_CampfireInk) ─────────────────────────
function BotAvatar({ size = 40 }) {
  return <Av_CampfireInk size={size}/>;
}

// ── Discord message shell ──────────────────────────────────────────────────
// `author`: 'bot' = Bonfire | 'user' = human-sent slash command
// `invoked`: text of the slash command when author='user'
function DiscordMsg({ children, time = 'Today at 21:14', author = 'bot', invoked }) {
  if (author === 'user') {
    return (
      <div style={{
        background: BF.discordBg, padding: '10px 20px 2px 16px',
        fontFamily: BF.fontBody, color: BF.discordText, width: 560,
      }}>
        <div style={{ display: 'flex', gap: 14 }}>
          <div style={{
            width: 40, height: 40, borderRadius: '50%', flexShrink: 0,
            background: '#5865f2', display: 'grid', placeItems: 'center',
            color: '#fff', fontWeight: 600, fontSize: 15,
          }}>o</div>
          <div style={{ flex: 1, minWidth: 0 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
              <span style={{ color: '#fff', fontWeight: 600, fontSize: 15 }}>otso</span>
              <span style={{ color: BF.discordMuted, fontSize: 12 }}>{time}</span>
            </div>
            <div style={{
              display: 'inline-flex', alignItems: 'center', gap: 8,
              background: '#3b3d44', padding: '6px 10px', borderRadius: 6,
              fontFamily: BF.fontBody, fontSize: 14, color: BF.discordMsg,
            }}>
              <BotAvatar size={20}/>
              <span style={{ color: '#a8b5e8' }}>/{invoked}</span>
            </div>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div style={{
      background: BF.discordBg, padding: '10px 20px 10px 16px',
      fontFamily: BF.fontBody, color: BF.discordText, width: 560,
    }}>
      <div style={{ display: 'flex', gap: 14 }}>
        <div style={{ flexShrink: 0 }}>
          <BotAvatar size={40}/>
        </div>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
            <span style={{ color: '#fff', fontWeight: 600, fontSize: 15 }}>Bonfire</span>
            <span style={{
              background: BF.discordAccent, color: '#fff',
              fontSize: 9, fontWeight: 700, padding: '1px 4px', borderRadius: 3,
              letterSpacing: 0.3, textTransform: 'uppercase',
            }}>App</span>
            <span style={{ color: BF.discordMuted, fontSize: 12 }}>{time}</span>
          </div>
          {children}
        </div>
      </div>
    </div>
  );
}

// Ephemeral "only you can see this" chrome — Discord shows a subtle gray banner.
function EphemeralWrap({ children }) {
  return (
    <div style={{ background: BF.discordBg }}>
      {children}
      <div style={{
        padding: '4px 20px 10px 70px',
        fontFamily: BF.fontBody, fontSize: 12, color: BF.discordMuted,
        display: 'flex', alignItems: 'center', gap: 6,
      }}>
        <span style={{ fontSize: 10 }}>👁</span>
        <span>Only you can see this · </span>
        <span style={{ color: '#a8b5e8', cursor: 'pointer' }}>Dismiss message</span>
      </div>
    </div>
  );
}

// ── Status pill, themed to a state ─────────────────────────────────────────
// NOTE: Discord embeds are static — no animation possible. `pulse` here is
// kept ONLY for our own mock UI (slash autocomplete, confirm dialogs); inside
// DiscordMsg we render static dots so the design matches reality.
function BFStatusPill({ state = 'running', size = 'md', animate = false }) {
  const s = STATES[state] || STATES.running;
  const sizing = size === 'lg'
    ? { padding: '6px 12px', fontSize: 13, dot: 8, gap: 8 }
    : { padding: '3px 9px',  fontSize: 11, dot: 6, gap: 6 };
  return (
    <span style={{
      display: 'inline-flex', alignItems: 'center', gap: sizing.gap,
      padding: sizing.padding,
      background: `color-mix(in oklab, ${s.color} 14%, transparent)`,
      border: `1px solid color-mix(in oklab, ${s.color} 45%, transparent)`,
      borderRadius: 999,
      fontFamily: BF.fontMono, fontSize: sizing.fontSize,
      color: s.color, textTransform: 'lowercase',
      letterSpacing: 0.3,
    }}>
      <span style={{
        width: sizing.dot, height: sizing.dot, borderRadius: '50%',
        background: s.color,
        boxShadow: s.glow ? `0 0 6px ${s.color}` : 'none',
        animation: (animate && s.pulse) ? 'bfPulse 1.2s ease-in-out infinite' : 'none',
      }}/>
      {s.label}
    </span>
  );
}

// ── Discord-ish button ─────────────────────────────────────────────────────
function DiscordBtn({ children, variant = 'secondary', onClick }) {
  const base = {
    padding: '6px 14px', borderRadius: 3, border: 'none',
    fontFamily: BF.fontBody, fontSize: 13, fontWeight: 500,
    cursor: 'pointer', letterSpacing: 0.2,
  };
  const styles = {
    primary:   { ...base, background: BF.ember, color: BF.ink },
    secondary: { ...base, background: '#4e5058', color: '#fff' },
    danger:    { ...base, background: 'transparent', color: BF.danger, border: `1px solid ${BF.danger}` },
    link:      { ...base, background: 'transparent', color: '#a8b5e8', padding: '6px 4px' },
  };
  return <button style={styles[variant] || styles.secondary} onClick={onClick}>{children}</button>;
}

function DiscordBtnRow({ children }) {
  return <div style={{ display: 'flex', gap: 8, marginTop: 10, flexWrap: 'wrap' }}>{children}</div>;
}

Object.assign(window, {
  STATES, BotAvatar, DiscordMsg, EphemeralWrap,
  BFStatusPill, DiscordBtn, DiscordBtnRow,
});
