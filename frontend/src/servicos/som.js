// Aviso sonoro de troca de fase, gerado na hora (sem arquivo de áudio).
let contexto = null

export function tocarAviso(fase) {
  try {
    contexto ??= new (window.AudioContext || window.webkitAudioContext)()
    if (contexto.state === 'suspended') contexto.resume()
    // Foco: duas notas subindo (vamos lá). Pausa: duas notas descendo (pode relaxar).
    const notas = fase === 'foco' ? [523.25, 783.99] : [659.25, 440]
    notas.forEach((freq, i) => {
      const inicio = contexto.currentTime + i * 0.22
      const osc = contexto.createOscillator()
      const volume = contexto.createGain()
      osc.type = 'sine'
      osc.frequency.value = freq
      volume.gain.setValueAtTime(0.0001, inicio)
      volume.gain.exponentialRampToValueAtTime(0.25, inicio + 0.02)
      volume.gain.exponentialRampToValueAtTime(0.0001, inicio + 0.5)
      osc.connect(volume).connect(contexto.destination)
      osc.start(inicio)
      osc.stop(inicio + 0.55)
    })
  } catch {
    // sem áudio neste navegador: segue sem som
  }
}
