// Three embed surfaces for the Bonfire bot:
//   Hero      — broadcast ("otso started valheim, come play"). Big, celebratory.
//   Card      — balanced default. The safe middle.
//   Line      — personal ("/status"). Minimal, inline, mono.
//   Alert     — errors / unauthorized / not_found. Clear stop signal.
//
// All share the same content primitives so state comparison is apples-to-apples.

// ── Primitives ─────────────────────────────────────────────────────────────

function KV({ k, v, mono, accent, size = 'md' }) {
  const km = size === 'sm'
    ? { labelSize: 9, valueSize: 12, gap: 2 }
    : { labelSize: 10, valueSize: 13, gap: 2 };
  return (
    <div>
      <div style={{
        fontFamily: BF.fontMono, fontSize: km.labelSize, letterSpacing: 1,
        color: BF.dim, textTransform: 'uppercase', marginBottom: km.gap,
      }}>{k}</div>
      <div style={{
        fontFamily: mono ? BF.fontMono : BF.fontBody,
        fontSize: km.valueSize, fontWeight: 500,
        color: accent ? BF.ember : BF.cream,
      }}>{v}</div>
    </div>
  );
}

// ── Hero — broadcast / celebratory ─────────────────────────────────────────
// Typographic, no AI art. The wordmark treatment becomes the hero.
function HeroEmbed({ game = 'Valheim', state = 'running', invoker = '@otso', uptime = '1h 42m', address = '13.48.12.34:2456', world = 'Midgard', backup = 'just now', headline, eta, elapsed }) {
  const s = STATES[state];
  const leadline = headline || {
    running:  `${invoker} lit the fire`,
    starting: `${invoker} is lighting the fire`,
    stopping: `banking the fire`,
    stopped:  'the fire is out',
  }[state] || '';

  return (
    <div style={{
      background: `linear-gradient(160deg, ${BF.dark} 0%, ${BF.ink} 100%)`,
      borderRadius: 6, overflow: 'hidden', maxWidth: 500,
      border: `1px solid ${BF.border}`, position: 'relative',
    }}>
      {/* Colored accent strip — Discord's own embed convention */}
      <div style={{ height: 3, background: s.color, opacity: 0.9 }}/>

      {/* Top: large typographic hero */}
      <div style={{ padding: '22px 22px 14px', position: 'relative' }}>
        {/* Soft ember glow at bottom-right, doubles as "fire behind the text" */}
        <div style={{
          position: 'absolute', right: -40, bottom: -60, width: 200, height: 200,
          background: `radial-gradient(circle, ${s.color} 0%, transparent 60%)`,
          opacity: state === 'running' || state === 'starting' ? 0.18 : 0.06,
          pointerEvents: 'none',
        }}/>
        <div style={{
          fontFamily: BF.fontMono, fontSize: 10, color: BF.muted, letterSpacing: 2,
          textTransform: 'uppercase', marginBottom: 6,
        }}>{leadline}</div>

        <div style={{ display: 'flex', alignItems: 'baseline', gap: 14, flexWrap: 'wrap', marginBottom: 4 }}>
          <div style={{
            fontFamily: BF.fontDisplay, fontSize: 44, fontWeight: 500,
            fontStyle: 'italic', color: BF.cream, letterSpacing: -1.5, lineHeight: 1,
          }}>{game}</div>
          <BFStatusPill state={state} size="lg"/>
        </div>
      </div>

      {/* Bottom: info grid — hidden when state makes it irrelevant */}
      {(state === 'running') && (
        <div style={{
          padding: '12px 22px 18px', display: 'grid',
          gridTemplateColumns: '1fr 1fr', rowGap: 14, columnGap: 22,
        }}>
          <KV k="ADDRESS" v={address} mono accent/>
          <KV k="UPTIME"  v={uptime}/>
          <KV k="BACKUP"  v={backup}/>
        </div>
      )}

      {(state === 'starting') && (
        <div style={{
          padding: '12px 22px 18px', fontFamily: BF.fontBody, fontSize: 13,
          color: BF.muted, lineHeight: 1.5,
        }}>
          tending to it now — i'll let you know when it's ready.{' '}
          {elapsed && <>lit{' '}<span style={{ color: BF.cream, fontFamily: BF.fontMono }}>{elapsed}</span>{' '}ago, </>}
          usually takes{' '}
          <span style={{ color: BF.cream, fontFamily: BF.fontMono }}>~2 min</span>.
        </div>
      )}

      {(state === 'stopping') && (
        <div style={{
          padding: '12px 22px 18px', fontFamily: BF.fontBody, fontSize: 13,
          color: BF.muted, lineHeight: 1.5,
        }}>
          saving the world, banking the coals.
        </div>
      )}

      {(state === 'stopped') && (
        <div style={{
          padding: '12px 22px 18px', fontFamily: BF.fontBody, fontSize: 13,
          color: BF.muted, lineHeight: 1.5,
        }}>
          <span style={{ color: BF.cream }}>/{game.toLowerCase()} start</span> to light it again.
          last session ended {backup}.
        </div>
      )}
    </div>
  );
}

