// Chamada de vídeo em malha (WebRTC): cada participante tem uma conexão direta com cada
// um dos outros. O servidor só repassa os sinais (oferta, resposta e candidatos).
//
// Para não haver duas ofertas cruzadas, quem tem o id de conexão "menor" sempre oferta.
// Cada conexão já nasce com um canal de áudio e um de vídeo; ligar a câmera, desligar ou
// trocar pela tela é só trocar a faixa (replaceTrack), sem renegociar.

// ---- qualidade da tela ----
// A tela sai em até 1080p e 30 fps. A cada 2 s o navegador conta o que está saindo e o
// "automático" desce um degrau quando o processador ou a internet não dão conta (inclusive
// quando o codificador começa a descartar quadros, que o navegador não acusa como limite)
// e sobe de novo quando sobra folga. Como cada pessoa recebe uma cópia, a banda total é
// dividida entre elas.
const NIVEIS_TELA = [
  { fps: 30, escala: 1, bitrate: 4_000_000 }, // 1080p
  { fps: 30, escala: 1.5, bitrate: 2_500_000 }, // 720p
  { fps: 15, escala: 1.5, bitrate: 1_200_000 },
  { fps: 10, escala: 2, bitrate: 600_000 }, // 540p
]
const TOTAL_TELA = 12_000_000
const MINIMO_TELA = 500_000
const ATRASO_TELA_MS = 150 // quem assiste segura um pouco de vídeo para não engasgar
const NOMES_CODEC = { 'video/av1': 'AV1', 'video/vp9': 'VP9', 'video/vp8': 'VP8', 'video/h264': 'H.264' }

/**
 * Codec de vídeo pela placa de vídeo: AV1 e VP9 dão a mesma imagem com bem menos banda que
 * o H.264. Só escolhe um que a placa de vídeo codifica (não pesa no processador); sem
 * nenhum, fica o que a conexão negociou. Todos continuam negociados: só muda o envio.
 */
async function escolherCodec() {
  const mc = navigator.mediaCapabilities
  if (!mc?.encodingInfo || !window.RTCRtpSender?.getCapabilities) return null
  const tem = new Set((RTCRtpSender.getCapabilities('video')?.codecs || []).map((c) => c.mimeType.toLowerCase()))
  for (const mime of ['video/AV1', 'video/VP9', 'video/H264']) {
    if (!tem.has(mime.toLowerCase())) continue
    try {
      const r = await mc.encodingInfo({ type: 'webrtc', video: { contentType: mime, width: 1920, height: 1080, bitrate: 4_000_000, framerate: 30 } })
      if (r.supported && r.powerEfficient) return mime
    } catch {
      // navegador sem esse teste: segue para o próximo
    }
  }
  return null
}

export class Chamada {
  constructor({ meuId, iceServers, enviarSinal, aoMudar }) {
    this.meuId = meuId
    this.iceServers = iceServers
    this.enviarSinal = enviarSinal
    this.aoMudar = aoMudar
    this.pares = new Map()
    this.camera = null // MediaStreamTrack
    this.microfone = null
    this.tela = null
    this.encerrada = false
    this.codec = null // codec de vídeo escolhido pela placa de vídeo (ou null)
    this.auto = null // estado do automático da tela
    this.infoTela = '' // o que a minha tela está mandando (ex.: "1080p · 30 fps · VP9")
    this.telaDe = new Set() // conexões que estão compartilhando a tela agora
    this.infoRecebida = {} // conexão -> o que chega da tela dela
  }

  /** Pede câmera e microfone. Devolve o que realmente ficou ligado (a pessoa pode negar). */
  async iniciar({ camera, microfone }) {
    this.codec = await escolherCodec()
    if (microfone) this.microfone = await pegarFaixa({ audio: { echoCancellation: true, noiseSuppression: true } })
    if (camera) this.camera = await pegarFaixa({ video: { width: { ideal: 640 }, height: { ideal: 360 }, frameRate: { max: 24 } } })
    this.avisar()
    return { camera: Boolean(this.camera), microfone: Boolean(this.microfone) }
  }

  /** O que este navegador está mandando. */
  get midia() {
    return {
      camera: Boolean(this.camera),
      microfone: Boolean(this.microfone?.enabled),
      tela: Boolean(this.tela),
    }
  }

