// 主题 → 标题栏纯色：窗口顶部那条原生标题栏跟用户选的背景一个色系，
// 别再白一条。渐变主题取渐变首色（aurora 的 --bg 是 linear-gradient，
// 标题栏只能用纯色）；未知主题回默认深色。
// 与 style.css 里各主题的 --bg 保持一致。
export const TITLE_BAR_COLOURS: Record<string, string> = {
  default: '#141414',
  aurora: '#120324',
  wood: '#E2D5C0',
  ocean: '#4a8aaa',
  '8bit': '#0e0e14',
}

export function titleBarColourForTheme(theme: string): string {
  return TITLE_BAR_COLOURS[theme] ?? TITLE_BAR_COLOURS.default
}
