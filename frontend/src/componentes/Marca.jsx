/** O logo: um relógio com o arco de marca-texto. */
export default function Marca({ grande = false }) {
  return (
    <span className={`marca ${grande ? 'marca--grande' : ''}`}>
      <svg viewBox="0 0 64 64" aria-hidden="true" className="marca__icone">
        <circle cx="32" cy="34" r="19" fill="none" stroke="var(--trilha)" strokeWidth="7" />
        <path d="M32 15a19 19 0 0 1 18.1 24.9" fill="none" stroke="var(--foco)" strokeWidth="7" strokeLinecap="round" />
        <path d="M32 26v9l6 4" fill="none" stroke="currentColor" strokeWidth="4" strokeLinecap="round" strokeLinejoin="round" />
      </svg>
      <span className="marca__nome">Estuda Junto</span>
    </span>
  )
}
