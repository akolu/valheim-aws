// Avatar Round — 6 candidates, each at 128 / 40 / 24 for squint test.

function _FlameOrganic({ size = 48, color = BF.ember }) {
  // Symmetric about x=24. Control points on left/right are mirrored.
  return (
    <svg width={size} height={size} viewBox="0 0 48 48" aria-hidden>
      {/* outer flame — symmetric silhouette */}
      <path d="M24 4
               C30 14, 38 18, 38 30
               C38 38, 32 44, 24 44
               C16 44, 10 38, 10 30
               C10 18, 18 14, 24 4 Z"
            fill={color}/>
      {/* inner hot core — symmetric */}
      <path d="M24 16
               C27 22, 30 26, 30 32
               C30 37, 27 41, 24 41
               C21 41, 18 37, 18 32
               C18 26, 21 22, 24 16 Z"
            fill={`color-mix(in oklab, ${color} 50%, #fff4c8)`}/>
    </svg>
  );
}

function _Logs({ size = 48, color = BF.cream }) {
  return (
    <svg width={size} height={size} viewBox="0 0 56 56" aria-hidden>
      <g transform="translate(28 36)">
        <rect x="-22" y="-2" width="44" height="5" rx="2.5" fill={color} transform="rotate(-18)"/>
        <rect x="-22" y="3"  width="44" height="5" rx="2.5" fill={color} transform="rotate(18)"/>
      </g>
    </svg>
  );
}

function _CampfireFull({ size = 128, flameColor = BF.ember, logColor = BF.ash }) {
  // Restored R1 flame shape (slight asymmetry the user liked), centered
  // horizontally under the log crossing. Logs use the original R1 offset
  // (y=-3 / y=4) for the characteristic thick-X look — not stacked-symmetric.
  return (
    <svg width={size} height={size} viewBox="0 0 128 128" aria-hidden>
      {/* flame — R1 path, scaled and translated so it centers at x=64 */}
      <g transform="translate(29 24) scale(1.46)">
        <path d="M24 4 C28 14, 38 18, 38 30 C38 38 32 44 24 44 C16 44 10 38 10 30 C10 22 16 20 18 14 C20 18 22 20 22 14 C22 10 23 7 24 4 Z"
              fill={flameColor}/>
        <path d="M24 16 C26 22, 30 25, 30 32 C30 37 27 41 24 41 C21 41 18 37 18 32 C18 28 21 26 22 22 C23 25 24 25 24 20 C24 18 24 17 24 16 Z"
              fill={`color-mix(in oklab, ${flameColor} 50%, #fff4c8)`}/>
      </g>
      {/* logs — R1 offset rects, cross centered at x=64, y=108 */}
      <g transform="translate(64 108)">
        <rect x="-36" y="-3" width="72" height="7" rx="3.5" fill={logColor} transform="rotate(-18)"/>
        <rect x="-36" y="4"  width="72" height="7" rx="3.5" fill={logColor} transform="rotate(18)"/>
      </g>
    </svg>
  );
}

// Avatar 1 — campfire on ink (CANONICAL: ash logs, no halo)
function Av_CampfireInk({ size = 128 }) {
  return (
    <div style={{ width: size, height: size, background: BF.ink, display: 'grid', placeItems: 'center', borderRadius: '50%', overflow: 'hidden' }}>
      {/* 128 viewBox. Artwork inset so logs stay inside the inscribed circle at every size. */}
      <svg width={size} height={size} viewBox="0 0 128 128" aria-hidden style={{ display: 'block' }}>
        {/* flame — centered at x=64, slightly lifted to clear the logs */}
        <g transform="translate(29 18) scale(1.46)">
          <path d="M24 4 C28 14, 38 18, 38 30 C38 38 32 44 24 44 C16 44 10 38 10 30 C10 22 16 20 18 14 C20 18 22 20 22 14 C22 10 23 7 24 4 Z"
                fill={BF.ember}/>
          <path d="M24 16 C26 22, 30 25, 30 32 C30 37 27 41 24 41 C21 41 18 37 18 32 C18 28 21 26 22 22 C23 25 24 25 24 20 C24 18 24 17 24 16 Z"
                fill={`color-mix(in oklab, ${BF.ember} 50%, #fff4c8)`}/>
        </g>
        {/* logs — cross centered at x=64, y=94; narrowed to fit comfortably inside r=64 circle at 18° tilt */}
        <g transform="translate(64 94)">
          <rect x="-28" y="-3" width="56" height="7" rx="3.5" fill={BF.ash} transform="rotate(-18)"/>
          <rect x="-28" y="4"  width="56" height="7" rx="3.5" fill={BF.ash} transform="rotate(18)"/>
        </g>
      </svg>
    </div>
  );
}

