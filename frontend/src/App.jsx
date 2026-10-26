import Entrar from './paginas/Entrar.jsx'
import Inicio from './paginas/Inicio.jsx'
import Sala from './paginas/Sala.jsx'
import { useCaminho } from './servicos/rota.jsx'
import { useSessao } from './servicos/sessao.jsx'

export default function App() {
  const caminho = useCaminho()
  const { usuario, carregando } = useSessao()

  if (carregando) return <div className="carregando-tela" aria-busy="true" />
  if (!usuario) return <Entrar />

  const sala = caminho.match(/^\/sala\/([A-Za-z0-9]{6})\/?$/)
  if (sala) return <Sala codigo={sala[1].toUpperCase()} />
  return <Inicio />
}
