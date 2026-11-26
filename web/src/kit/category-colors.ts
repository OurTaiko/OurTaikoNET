// Each category's colour, after the in-game genre colours (the hues of
// donder-tool.llx.life), lightened so the ink text on a label passes AA in
// both themes. The chart card's stripe takes its first category's colour;
// unknown categories have none.
const colors: Record<string, string> = {
  pop: '#60a5fa',
  anime: '#f472b6',
  game: '#c084fc',
  vocaloid: '#9ca3af',
  'virtual-singer': '#9ca3af',
  variety: '#4ade80',
  classic: '#fbbf24',
  classics: '#fbbf24',
  kids: '#fde047',
  namco: '#f87171',
  // Fanmade's own.
  touhou: '#fb7185',
  gekichumai: '#22d3ee',
  bemani: '#a3e635',
  'project-sekai': '#2dd4bf',
  'other-rhythm-games': '#818cf8',
  bms: '#94a3b8',
  album: '#e879f9',
  internet: '#fb923c',
}

export function categoryColor(id: string): string | undefined {
  return colors[id]
}

export function categoryStripeColor(ids: string[] = []) {
  return colors[ids.find((id) => id in colors) ?? ''] ?? 'var(--primary-container)'
}