// ── Card — balanced default ────────────────────────────────────────────────
function CardEmbed({ game = 'Valheim', state = 'running', uptime = '1h 42m', address = '13.48.12.34:2456', world = 'Midgard', backup = 'just now', invoker = '@otso', eta }) {
  const s = STATES[state];
  return (
    <div style={{
      display: 'flex', background: BF.dark, borderRadius: 4,
      overflow: 'hidden', maxWidth: 480,
    }}>
      <div style={{ width: 4, background: s.color, flexShrink: 0 }}/>
      <div style={{ padding: '14px 16px', flex: 1, position: 'relative' }}>
        <div style={{ display: 'flex', alignItems: 'baseline', gap: 10, marginBottom: 2, flexWrap: 'wrap' }}>
          <div style={{
            fontFamily: BF.fontDisplay, fontSize: 26, fontWeight: 600,
            color: BF.cream, fontStyle: 'italic', letterSpacing: -0.3,
          }}>{game}</div>
          <BFStatusPill state={state} size="lg"/>
        </div>

        {state === 'running' && (
          <>
            <div style={{ fontFamily: BF.fontBody, fontSize: 13, color: BF.muted, marginBottom: 14 }}>
              burning for {uptime} · lit by {invoker}
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: 'auto 1fr', rowGap: 8, columnGap: 14, fontSize: 13 }}>
              <div style={{ color: BF.dim, fontFamily: BF.fontMono, fontSize: 11, letterSpacing: 0.5 }}>ADDRESS</div>
              <div style={{ fontFamily: BF.fontMono, color: BF.ember, fontSize: 13 }}>{address}</div>
              <div style={{ color: BF.dim, fontFamily: BF.fontMono, fontSize: 11, letterSpacing: 0.5 }}>UPTIME</div>
              <div style={{ color: BF.cream, fontFamily: BF.fontBody }}>{uptime}</div>
              <div style={{ color: BF.dim, fontFamily: BF.fontMono, fontSize: 11, letterSpacing: 0.5 }}>BACKUP</div>
              <div style={{ color: BF.cream, fontFamily: BF.fontBody }}>{backup}</div>
            </div>
          </>
        )}
        {state === 'starting' && (
          <div style={{ fontFamily: BF.fontBody, fontSize: 13, color: BF.muted, marginTop: 4, lineHeight: 1.5 }}>
            {invoker} lit it{' '}
            <span style={{ color: BF.cream, fontFamily: BF.fontMono }}>{eta || '~40s'}</span>{' '}
            ago. usually takes{' '}
            <span style={{ color: BF.cream, fontFamily: BF.fontMono }}>~2 min</span>.
          </div>
        )}
        {state === 'stopping' && (
          <div style={{ fontFamily: BF.fontBody, fontSize: 13, color: BF.muted, marginTop: 4, lineHeight: 1.5 }}>
            saving world · backing up.
          </div>
        )}
        {state === 'stopped' && (
          <div style={{ fontFamily: BF.fontBody, fontSize: 13, color: BF.muted, marginTop: 4, lineHeight: 1.5 }}>
            last session ended {backup}.
          </div>
        )}
      </div>
    </div>
  );
}

