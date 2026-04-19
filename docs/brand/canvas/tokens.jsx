// Shared design tokens for Bonfire exploration canvas.
// All embeds share the same base type + color system so you can compare apples-to-apples.

const BF = {
  // Core ember palette — warm, grounded. All share chroma 0.12–0.15.
  ink:      '#1a1612',   // deepest — "charcoal"
  dark:     '#2a221b',   // card bg dark
  panel:    '#3a2f25',   // raised panel
  border:   '#4a3d30',   // hairline
  muted:    '#8a7d6d',   // secondary text
  dim:      '#6b5f50',   // tertiary text
  cream:    '#f0e7d6',   // primary text on dark
  paper:    '#e8dfc9',   // off-white / warm paper

  // State colors
  ember:    '#e8793a',   // RUNNING — warm orange
  emberHot: '#f5a862',   // RUNNING glow
  spark:    '#f2c14e',   // STARTING — kindling yellow
  ash:      '#9a8e7d',   // STOPPED — cool neutral
  ice:      '#6b8f9c',   // STOPPING — cooling blue-gray
  ok:       '#8fb369',   // success sage
  danger:   '#c65d4a',   // error / unauthorized

  // Discord-adjacent surface (not Discord's exact chrome, but tuned to sit in it)
  discordBg:    '#2b2d31',
  discordCard:  '#313338',
  discordText:  '#dbdee1',
  discordMuted: '#949ba4',
  discordMsg:   '#f2f3f5',
  discordAccent:'#5865f2', // only for "send" affordance; bot content uses ember palette

  // Type
  fontDisplay: '"Cormorant Garamond", "Iowan Old Style", Georgia, serif',
  fontBody:    '"Inter Tight", -apple-system, "Segoe UI", system-ui, sans-serif',
  fontMono:    '"JetBrains Mono", "SF Mono", ui-monospace, monospace',
};

// Small utility: a striped placeholder rect for where real art would live.
function Placeholder({ w = 120, h = 72, label = 'game art', style = {} }) {
  return (
    <div style={{
      width: w, height: h,
      backgroundImage: 'repeating-linear-gradient(135deg, rgba(255,255,255,0.04) 0 6px, rgba(255,255,255,0.08) 6px 12px)',
      background: 'rgba(255,255,255,0.03)',
      backgroundImage: 'repeating-linear-gradient(135deg, rgba(255,255,255,0.04) 0 6px, transparent 6px 12px)',
      border: `1px dashed ${BF.border}`,
      display: 'flex', alignItems: 'center', justifyContent: 'center',
      fontFamily: BF.fontMono, fontSize: 10, color: BF.muted,
      letterSpacing: 0.5, textTransform: 'uppercase',
      ...style,
    }}>{label}</div>
  );
}

// Status pill — the central metaphor, but keeping copy literal.
function StatusPill({ state = 'running', size = 'md' }) {
  const stateMap = {
    running:  { label: 'running',   color: BF.ember,    dot: BF.emberHot, glow: true },
    starting: { label: 'starting',  color: BF.spark,    dot: BF.spark,    glow: true, pulse: true },
    stopping: { label: 'stopping',  color: BF.ice,      dot: BF.ice,      glow: false, pulse: true },
    stopped:  { label: 'stopped',   color: BF.ash,      dot: BF.ash,      glow: false },
    archived: { label: 'archived',  color: BF.muted,    dot: BF.muted,    glow: false },
    error:    { label: 'error',     color: BF.danger,   dot: BF.danger,   glow: false },
  };
  const s = stateMap[state] || stateMap.running;
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
        background: s.dot,
        boxShadow: s.glow ? `0 0 6px ${s.dot}` : 'none',
        animation: s.pulse ? 'bfPulse 1.2s ease-in-out infinite' : 'none',
      }}/>
      {s.label}
    </span>
  );
}

Object.assign(window, { BF, Placeholder, StatusPill });
