import { describe, expect, it } from 'vitest'
import { initializeHomeState } from './startup'

describe('startup initialization', () => {
  it('启动时刷新首页观看记录与收藏', async () => {
    const events: string[] = []

    await initializeHomeState(async () => { events.push('home') })

    expect(events).toEqual(['home'])
  })
})
