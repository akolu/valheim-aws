// Brand book sections — composable blocks for the Bonfire brand doc.

function BookSection({ kicker, title, children, noPad }) {
  return (
    <section style={{ padding: noPad ? 0 : '80px 72px 40px', maxWidth: 980, position: 'relative' }}>
      {kicker && (
        <div style={{
          fontFamily: BF.fontMono, fontSize: 11, color: BF.ember,
          letterSpacing: 3, textTransform: 'uppercase', marginBottom: 14,
        }}>{kicker}</div>
      )}
      {title && (
        <div style={{
          fontFamily: BF.fontDisplay, fontSize: 64, fontWeight: 500,
          fontStyle: 'italic', color: BF.cream, letterSpacing: -1.8,
          lineHeight: 1.02, marginBottom: 24,
        }}>{title}</div>
      )}
      {children}
    </section>
  );
}

function Prose({ children, size = 17, maxWidth = 620 }) {
  return (
    <div style={{
      fontFamily: BF.fontBody, fontSize: size, color: BF.cream,
      opacity: 0.82, lineHeight: 1.6, maxWidth, textWrap: 'pretty',
      marginBottom: 14,
    }}>{children}</div>
  );
}

function Rule({ mt = 40, mb = 40 }) {
  return <div style={{ height: 1, background: BF.border, margin: `${mt}px 0 ${mb}px`, maxWidth: 900 }}/>;
}

function FieldLabel({ children }) {
  return (
    <div style={{
      fontFamily: BF.fontMono, fontSize: 10, color: BF.dim,
      letterSpacing: 2, textTransform: 'uppercase', marginBottom: 8,
    }}>{children}</div>
  );
}

function DoDont({ doItems = [], dontItems = [] }) {
  return (
    <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 24, maxWidth: 800, marginTop: 24 }}>
      <div style={{ background: '#1f1a15', padding: '20px 22px', borderRadius: 4, borderLeft: `3px solid ${BF.ok}` }}>
        <FieldLabel>Do</FieldLabel>
        <ul style={{ margin: 0, paddingLeft: 18, fontFamily: BF.fontBody, fontSize: 14, color: BF.cream, lineHeight: 1.8 }}>
          {doItems.map((x, i) => <li key={i} style={{ marginBottom: 10 }}>{x}</li>)}
        </ul>
      </div>
      <div style={{ background: '#1f1a15', padding: '20px 22px', borderRadius: 4, borderLeft: `3px solid ${BF.danger}` }}>
        <FieldLabel>Don't</FieldLabel>
        <ul style={{ margin: 0, paddingLeft: 18, fontFamily: BF.fontBody, fontSize: 14, color: BF.cream, lineHeight: 1.8 }}>
          {dontItems.map((x, i) => <li key={i} style={{ marginBottom: 10 }}>{x}</li>)}
        </ul>
      </div>
    </div>
  );
}

Object.assign(window, { BookSection, Prose, Rule, FieldLabel, DoDont });