// Avatar 2 — campfire on cream (stands out in dark Discord)
function Av_CampfireCream({ size = 128 }) {
  return (
    <div style={{ width: size, height: size, background: BF.paper, display: 'grid', placeItems: 'center', borderRadius: '50%', overflow: 'hidden' }}>
      <_CampfireFull size={size} flameColor={BF.ember} logColor={BF.ink}/>
    </div>
  );
}

// Avatar 3 — ember dot alone (echoes wordmark's dot)
function Av_Ember({ size = 128 }) {
  return (
    <div style={{ width: size, height: size, background: BF.ink, borderRadius: '50%', overflow: 'hidden', position: 'relative' }}>
      <svg width={size} height={size} viewBox="0 0 128 128">
        <defs>
          <radialGradient id="avEmber" cx="50%" cy="55%" r="42%">
            <stop offset="0%" stopColor="#fff4c8"/>
            <stop offset="30%" stopColor={BF.ember}/>
            <stop offset="100%" stopColor={BF.ink}/>
          </radialGradient>
        </defs>
        <circle cx="64" cy="64" r="42" fill="url(#avEmber)"/>
      </svg>
    </div>
  );
}

// Avatar 4 — campfire inside concentric ring (more logo-like, enclosed)
function Av_CampfireRing({ size = 128 }) {
  return (
    <div style={{ width: size, height: size, background: BF.ink, borderRadius: '50%', overflow: 'hidden', position: 'relative' }}>
      <div style={{ position: 'absolute', inset: '8%', border: `${size*0.012}px solid ${BF.ember}`, borderRadius: '50%', opacity: 0.35 }}/>
      <div style={{ position: 'absolute', inset: 0, display: 'grid', placeItems: 'center' }}>
        <div style={{ transform: 'scale(0.82)' }}>
          <_CampfireFull size={size}/>
        </div>
      </div>
    </div>
  );
}

// Avatar 5 — flame only on ink (no logs — cleanest, tiniest squint still reads)
function Av_FlameOnly({ size = 128 }) {
  return (
    <div style={{ width: size, height: size, background: BF.ink, display: 'grid', placeItems: 'center', borderRadius: '50%', overflow: 'hidden' }}>
      <_FlameOrganic size={size * 0.62} color={BF.ember}/>
    </div>
  );
}

// Avatar 6 — unlit: logs alone (off/asleep variant — server not running)
// Paired with Av1 as the "state-aware" pair.
function Av_Unlit({ size = 128 }) {
  return (
    <div style={{ width: size, height: size, background: BF.ink, display: 'grid', placeItems: 'center', borderRadius: '50%', overflow: 'hidden' }}>
      <svg width={size} height={size} viewBox="0 0 128 128" aria-hidden style={{ display: 'block' }}>
        {/* logs centered on the vertical midline for the unlit state */}
        <g transform="translate(64 72)">
          <rect x="-28" y="-3" width="56" height="7" rx="3.5" fill={BF.ash} transform="rotate(-18)"/>
          <rect x="-28" y="4"  width="56" height="7" rx="3.5" fill={BF.ash} transform="rotate(18)"/>
        </g>
        {/* tiny dim ember above */}
        <circle cx="64" cy="52" r="3" fill={BF.dim}/>
      </svg>
    </div>
  );
}

Object.assign(window, {
  Av_CampfireInk, Av_CampfireCream, Av_Ember, Av_CampfireRing, Av_FlameOnly, Av_Unlit,
});
