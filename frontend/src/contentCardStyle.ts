export type ContentCardStyle = 'list' | 'grid'

export const contentCardStyleOptions = [
  { value: 'list', label: '列表' },
  { value: 'grid', label: '卡片' },
] as const

export function contentCardStyleLabel(value: ContentCardStyle): string {
  return value === 'grid' ? '卡片' : '列表'
}

export function normalizeContentCardStyle(value: string | null | undefined): ContentCardStyle {
  // Netflix 改版（v0.9.9）起默认是海报卡片；只有明确存过「列表」才用列表。
  return value === 'list' ? 'list' : 'grid'
}
