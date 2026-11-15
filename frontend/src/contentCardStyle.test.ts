import { describe, expect, it } from 'vitest'
import { contentCardStyleLabel, contentCardStyleOptions, normalizeContentCardStyle, type ContentCardStyle } from './contentCardStyle'

describe('content card style', () => {
  it('accepts list and grid styles', () => {
    const styles: ContentCardStyle[] = ['list', 'grid']

    expect(styles.map(normalizeContentCardStyle)).toEqual(['list', 'grid'])
  })

  it('falls back to grid for missing or invalid values', () => {
    // Netflix 改版（v0.9.9）起默认是海报卡片；只有明确存过「列表」才用列表。
    expect(normalizeContentCardStyle('')).toBe('grid')
    expect(normalizeContentCardStyle('cards')).toBe('grid')
  })

  it('presents the persisted grid style as the card option', () => {
    expect(contentCardStyleOptions).toEqual([
      { value: 'list', label: '列表' },
      { value: 'grid', label: '卡片' },
    ])
    expect(contentCardStyleLabel('grid')).toBe('卡片')
  })
})
