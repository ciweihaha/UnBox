import { describe, expect, it } from 'vitest'
import { isValidCfVodURL, vodSourceKindLabel } from './vodSource'

describe('vod source helpers', () => {
  it('来源中文名：cloudflare 显示 Cloudflare，其余显示本地', () => {
    expect(vodSourceKindLabel('cloudflare')).toBe('Cloudflare')
    expect(vodSourceKindLabel('local')).toBe('本地')
    expect(vodSourceKindLabel('')).toBe('本地')
    expect(vodSourceKindLabel('other')).toBe('本地')
  })

  it('CF 地址只接受 http(s)', () => {
    expect(isValidCfVodURL('https://live.ciweihaha.win/vod.json')).toBe(true)
    expect(isValidCfVodURL('http://127.0.0.1:8080/vod.json')).toBe(true)
    expect(isValidCfVodURL('  https://ysp.ciweihaha.win/agg  ')).toBe(true)
    expect(isValidCfVodURL('')).toBe(false)
    expect(isValidCfVodURL('/tmp/vod.json')).toBe(false)
    expect(isValidCfVodURL('file:///tmp/vod.json')).toBe(false)
    expect(isValidCfVodURL('ftp://example.com/vod.json')).toBe(false)
    expect(isValidCfVodURL('not a url')).toBe(false)
  })
})
