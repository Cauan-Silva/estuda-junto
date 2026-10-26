import { Link } from '../servicos/rota.jsx'
import { useSessao } from '../servicos/sessao.jsx'
import Avatar from './Avatar.jsx'
import Marca from './Marca.jsx'

export default function Cabecalho({ children }) {
  const { usuario, sair } = useSessao()
  return (
    <header className="cabecalho">
      <Link para="/" className="cabecalho__marca" aria-label="Estuda Junto, voltar ao início">
        <Marca />
      </Link>
      <div className="cabecalho__meio">{children}</div>
      <div className="cabecalho__conta">
        <Avatar nome={usuario.nome} id={usuario.id} />
        <span className="cabecalho__nome">{usuario.nome}</span>
        <button type="button" className="botao-link" onClick={sair}>
          Sair
        </button>
      </div>
    </header>
  )
}
