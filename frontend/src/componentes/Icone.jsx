// Ícones de traço simples (24×24), herdam a cor do texto.
const CAMINHOS = {
  microfone: 'M12 3a3 3 0 0 0-3 3v6a3 3 0 0 0 6 0V6a3 3 0 0 0-3-3Z M5 11a7 7 0 0 0 14 0 M12 18v3',
  'microfone-off': 'M9 9v3a3 3 0 0 0 5.1 2.1 M15 9.3V6a3 3 0 0 0-5.7-1.3 M5 11a7 7 0 0 0 11.6 5.3 M19 11a7 7 0 0 1-.6 2.8 M12 18v3 M3 3l18 18',
  camera: 'M3 7h11a1 1 0 0 1 1 1v8a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V8a1 1 0 0 1 1-1Z M15 10l6-3v10l-6-3',
  'camera-off': 'M8 7h6a1 1 0 0 1 1 1v6 M15 10l6-3v10l-6-3 M14.5 17H3a1 1 0 0 1-1-1V8a1 1 0 0 1 1-1h1 M3 3l18 18',
  tela: 'M3 4h18v12H3z M8 20h8 M12 16v4 M9 10l3-3 3 3 M12 7v6',
  sair: 'M10 14l-2 2m0 0 2 2m-2-2h6 M3 10c5-4 13-4 18 0l-2 3-3-1v-2c-2.6-.6-5.4-.6-8 0v2l-3 1z',
  copiar: 'M9 9h10v10H9z M5 15V5h10',
  config: 'M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6Z M19.4 13a7.6 7.6 0 0 0 0-2l2-1.5-2-3.4-2.4 1a7.6 7.6 0 0 0-1.7-1L15 3.5h-4l-.4 2.6a7.6 7.6 0 0 0-1.7 1l-2.4-1-2 3.4L6.6 11a7.6 7.6 0 0 0 0 2l-2 1.5 2 3.4 2.4-1a7.6 7.6 0 0 0 1.7 1l.4 2.6h4l.4-2.6a7.6 7.6 0 0 0 1.7-1l2.4 1 2-3.4z',
  voltar: 'M15 18l-6-6 6-6',
}

export default function Icone({ nome, tamanho = 20 }) {
  return (
    <svg width={tamanho} height={tamanho} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d={CAMINHOS[nome]} />
    </svg>
  )
}