// ── Line — personal, minimal, inline ───────────────────────────────────────
function LineEmbed({ game = 'valheim', state = 'running', address = '13.48.12.34:2456', uptime = '1h 42m', backup = '0m ago', eta, elapsed }) {
  const s = STATES[state];
  return (
    <div style={{
      fontFamily: BF.fontMono, fontSize: 14, color: BF.discordText,
      padding: '2px 0', display: 'flex', alignItems: 'center', gap: 10,
      flexWrap: 'wrap',
    }}>
      <span style={{ color: s.color, fontWeight: 600 }}>{game}</span>
      <span style={{ color: BF.discordMuted }}>·</span>
      <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
        <span style={{
          width: 7, height: 7, borderRadius: '50%',
          background: s.color,
          boxShadow: s.glow ? `0 0 6px ${s.color}` : 'none',
        }}/>
        <span style={{ color: s.color }}>{s.label}</span>
      </span>
      {state === 'running' && (<>
        <span style={{ color: BF.discordMuted }}>·</span>
        <span>{address}</span>
        <span style={{ color: BF.discordMuted }}>·</span>
        <span style={{ color: BF.discordMuted }}>{uptime} · backup {backup}</span>
      </>)}
      {state === 'starting' && (<>
        <span style={{ color: BF.discordMuted }}>·</span>
        <span style={{ color: BF.discordMuted }}>lit {elapsed || '40s'} ago · ~2 min total</span>
      </>)}
      {state === 'stopping' && (<>
        <span style={{ color: BF.discordMuted }}>·</span>
        <span style={{ color: BF.discordMuted }}>saving world</span>
      </>)}
      {state === 'stopped' && (<>
        <span style={{ color: BF.discordMuted }}>·</span>
        <span style={{ color: BF.discordMuted }}>last burned {backup}</span>
      </>)}
    </div>
  );
}

// ── Alert — errors / unauthorized / not_found ──────────────────────────────
function AlertEmbed({ kind = 'error', title, body, hint }) {
  // Not state-based like the others — this is its own class of message.
  const tone = {
    error:        { color: BF.danger, icon: '◬', headline: title || 'something went sideways' },
    unauthorized: { color: BF.ash,    icon: '◔', headline: title || 'you can\'t tend this fire' },
    not_found:    { color: BF.ash,    icon: '◌', headline: title || 'no such fire' },
  }[kind];
  return (
    <div style={{
      display: 'flex', gap: 12, background: BF.dark, borderRadius: 4,
      maxWidth: 480, overflow: 'hidden',
    }}>
      <div style={{ width: 4, background: tone.color, flexShrink: 0 }}/>
      <div style={{ padding: '12px 16px 14px', flex: 1 }}>
        <div style={{
          display: 'flex', alignItems: 'center', gap: 10, marginBottom: 6,
        }}>
          <span style={{
            fontFamily: BF.fontMono, fontSize: 16, color: tone.color, fontWeight: 600,
          }}>{tone.icon}</span>
          <span style={{
            fontFamily: BF.fontDisplay, fontStyle: 'italic', fontSize: 18,
            fontWeight: 500, color: BF.cream, letterSpacing: -0.2,
          }}>{tone.headline}</span>
        </div>
        {body && (
          <div style={{ fontFamily: BF.fontBody, fontSize: 13, color: BF.discordText, lineHeight: 1.5 }}>
            {body}
          </div>
        )}
        {hint && (
          <div style={{ fontFamily: BF.fontMono, fontSize: 11, color: BF.muted, marginTop: 8, letterSpacing: 0.3 }}>
            {hint}
          </div>
        )}
      </div>
    </div>
  );
}

Object.assign(window, {
  KV, HeroEmbed, CardEmbed, LineEmbed, AlertEmbed,
});