  get streamLocal() {
    const faixa = this.tela || this.camera
    return faixa ? new MediaStream([faixa]) : null
  }

  /** Abre conexão com quem chegou e fecha com quem saiu (lista vinda do servidor). */
  sincronizar(participantes) {
    if (this.encerrada) return
    const ids = new Set(participantes.map((p) => p.id).filter((id) => id !== this.meuId))
    for (const id of ids) {
      if (!this.pares.has(id)) this.criarPar(id)
    }
    for (const id of [...this.pares.keys()]) {
      if (!ids.has(id)) this.fecharPar(id)
    }
    // Só a tela ganha a reserva de vídeo: na câmera, ela atrasaria a imagem em relação à voz.
    this.telaDe = new Set(participantes.filter((p) => p.tela && p.id !== this.meuId).map((p) => p.id))
    for (const par of this.pares.values()) {
      const receptor = par.pc.getTransceivers().find((t) => !t.stopped && t.receiver.track?.kind === 'video')?.receiver
      if (receptor) definirAtraso(receptor, this.telaDe.has(par.id) ? ATRASO_TELA_MS : 0)
      if (!this.telaDe.has(par.id)) delete this.infoRecebida[par.id]
    }
    this.ajustarMedicao()
    this.ajustarQualidade()
    this.avisar()
  }

  criarPar(id) {
    const pc = new RTCPeerConnection({ iceServers: this.iceServers })
    const par = { id, pc, stream: new MediaStream(), ofertante: this.meuId < id, candidatosPendentes: [], estado: 'conectando' }
    this.pares.set(id, par)

    pc.onicecandidate = (e) => {
      if (e.candidate) this.enviarSinal(id, { candidato: e.candidate.toJSON() })
    }
    pc.ontrack = (e) => {
      par.stream = new MediaStream([...par.stream.getTracks().filter((t) => t.kind !== e.track.kind), e.track])
      e.track.onmute = e.track.onunmute = () => this.avisar()
      this.avisar()
    }
    pc.onconnectionstatechange = () => {
      par.estado = pc.connectionState
      if (pc.connectionState === 'connected') this.ajustarQualidade()
      this.avisar()
    }
    pc.oniceconnectionstatechange = () => {
      // A rede mudou (ex.: trocou de Wi-Fi para 4G): quem oferta tenta um caminho novo.
      if (pc.iceConnectionState === 'failed' && par.ofertante) pc.restartIce()
    }
    pc.onnegotiationneeded = async () => {
      if (!par.ofertante) return
      try {
        await pc.setLocalDescription()
        this.enviarSinal(id, { descricao: pc.localDescription })
      } catch (erro) {
        console.warn('Falha ao montar a oferta', erro)
      }
    }

    if (par.ofertante) {
      pc.addTransceiver('audio', { direction: 'sendrecv' }).sender.replaceTrack(this.microfone)
      pc.addTransceiver('video', { direction: 'sendrecv' }).sender.replaceTrack(this.faixaDeVideo())
    }
    return par
  }

  fecharPar(id) {
    const par = this.pares.get(id)
    if (!par) return
    par.pc.close()
    this.pares.delete(id)
    delete this.infoRecebida[id]
  }

  /** Sinal vindo de outra conexão, repassado pelo servidor. */
  async aoSinal(de, dados) {
    if (this.encerrada || !dados) return
    const par = this.pares.get(de) || this.criarPar(de) // o sinal pode chegar antes da lista
    const { pc } = par
    try {
      if (dados.descricao) {
        await pc.setRemoteDescription(dados.descricao)
        if (dados.descricao.type === 'offer') {
          for (const t of pc.getTransceivers()) {
            if (t.stopped) continue
            t.direction = 'sendrecv'
            const tipo = t.receiver.track?.kind
            await t.sender.replaceTrack(tipo === 'audio' ? this.microfone : this.faixaDeVideo())
          }
          await pc.setLocalDescription()
          this.enviarSinal(de, { descricao: pc.localDescription })
        }
        for (const c of par.candidatosPendentes.splice(0)) await pc.addIceCandidate(c).catch(() => {})
        await this.ajustarQualidade() // os codecs acabaram de ser negociados
      } else if (dados.candidato) {
        if (pc.remoteDescription) await pc.addIceCandidate(dados.candidato).catch(() => {})
        else par.candidatosPendentes.push(dados.candidato)
      }
    } catch (erro) {
      console.warn('Sinal WebRTC recusado', erro)
    }
  }

