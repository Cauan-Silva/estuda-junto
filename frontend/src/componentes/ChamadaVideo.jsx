import { forwardRef, useCallback, useEffect, useImperativeHandle, useReducer, useRef, useState } from 'react'
import { api } from '../servicos/api.js'
import { Chamada } from '../servicos/chamada.js'
import Avatar from './Avatar.jsx'
import Icone from './Icone.jsx'

/**
 * Chamada de vídeo da sala. O componente pai repassa os sinais WebRTC que chegam pelo
 * WebSocket chamando ref.current.aoSinal(de, dados).
 */
const ChamadaVideo = forwardRef(function ChamadaVideo({ voce, participantes, limite, enviar, aoMudarEstado }, ref) {
  const chamadaRef = useRef(null)
  const [, redesenhar] = useReducer((n) => n + 1, 0)
  const [entrando, setEntrando] = useState(false)
  const [erro, setErro] = useState(null)
  const naChamada = Boolean(chamadaRef.current)

  useImperativeHandle(ref, () => ({ aoSinal: (de, dados) => chamadaRef.current?.aoSinal(de, dados) }), [])

  const avisarMidia = useCallback(() => {
    const c = chamadaRef.current
    if (c) enviar({ tipo: 'midia', ...c.midia })
  }, [enviar])

  const sair = useCallback(() => {
    chamadaRef.current?.sair()
    chamadaRef.current = null
    enviar({ tipo: 'chamada_sair' })
    redesenhar()
  }, [enviar])

  useEffect(() => aoMudarEstado?.(naChamada), [naChamada, aoMudarEstado])

  // Sai da chamada ao sair da sala.
  useEffect(() => () => chamadaRef.current?.sair(), [])

  // A lista do servidor manda: abre conexão com quem chegou e fecha com quem saiu.
  useEffect(() => {
    const c = chamadaRef.current
    if (!c) return
    const estou = participantes.some((p) => p.id === voce.conexaoId)
    if (estou) c.confirmada = true
    if (!estou && c.confirmada) {
      // O servidor nos tirou (ex.: o WebSocket reconectou com outro id): encerra aqui também.
      c.sair()
      chamadaRef.current = null
      redesenhar()
      return
    }
    c.sincronizar(participantes)
  }, [participantes, voce?.conexaoId])

  async function entrar() {
    setErro(null)
    setEntrando(true)
    try {
      const { iceServers } = await api.rtc()
      const c = new Chamada({
        meuId: voce.conexaoId,
        iceServers,
        enviarSinal: (para, dados) => enviar({ tipo: 'sinal', para, dados }),
        aoMudar: redesenhar,
      })
      c.aoTelaParada = avisarMidia
      const midia = await c.iniciar({ camera: true, microfone: true })
      if (!midia.camera && !midia.microfone) {
        setErro('Sem câmera e microfone: você entra só para ver e ouvir. Para falar, libere o acesso no cadeado da barra de endereço.')
      }
      chamadaRef.current = c
      enviar({ tipo: 'chamada_entrar', ...midia })
      redesenhar()
    } catch (e) {
      setErro(e.message || 'Não foi possível entrar na chamada.')
    } finally {
      setEntrando(false)
    }
  }

  async function alternar(oque) {
    const c = chamadaRef.current
    if (!c) return
    setErro(null)
    try {
      if (oque === 'microfone') await c.alternarMicrofone()
      if (oque === 'camera') await c.alternarCamera()
      if (oque === 'tela') await (c.tela ? c.pararTela() : c.compartilharTela())
      avisarMidia()
      redesenhar()
    } catch (e) {
      setErro(e.message)
    }
  }

  const c = chamadaRef.current
  const cheia = participantes.length >= limite

  if (!c) {
    return (
      <section className="chamada chamada--fora" aria-label="Chamada de vídeo">
        <div>
          <strong>Chamada de vídeo</strong>
          <span>
            {participantes.length
              ? `${participantes.map((p) => p.nome.split(' ')[0]).join(', ')} ${participantes.length === 1 ? 'está' : 'estão'} na chamada`
              : 'Ninguém na chamada. Use nas pausas para tirar dúvidas e mostrar a tela.'}
          </span>
        </div>
        <button type="button" className="botao" onClick={entrar} disabled={entrando || cheia}>
          {entrando ? 'Entrando…' : cheia ? `Cheia (${limite} pessoas)` : 'Entrar na chamada'}
        </button>
        {erro && <p className="erro">{erro}</p>}
      </section>
    )
  }

  const remotos = new Map(c.remotos().map((r) => [r.id, r]))
  const outros = participantes.filter((p) => p.id !== voce.conexaoId)
  const eu = participantes.find((p) => p.id === voce.conexaoId)
  const alguemNaTela = participantes.find((p) => p.tela)
  const midia = c.midia

  return (
    <section className="chamada" aria-label="Chamada de vídeo">
      <div className={`chamada__grade chamada__grade--${Math.min(participantes.length, 6)} ${alguemNaTela ? 'chamada__grade--com-tela' : ''}`}>
        <Quadro
          nome={`${eu?.nome || 'Você'} (você)`}
          usuarioId={voce.usuarioId}
          stream={c.streamLocal}
          video={midia.camera || midia.tela}
          tela={midia.tela}
          microfone={midia.microfone}
          espelhar={!midia.tela}
          mudo
          destaque={alguemNaTela?.id === voce.conexaoId}
        />
        {outros.map((p) => {
          const r = remotos.get(p.id)
          return (
            <Quadro
              key={p.id}
              nome={p.nome}
              usuarioId={p.usuarioId}
              stream={r?.stream}
              video={p.camera || p.tela}
              tela={p.tela}
              microfone={p.microfone}
              conectando={!r || (r.estado !== 'connected' && r.estado !== 'completed')}
              destaque={alguemNaTela?.id === p.id}
            />
          )
        })}
      </div>

      <div className="chamada__controles" role="toolbar" aria-label="Controles da chamada">
        <BotaoMidia ligado={midia.microfone} icone={midia.microfone ? 'microfone' : 'microfone-off'} onClick={() => alternar('microfone')}>
          {midia.microfone ? 'Desligar microfone' : 'Ligar microfone'}
        </BotaoMidia>
        <BotaoMidia ligado={midia.camera} icone={midia.camera ? 'camera' : 'camera-off'} onClick={() => alternar('camera')}>
          {midia.camera ? 'Desligar câmera' : 'Ligar câmera'}
        </BotaoMidia>
        <BotaoMidia ligado={!midia.tela} destaque={midia.tela} icone="tela" onClick={() => alternar('tela')}>
          {midia.tela ? 'Parar de compartilhar' : 'Compartilhar tela'}
        </BotaoMidia>
        <button type="button" className="botao-redondo botao-redondo--sair" onClick={sair}>
          <Icone nome="sair" />
          <span>Sair</span>
        </button>
      </div>
      {erro && <p className="erro">{erro}</p>}
    </section>
  )
})

