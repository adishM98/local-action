export default function HealthItem({ icon: Icon, label, ok, error, onClick }) {
  const state = ok == null ? 'pending' : ok ? 'ok' : 'bad'
  const title =
    ok == null
      ? `checking ${label}…`
      : ok
        ? `${label} ready — click to recheck`
        : `${label} not available${error ? `: ${error}` : ''} — click to recheck`
  return (
    <button className={`health__item health__item--${state}`} title={title} onClick={onClick}>
      <span className={`dot dot--${state}`} />
      {Icon && <Icon size={13} />}
      {label}
    </button>
  )
}