  faixaDeVideo() {
    return this.tela || this.camera || null
  }

  async trocarVideo() {
    const faixa = this.faixaDeVideo()
    await Promise.all(
      [...this.pares.values()].flatMap(({ pc }) =>
        pc
          .getTransceivers()
          .filter((t) => !t.stopped && (t.sender.track?.kind === 'video' || t.receiver.track?.kind === 'video'))
          .map((t) => t.sender.replaceTrack(faixa)),
      ),
    )
    await this.ajustarQualidade()
    this.avisar()
  }

  /** Os envios de vídeo (um por pessoa). */
  enviosDeVideo() {
    return [...this.pares.values()].flatMap(({ pc }) =>
      pc
        .getTransceivers()
        .filter((t) => !t.stopped && (t.sender.track?.kind === 'video' || t.receiver.track?.kind === 'video'))
        .map((t) => t.sender),
    )
  }

  /**
   * Limites de envio do vídeo. A banda é dividida pelas cópias (uma por pessoa), para não
   * estourar o upload de quem manda; a tela pode usar bem mais que a câmera.
   */
  async ajustarQualidade() {
    const envios = this.enviosDeVideo().filter((e) => e.track)
    const copias = Math.max(1, this.pares.size)
    const nivel = this.tela ? NIVEIS_TELA[this.auto?.nivel ?? 0] : null
    for (const envio of envios) {
      try {
        const p = envio.getParameters()
        if (!p.encodings?.length) continue // ainda negociando
        const e = p.encodings[0]
        if (nivel) {
          e.maxBitrate = Math.max(MINIMO_TELA, Math.min(nivel.bitrate, Math.round(TOTAL_TELA / copias)))
          e.maxFramerate = nivel.fps
          e.scaleResolutionDownBy = nivel.escala
          p.degradationPreference = 'maintain-framerate'
        } else {
          e.maxBitrate = Math.max(250_000, Math.min(800_000, Math.round(2_400_000 / copias)))
          e.maxFramerate = 24
          e.scaleResolutionDownBy = 1
          p.degradationPreference = 'balanced'
        }
        await envio.setParameters(p)
      } catch {
        // navegador sem algum desses ajustes: segue com o padrão
      }
      await this.aplicarCodec(envio)
    }
  }

  async aplicarCodec(envio) {
    try {
      const p = envio.getParameters()
      if (!p.encodings?.length || !p.codecs?.length) return
      const quero = this.codec?.toLowerCase()
      const atual = p.encodings[0].codec?.mimeType?.toLowerCase()
      if (quero === atual) return
      const c = quero && p.codecs.find((x) => x.mimeType.toLowerCase() === quero)
      if (quero && !c) return // a outra pessoa não decodifica esse: fica o negociado
      if (c) p.encodings[0].codec = { mimeType: c.mimeType, clockRate: c.clockRate, ...(c.sdpFmtpLine ? { sdpFmtpLine: c.sdpFmtpLine } : {}) }
      else delete p.encodings[0].codec
      await envio.setParameters(p)
    } catch {
      // navegador sem escolha de codec por envio: fica o negociado
    }
  }

  // ---- medição: o automático da minha tela e o que chega das telas dos outros ----

  ajustarMedicao() {
    const precisa = Boolean(this.tela) || this.telaDe.size > 0
    if (precisa && !this.timerMedicao) this.timerMedicao = setInterval(() => this.medir(), 2000)
    if (!precisa && this.timerMedicao) {
      clearInterval(this.timerMedicao)
      this.timerMedicao = null
    }
  }

  async medir() {
    if (this.encerrada) return
    if (this.tela && this.auto) await this.medirTela()
    await this.medirRecepcao()
    this.avisar()
  }