export default ChamadaVideo

function BotaoMidia({ ligado, destaque, icone, onClick, children }) {
  return (
    <button
      type="button"
      className={`botao-redondo ${ligado ? '' : 'desligado'} ${destaque ? 'ativo' : ''}`}
      onClick={onClick}
      title={children}
    >
      <Icone nome={icone} />
      <span className="sr">{children}</span>
    </button>
  )
}

function Quadro({ nome, usuarioId, stream, video, tela, microfone, espelhar, mudo, conectando, destaque }) {
  const elemento = useRef(null)
  useEffect(() => {
    const el = elemento.current
    if (el && el.srcObject !== stream) el.srcObject = stream || null
  }, [stream])

  return (
    <figure className={`quadro ${destaque ? 'quadro--destaque' : ''} ${tela ? 'quadro--tela' : ''}`}>
      {/* O vídeo fica sempre montado: mesmo com a câmera desligada, o áudio vem por ele. */}
      <video ref={elemento} autoPlay playsInline muted={mudo} className={`${video ? '' : 'escondido'} ${espelhar ? 'espelhado' : ''}`} />
      {!video && (
        <div className="quadro__sem-video">
          <Avatar nome={nome} id={usuarioId} tamanho={64} />
        </div>
      )}
      <figcaption>
        <span>{nome}</span>
        {tela && <span className="selo">tela</span>}
        {!microfone && <span className="selo selo--mudo">sem áudio</span>}
        {conectando && <span className="selo">conectando…</span>}
      </figcaption>
    </figure>
  )
}
