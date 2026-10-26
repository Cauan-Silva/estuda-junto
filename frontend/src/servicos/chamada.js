// Chamada de vídeo em malha (WebRTC): cada participante tem uma conexão direta com cada
// um dos outros. O servidor só repassa os sinais (oferta, resposta e candidatos).
//
// Para não haver duas ofertas cruzadas, quem tem o id de conexão "menor" sempre oferta.
// Cada conexão já nasce com um canal de áudio e um de vídeo; ligar a câmera, desligar ou
// trocar pela tela é só trocar a faixa (replaceTrack), sem renegociar.

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
  }

  /** Pede câmera e microfone. Devolve o que realmente ficou ligado (a pessoa pode negar). */
  async iniciar({ camera, microfone }) {
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
    this.avisar()
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
      stream = await navigator.mediaDevices.getDisplayMedia({ video: { frameRate: { max: 15 } }, audio: false })
    } catch {
      return false // a pessoa cancelou a escolha da tela
    }
    this.tela = stream.getVideoTracks()[0]
    this.tela.contentHint = 'detail' // prioriza nitidez (texto, slides, código)
    this.tela.onended = () => this.pararTela() // botão "parar de compartilhar" do navegador
    await this.trocarVideo()
    return true
  }

  async pararTela() {
    if (!this.tela) return
    this.tela.onended = null
    this.tela.stop()
    this.tela = null
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
    for (const id of [...this.pares.keys()]) this.fecharPar(id)
    for (const f of [this.camera, this.microfone, this.tela]) f?.stop()
    this.camera = this.microfone = this.tela = null
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