  async medirTela() {
    const a = this.auto
    let pior = null
    for (const envio of this.enviosDeVideo()) {
      if (!envio.track) continue
      const st = await envio.getStats().catch(() => null)
      if (!st) continue
      let s = null
      let banda = null
      let fonte = null
      st.forEach((r) => {
        if (r.type === 'outbound-rtp' && r.kind === 'video')
          s = { fps: r.framesPerSecond || 0, limite: r.qualityLimitationReason || 'none', altura: r.frameHeight || 0, gpu: r.powerEfficientEncoder, codecId: r.codecId }
        if (r.type === 'candidate-pair' && r.availableOutgoingBitrate && (r.nominated || r.state === 'succeeded')) banda = Math.max(banda || 0, r.availableOutgoingBitrate)
        if (r.type === 'media-source' && r.kind === 'video' && r.framesPerSecond != null) fonte = r.framesPerSecond
      })
      if (!s) continue
      s.banda = banda
      s.fonte = fonte
      s.codec = NOMES_CODEC[st.get(s.codecId)?.mimeType?.toLowerCase()] || ''
      if (!pior || (s.limite !== 'none' && pior.limite === 'none') || s.fps < pior.fps - 3) pior = s
    }
    if (!pior || this.auto !== a) return
    a.amostras.push(pior)
    if (a.amostras.length > 6) a.amostras.shift()
    const agora = performance.now()
    const ultimas = (n) => a.amostras.slice(-n)
    const todas = (n, f) => ultimas(n).length === n && ultimas(n).every(f)
    // no começo e logo depois de cada troca o navegador ainda está medindo a internet
    const medindo = agora - a.inicio < 15000 || agora - a.ultimaTroca < 6000
    const precisa = NIVEIS_TELA[a.nivel].bitrate
    const faltaBanda = (s) => s.limite === 'bandwidth' && (s.banda == null || s.banda < precisa * 0.7)
    // captura N quadros mas saem bem menos: o codificador está descartando quadros
    const perdendo = (s) => s.fonte != null && s.fonte >= 12 && s.fps < s.fonte * 0.8 && s.fonte - s.fps >= 4
    const descer = todas(2, (s) => s.limite === 'cpu') || (!medindo && (todas(4, faltaBanda) || todas(3, perdendo)))
    const acima = a.nivel > 0 ? NIVEIS_TELA[a.nivel - 1].bitrate : 0
    const folga = (s) => s.limite !== 'cpu' && !faltaBanda(s) && !perdendo(s) && (s.banda == null || s.banda >= acima * 0.8)
    let novo = a.nivel
    if (descer && a.nivel < NIVEIS_TELA.length - 1 && agora - a.ultimaTroca > 3000) {
      a.bloqueio[a.nivel] = agora + 30000 // não tenta este degrau de novo por 30 s
      novo = a.nivel + 1
    } else if (a.nivel > 0 && todas(4, folga) && (a.bloqueio[a.nivel - 1] || 0) < agora && agora - a.ultimaTroca > 8000) {
      novo = a.nivel - 1
    }
    if (novo !== a.nivel) {
      a.nivel = novo
      a.ultimaTroca = agora
      a.amostras = []
      await this.tela?.applyConstraints({ frameRate: { max: NIVEIS_TELA[novo].fps } }).catch(() => {})
      await this.ajustarQualidade()
    }
    const quem = pior.gpu === true ? 'na placa de vídeo' : pior.gpu === false ? 'no processador' : ''
    const fps = Math.round(pior.fps)
    this.infoTela = [pior.altura ? `${pior.altura}p` : '', `${fps} fps`, [pior.codec, quem].filter(Boolean).join(' ')].filter(Boolean).join(' · ')
  }

