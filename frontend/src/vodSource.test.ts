import { describe, expect, it } from 'vitest'
import {
  DEFAULT_CF_VOD_SOURCES,
  isValidCfVodURL,
  parseCfVodDirectory,
  resolveCfVodSources,
  vodSourceKindLabel,
} from './vodSource'

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

describe('点播源目录解析', () => {
  it('接受 { sources: [...] }，只要带名字和 http(s) 地址的项', () => {
    expect(
      parseCfVodDirectory({
        sources: [
          { name: '聚合万能源（44站合一·支持4K）', url: 'https://live.ciweihaha.win/vod.json' },
          { name: 'OrionTV 聚合（38站）', url: 'https://ysp.ciweihaha.win/agg' },
          { name: '亚洲点播', url: 'https://ysp.ciweihaha.win/oktv-asia.json' },
        ],
      }),
    ).toEqual([
      { name: '聚合万能源（44站合一·支持4K）', url: 'https://live.ciweihaha.win/vod.json' },
      { name: 'OrionTV 聚合（38站）', url: 'https://ysp.ciweihaha.win/agg' },
      { name: '亚洲点播', url: 'https://ysp.ciweihaha.win/oktv-asia.json' },
    ])
  })

  it('接受裸数组', () => {
    expect(parseCfVodDirectory([{ name: '甲', url: 'https://a.example/vod.json' }])).toEqual([
      { name: '甲', url: 'https://a.example/vod.json' },
    ])
  })

  it('丢弃缺名字、地址非法、重复地址的项', () => {
    expect(
      parseCfVodDirectory({
        sources: [
          { name: '', url: 'https://a.example/vod.json' },
          { name: '坏地址', url: 'ftp://a.example/vod.json' },
          { name: '甲', url: 'https://a.example/vod.json' },
          { name: '甲又来了', url: 'https://a.example/vod.json' },
          null,
          'not an object',
        ],
      }),
    ).toEqual([{ name: '甲', url: 'https://a.example/vod.json' }])
  })

  it('格式不对返回空数组（不抛错，下游用默认源兜底）', () => {
    expect(parseCfVodDirectory(null)).toEqual([])
    expect(parseCfVodDirectory('hello')).toEqual([])
    expect(parseCfVodDirectory({})).toEqual([])
    expect(parseCfVodDirectory({ sources: 'nope' })).toEqual([])
  })

  it('目录里没有当前正在用的地址时追加“正在用的源”，免得下拉框选不中', () => {
    const dir = [{ name: '甲', url: 'https://a.example/vod.json' }]
    expect(resolveCfVodSources(dir, 'https://b.example/vod.json')).toEqual([
      { name: '甲', url: 'https://a.example/vod.json' },
      { name: '正在用的源', url: 'https://b.example/vod.json' },
    ])
    expect(resolveCfVodSources(dir, 'https://a.example/vod.json')).toEqual(dir)
  })

  it('目录和当前地址都没有时用内置默认源', () => {
    expect(resolveCfVodSources([], '')).toEqual(DEFAULT_CF_VOD_SOURCES)
    expect(DEFAULT_CF_VOD_SOURCES.map((s) => s.url)).toEqual([
      'https://live.ciweihaha.win/vod.json',
      'https://ysp.ciweihaha.win/agg',
      'https://ysp.ciweihaha.win/oktv-asia.json',
    ])
  })

  it('已证实 0 站点的源不显示；没验证出来的保留', () => {
    const dir = [
      { name: '好源', url: 'https://a.example/vod.json', sites: 1 },
      { name: '空源', url: 'https://b.example/agg', sites: 0 },
      { name: '未知源', url: 'https://c.example/vod.json', sites: -1 },
      { name: '没验证', url: 'https://d.example/vod.json' },
    ]
    expect(resolveCfVodSources(dir, '')).toEqual([
      { name: '好源', url: 'https://a.example/vod.json', sites: 1 },
      { name: '未知源', url: 'https://c.example/vod.json', sites: -1 },
      { name: '没验证', url: 'https://d.example/vod.json' },
    ])
  })

  it('当前正在用的地址哪怕在目录里被滤掉，也以“正在用的源”出现', () => {
    const dir = [{ name: '空源', url: 'https://b.example/agg', sites: 0 }]
    expect(resolveCfVodSources(dir, 'https://b.example/agg')).toEqual([
      { name: '正在用的源', url: 'https://b.example/agg' },
    ])
  })

  it('parseCfVodDirectory 透传后端验证出的 sites', () => {
    expect(
      parseCfVodDirectory({
        sources: [
          { name: '好源', url: 'https://a.example/vod.json', sites: 1 },
          { name: '空源', url: 'https://b.example/agg', sites: 0 },
          { name: '未知源', url: 'https://c.example/vod.json', sites: -1 },
          { name: '没验证', url: 'https://d.example/vod.json' },
        ],
      }),
    ).toEqual([
      { name: '好源', url: 'https://a.example/vod.json', sites: 1 },
      { name: '空源', url: 'https://b.example/agg', sites: 0 },
      { name: '未知源', url: 'https://c.example/vod.json', sites: -1 },
      { name: '没验证', url: 'https://d.example/vod.json' },
    ])
  })
})