  // O que chega de cada tela: resolução, fps, perda de pacotes e travadas.
  async medirRecepcao() {
    for (const par of this.pares.values()) {
      if (!this.telaDe.has(par.id)) continue
      const receptor = par.pc.getTransceivers().find((t) => !t.stopped && t.receiver.track?.kind === 'video')?.receiver
      const st = await receptor?.getStats().catch(() => null)
      if (!st) continue
      let s = null
      st.forEach((x) => {
        if (x.type === 'inbound-rtp' && x.kind === 'video') s = x
      })
      if (!s) continue
      const antes = par.estatisticaAnterior
      par.estatisticaAnterior = s
      const partes = []
      if (s.frameHeight) partes.push(`${s.frameHeight}p`)
      if (s.framesPerSecond != null) partes.push(`${Math.round(s.framesPerSecond)} fps`)
      if (antes) {
        const recebidos = (s.packetsReceived || 0) - (antes.packetsReceived || 0)
        const perdidos = Math.max(0, (s.packetsLost || 0) - (antes.packetsLost || 0))
        const pct = recebidos + perdidos > 0 ? (perdidos / (recebidos + perdidos)) * 100 : 0
        if (pct >= 0.5) partes.push(`perda ${pct < 10 ? pct.toFixed(1) : Math.round(pct)}%`)
        const travadas = (s.freezeCount || 0) - (antes.freezeCount || 0)
        if (travadas > 0) partes.push(travadas === 1 ? '1 travada' : `${travadas} travadas`)
      }
      this.infoRecebida[par.id] = partes.join(' · ')
    }
  }

  async alternarCamera() {
    if (this.camera) {
      this.camera.stop() // desliga a luz da câmera, não só o envio
      this.camera = null
    } else {
      this.camera = await pegarFaixa({ video: { width: { ideal: 640 }, height: { ideal: 360 }, frameRate: { max: 24 } } })
      if (!this.camera) throw new Error('Não foi possível ligar a câmera. Confira a permissão do navegador.')
    }
    await this.trocarVideo()
  }

  async alternarMicrofone() {
    if (!this.microfone) {
      this.microfone = await pegarFaixa({ audio: { echoCancellation: true, noiseSuppression: true } })
      if (!this.microfone) throw new Error('Não foi possível ligar o microfone. Confira a permissão do navegador.')
      for (const { pc } of this.pares.values()) {
        for (const t of pc.getTransceivers()) {
          if (!t.stopped && t.receiver.track?.kind === 'audio') await t.sender.replaceTrack(this.microfone)
        }
      }
    } else {
      this.microfone.enabled = !this.microfone.enabled
    }
    this.avisar()
  }

  async compartilharTela() {
    if (!navigator.mediaDevices?.getDisplayMedia) throw new Error('Este navegador não compartilha a tela (no celular, poucos permitem).')
    let stream
    try {
      stream = await navigator.mediaDevices.getDisplayMedia({
        video: { width: { max: 1920 }, height: { max: 1080 }, frameRate: { ideal: 30, max: 30 } },
        audio: false,
      })
    } catch {
      return false // a pessoa cancelou a escolha da tela
    }
    this.tela = stream.getVideoTracks()[0]
    this.tela.contentHint = 'detail' // texto, slides e código: o codificador cuida das letras
    this.tela.onended = () => this.pararTela() // botão "parar de compartilhar" do navegador
    const agora = performance.now()
    this.auto = { nivel: 0, amostras: [], bloqueio: {}, inicio: agora, ultimaTroca: agora }
    this.infoTela = ''
    this.ajustarMedicao()
    await this.trocarVideo()
    return true
  }

  async pararTela() {
    if (!this.tela) return
    this.tela.onended = null
    this.tela.stop()
    this.tela = null
    this.auto = null
    this.infoTela = ''
    this.ajustarMedicao()
    await this.trocarVideo()
    this.aoTelaParada?.()
  }

  remotos() {
    return [...this.pares.values()].map((p) => ({ id: p.id, stream: p.stream, estado: p.estado }))
  }

  avisar() {
    if (!this.encerrada) this.aoMudar?.()
  }

  sair() {
    this.encerrada = true
    clearInterval(this.timerMedicao)
    this.timerMedicao = null
    for (const id of [...this.pares.keys()]) this.fecharPar(id)
    for (const f of [this.camera, this.microfone, this.tela]) f?.stop()
    this.camera = this.microfone = this.tela = null
  }
}

function definirAtraso(receptor, ms) {
  try {
    if ('jitterBufferTarget' in receptor) receptor.jitterBufferTarget = ms || null
    else if ('playoutDelayHint' in receptor) receptor.playoutDelayHint = ms ? ms / 1000 : null
  } catch {
    // navegador sem esse ajuste
  }
}

async function pegarFaixa(restricoes) {
  try {
    const stream = await navigator.mediaDevices.getUserMedia(restricoes)
    return stream.getTracks()[0] || null
  } catch {
    return null
  }
}
