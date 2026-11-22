import { describe, expect, it } from 'vitest'
// @ts-expect-error Vitest runs in Node; the app intentionally has no Node type dependency.
import { readFileSync } from 'node:fs'

const stylesheet = readFileSync('public/style.css', 'utf8')
const app = readFileSync('src/App.vue', 'utf8')

describe('live layout contract', () => {
  it('keeps a usable minimum width for the live player column', () => {
    expect(stylesheet).toContain('minmax(280px, 1fr)')
    expect(stylesheet).toMatch(/\.player \.playback-view\s*\{[^}]*aspect-ratio:\s*16 \/ 9/s)
  })

  it('uses a breakpoint for a stacked player layout on narrow windows', () => {
    // 2026-11-19 直播改两栏：窄屏上下叠放（播放区在上、频道列表在下），
    // 不再有左边的分类栏。
    expect(stylesheet).toMatch(/@media \(max-width: 900px\)[\s\S]*grid-template-areas:[\s\S]*"player"[\s\S]*"channels"/)
    expect(stylesheet).not.toMatch(/@media \(max-width: 900px\)[\s\S]*grid-template-areas:[\s\S]*"groups"/)
  })
})

describe('vod layout contract', () => {
  it('shares one rhythm between the card grid and the vod panel', () => {
    expect(stylesheet).toMatch(/--content-col:\s*148px;/)
    expect(stylesheet).toMatch(/--content-gap:\s*0\.75rem;/)

    // 卡片列宽、卡片间距、卡片内边距统一走 token，不再散落字面量。
    expect(stylesheet).toMatch(/\.content-card-grid\s*\{[^}]*grid-template-columns:\s*repeat\(auto-fill, minmax\(min\(100%, var\(--content-col\)\), 1fr\)\);/s)
    expect(stylesheet).toMatch(/\.content-card-grid\s*\{[^}]*gap:\s*var\(--content-gap\);/s)
    expect(stylesheet).toMatch(/\.content-card-grid\s*\{[^}]*padding:\s*var\(--content-gap\);/s)

    // 点播页必须重新声明 padding：.vod-main ul { padding: 0 } 的特异性更高，
    // 不重新声明会让卡片贴着面板边框。
    expect(stylesheet).toMatch(/\.vod-main ul\.content-card-grid\s*\{[^}]*padding:\s*var\(--content-gap\);/s)

    // 点播页不再有侧边类目栏：.vod-layout 只是普通块，不再分栏。
    expect(stylesheet).not.toContain('.vod-cats')
    expect(stylesheet).not.toContain('--vod-rail')
    expect(stylesheet).toMatch(/\.vod-layout\s*\{[^}]*flex:\s*1;[^}]*min-height:\s*0;/s)
    expect(stylesheet).not.toMatch(/\.vod-layout\s*\{[^}]*grid-template-columns:/s)

    // 2026-11-19 回归：列表滚不动——.vod-layout 只是普通块、.vod-main
    // 高度由内容撑开，ul 永远拿不到受限高度，overflow-y: auto 触发不了，
    // 60 条卡片全堆在视口外、底部够不着。.vod-layout 改成纵向 flex、
    // .vod-main 占满剩余高度，ul 才有可滚动的高度。
    expect(stylesheet).toMatch(/\.vod-layout\s*\{[^}]*display:\s*flex;/s)
    expect(stylesheet).toMatch(/\.vod-layout\s*\{[^}]*flex-direction:\s*column;/s)
    expect(stylesheet).toMatch(/\.vod-main\s*\{[^}]*flex:\s*1;/s)
    expect(stylesheet).toMatch(/\.vod-main\s*\{[^}]*min-height:\s*0;/s)
  })

  it('keeps a single vod item at the configured card width', () => {
    // auto-fit 折叠掉空列后会把唯一卡片拉伸到整行；auto-fill 保留固定列宽。
    expect(stylesheet).toMatch(/\.vod-main ul\.content-card-grid\s*\{[^}]*grid-template-columns:\s*repeat\(auto-fill, minmax\(min\(100%, var\(--content-col\)\), 1fr\)\);/s)
  })

  it('sizes each poster to the full poster height', () => {
    // 2026-11-14 卡片改版：2:3 由海报框 .content-card-poster 承担，标题移到
    // 海报下方正常排布，卡片本身不再靠 aspect-ratio 定高。海报框仍需
    // overflow: clip：hidden 会把它变成滚动容器，aspect-ratio 就撑不出行高，
    // 1280×800 下海报会被压扁、整列互相压在一起；clip 只裁圆角外的溢出，
    // 不是滚动容器，行高回到海报的真实高度。
    expect(stylesheet).toMatch(/\.content-card-poster\s*\{[^}]*aspect-ratio:\s*2 \/ 3;/s)
    expect(stylesheet).toMatch(/\.content-card-poster\s*\{[^}]*overflow:\s*clip;/s)
    expect(stylesheet).toMatch(/\.content-card-grid \.content-card\s*\{[^}]*overflow:\s*clip;/s)
  })

  it('keeps the title below the poster and the progress badge on it', () => {
    // 2026-11-14 卡片改版：标题不再是压在海报上的蒙层，改在海报下方正常排布；
    // 进度徽标仍叠在海报底部，不再需要给标题蒙层留 2rem 空隙。
    expect(stylesheet).not.toMatch(/\.content-card-title\s*\{[^}]*position:\s*absolute;/s)
    expect(stylesheet).toMatch(/\.content-card-title\s*\{[^}]*margin-top:\s*0\.45rem;/s)
    expect(stylesheet).toMatch(/\.content-card-progress\s*\{[^}]*bottom:\s*0\.35rem;/s)
  })
})

describe('card badge theme contract', () => {
  // 相对亮度（WCAG），用来断言字面量颜色是浅色
  const relLum = (hex: string) => {
    const expand = (h: string) => h.length === 3 ? h.split('').map(c => c + c) : h.match(/.{2}/g)!
    const [r, g, b] = expand(hex).map(c => parseInt(c, 16))
    const f = (c: number) => { c /= 255; return c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4) }
    return 0.2126 * f(r) + 0.7152 * f(g) + 0.0722 * f(b)
  }

  it('keeps card badge text light under the wood theme', () => {
    // 木纹主题把 --text-white 改成深棕 #1a1208（给浅色面板用的强字色）。
    // 徽标底是半透明黑蒙层 rgba(0,0,0,0.58)，深棕字配深底看不清。
    // 必须给徽标单独钉一个浅色字面量，不能继续吃 --text-white。
    const m = stylesheet.match(/\[data-theme="wood"\] \.content-card-badge\s*\{[^}]*color:\s*#([0-9a-fA-F]{3,8});/s)
    expect(m, '缺少 wood 主题 .content-card-badge 颜色覆盖').not.toBeNull()
    expect(relLum(m![1])).toBeGreaterThan(0.5)
  })
})

describe('web player controls contract', () => {
  it('lets every interactive control receive clicks through the overlay', () => {
    // 覆盖层是 pointer-events: none，只给交互元素单独放行。漏掉任何可点元素
    // （曾经的 select、后来的倍速菜单 <li>、音量悬停弹层）都会让点击穿透到
    // <video>，表现为「点控件却触发了播放/暂停」。
    expect(stylesheet).toMatch(/\.player-controls button,\s*\.player-controls input,\s*\.player-controls \.rate-menu,\s*\.player-controls \.volume-menu\s*\{\s*pointer-events:\s*auto;\s*\}/)
    // 菜单项自身不再单独设 pointer-events，靠父级 .rate-menu 的 auto 继承。
    expect(stylesheet).not.toMatch(/\.rate-menu li\s*\{[^}]*pointer-events:/s)
  })

  it('keeps the control bar tight against the bottom edge', () => {
    expect(stylesheet).toMatch(/\.player-bar\s*\{[^}]*bottom:\s*0\.15rem;/s)
    expect(stylesheet).toMatch(/\.player-tools\s*\{[^}]*top:\s*0\.2rem;/s)
  })

  it('keeps the player controls fully transparent (no background band, no blur)', () => {
    expect(stylesheet).toMatch(/\.player-bar\s*\{[^}]*background:\s*transparent;/s)
    // 控件条/工具按钮不得用 backdrop-filter 压住画面（主题面板的模糊不受影响）
    const controlRules = stylesheet.match(/\.player-(?:bar|tools)[^{]*\{[^}]*\}/g) ?? []
    expect(controlRules.length).toBeGreaterThan(0)
    expect(controlRules.join('\n')).not.toContain('backdrop-filter')
  })

  it('renders progress and volume sliders without the opaque native track', () => {
    expect(stylesheet).toMatch(/\.ctrl-seek,\s*\.ctrl-volume\s*\{[^}]*appearance:\s*none;/)
    expect(stylesheet).toMatch(/\.ctrl-seek::-webkit-slider-runnable-track,[\s\S]*?background:\s*rgba\(255, 255, 255, 0\.28\);/)
  })
})

describe('源配置历史滚动契约', () => {
  // 纯 CSS 的 translateX(-40%) 对每一项都生效，短配置也会被滑走。
  // 改为 scrollxEnter/Leave 后按真实溢出量决定是否滚动。
  it('只在配置真正溢出时滚动', () => {
    expect(stylesheet).not.toMatch(/\.src-history-item:hover \.src-ref\s*\{[^}]*animation:/s)
    const refs = app.match(/class="src-ref"[^>]*@mouseenter="scrollxEnter"/g) ?? []
    expect(refs.length).toBe(2) // 点播源历史 + 直播源历史
  })
})

describe('搜索历史下拉框契约', () => {
  // 下拉框与外显历史区同起点同宽度：末尾被裁掉一半的历史词也算在可见宽度里。
  it('与外显历史区同宽', () => {
    expect(app).toContain('ref="searchHistoryInline"')
    expect(app).toMatch(/class="history-dropdown" :style="searchHistoryDropdownStyle"/)
    expect(app).toContain('width: `${box.clientWidth}px`')
  })
})

describe('浮层主题契约', () => {
  // 两个下拉框（搜索历史 / 源配置历史）共用 .history-dropdown，
  // 必须和「关于我们」面板一样吃主题变量与主题浮层规则。
  it('下拉框与关于面板共用面板绘制', () => {
    expect(stylesheet).toMatch(/\.history-dropdown\s*\{[^}]*background:\s*var\(--panel\);/s)
    for (const theme of ['aurora', 'wood', 'ocean']) {
      expect(stylesheet).toContain(`[data-theme="${theme}"] .history-dropdown {`)
    }
    // 8bit 是方角像素风，浮层不能保留圆角
    expect(stylesheet).toMatch(/\[data-theme="8bit"\] \.history-dropdown\s*\{[^}]*border-radius:\s*0;/s)
  })
})

describe('证书一键豁免契约', () => {
  // 证书过期时不让用户复制站点再切设置页：错误条与失效弹窗都要能一键豁免并重试。
  it('错误条与失效弹窗都提供一键豁免', () => {
    expect(app).toContain("import { certErrorHost } from './tlsExempt'")
    expect(app).toMatch(/v-if="certExemptHost"[^>]*@click="exemptCertHost\(certExemptHost\)"/)
    expect(app).toMatch(/v-if="deadCertExemptHost"[^>]*@click="exemptCertHost\(deadCertExemptHost\)"/)
    expect(app).toContain('tlsRetry = reloadVodCategories')
    expect(app).toContain('tlsRetry = () => openVodDetail(item)')
  })

  it('设置页的豁免列表走历史配置同款下拉，添加后自动展开', () => {
    expect(app).toContain('豁免历史')
    expect(app).toContain('收起历史')
    expect(app).toContain('showTLSHistory.value = true')
  })
})

describe('错误展示去重契约', () => {
  // 用户多次反馈同一个错误在上下两处同时出现：原始错误文本只留在底部错误条。
  it('同一个错误不在页面上重复展示', () => {
    expect(app.match(/v-if="errMsg"/g) ?? []).toHaveLength(1)
    expect(app).not.toContain('分类加载失败：{{ vodCategoryError }}')
    expect(app).toMatch(/v-else-if="vodCategoryError"[^>]*>分类加载失败</)
  })
})

describe('错误浮层可读性契约', () => {
  // 错误横幅原来用 12% 半透明红底，背后正文会透出来导致读不清。
  it('错误浮层使用不透明底色', () => {
    expect(stylesheet).toMatch(/\.error-box\s*\{[^}]*background:\s*var\(--error-bg\);/s)
    expect(stylesheet).toMatch(/:root\s*\{[^}]*--error-bg:/s)
    // 木纹是浅色主题，错误浮层要换成浅底深红字
    expect(stylesheet).toMatch(/\[data-theme="wood"\]\s*\{[^}]*--error-bg:/s)
  })
})

describe('点播列表底部契约', () => {
  // 到底后「继续加载」按钮保留但禁用并改文案，分割线移到两个按钮上方。
  it('到底状态保留按钮并把分割线放在按钮上方', () => {
    expect(app).toContain("vodListEnd ? '已加载完'")
    expect(app).toContain(':disabled="vodListLoading || vodListEnd"')
    expect(stylesheet).toMatch(/\.vod-list-footer\s*\{[^}]*flex-direction:\s*column;/s)
    expect(stylesheet).toMatch(/\.vod-list-actions\s*\{[^}]*display:\s*flex;/s)
  })

  // 只有一页或没有内容（从未翻到第 2 页）时不出按钮，只留分割线。
  it('单页或空列表不展示按钮', () => {
    expect(app).toContain('v-if="vodHasMore || vodPage > 1"')
  })

  // 分割线跟着真实滚动位置：到最底才出现，往回划立刻消失；无内容/单页也显示。
  it('分割线只在全部内容滑到最底时出现', () => {
    expect(app).toContain('v-if="vodListEnd" class="vod-list-end" :class="{ visible: vodListAtBottom }"')
    expect(app).toContain('@at-bottom-change="onVodListScroll"')
    expect(app).toContain('vodListRef.value?.isAtBottom()')
    // 用 visibility 占位显形，避免出现/隐藏改变 footer 高度导致闪烁
    expect(stylesheet).toMatch(/\.vod-list-end\s*\{[^}]*visibility:\s*hidden;/s)
    expect(stylesheet).toMatch(/\.vod-list-end\.visible\s*\{[^}]*visibility:\s*visible;/s)
    // 列表只要纵向滚动，避免空列表/单页时多出横向滚动条
    expect(stylesheet).toMatch(/\.vod-main ul\s*\{[^}]*overflow-x:\s*hidden;/s)
  })
})

describe('源导入状态契约', () => {
  // 点播与直播共用一套导入流程，但按钮文案必须各管各的。
  it('点播与直播的导入中状态互不影响', () => {
    expect(app).toContain("importingKind === 'vod' ? '导入中…' : '导入'")
    expect(app).toContain("importingKind === 'live' ? '导入中…' : '导入'")
  })
})

describe('内容列表布局契约', () => {
  // 观看记录 / 收藏 / 点播列表 / 搜索结果四处共用 ContentCardList 与 .content-row，
  // 列表模式统一两行。若又出现按页面分叉的行样式，说明有人绕开了组件。
  it('四处内容列表共用一套行样式', () => {
    expect(stylesheet).toMatch(/\.content-row\s*\{[^}]*cursor:\s*pointer;/s)
    expect(stylesheet).toMatch(/\.content-row-info\s*\{[^}]*flex-direction:\s*column;/s)
    expect(stylesheet).toMatch(/\.content-row-info \.sub\s*\{[^}]*color:\s*var\(--text-dim\);/s)

    // 旧的两套样式已并入 .content-row，不该复活。
    expect(stylesheet).not.toContain('.home-info')
    expect(stylesheet).not.toContain('.home-list li')
    expect(stylesheet).not.toContain('.favorites-list li')
  })

  // 直播频道列表仍走 .channel，别被上面的合并误伤。
  it('直播频道行样式不受内容列表合并影响', () => {
    expect(stylesheet).toMatch(/\.channel\s*\{/)
    expect(stylesheet).toMatch(/\.channel \.group\s*\{/)
  })
})

describe('搜索进度布局契约', () => {
  // 进度文案只允许浮在输入框右端：塞进搜索行会挤压输入框，换行到第二行
  // 会把下面的列表整体顶下去（搜索中内容跳动）。绝对定位两种毛病都没有。
  it('进度浮在输入框内，不参与布局', () => {
    const vodSearchForm = app.match(/<form class="search"[\s\S]*?<\/form>/g)?.find((f: string) => f.includes('vodQuery')) ?? ''
    expect(vodSearchForm).toContain('class="search-field"')
    expect(vodSearchForm).toContain('class="search-inline-progress"')
    expect(vodSearchForm).toContain('with-inline-progress')

    expect(stylesheet).toMatch(/\.search-field\s*\{[^}]*position:\s*relative;/s)
    expect(stylesheet).toMatch(/\.search-inline-progress\s*\{[^}]*position:\s*absolute;[^}]*right:\s*0\.6rem;[^}]*text-overflow:\s*ellipsis;/s)
    // 有进度时给输入区留出右侧空间，打的字不会被提示压住。
    expect(stylesheet).toMatch(/\.search input\.with-inline-progress\s*\{[^}]*padding-right:/s)
  })
})

describe('详情返回恢复现场契约', () => {
  // 之前只按「选中条目居中」恢复，卡片原本靠边时返回就差了半屏（多次反馈的跳动）。
  // 现在离开前记下真实滚动偏移，返回时原样恢复；居中是拿不到偏移时的兜底。
  it('离开前记偏移（在任何请求之前），返回时按偏移恢复', () => {
    const captureAt = app.indexOf('searchScrollTop.value = searchListRef.value?.getScrollTop() ?? 0')
    const fetchAt = app.indexOf('const d = await ShellService.VodDetail(site, item.ID)')
    expect(captureAt).toBeGreaterThan(-1)
    expect(fetchAt).toBeGreaterThan(captureAt)

    expect(app).toContain('searchListRef.value?.restoreScroll(top, index)')
    expect(app).toContain('searchListRef.value?.scrollToIndex(index)')
  })
})

describe('停止播放不阻塞页面切换契约', () => {
  // 从详情返回搜索结果要立刻切页面；StopPlayback 是 mpv 的 IPC 往返，
  // mpv 主循环忙时要等到命令应答超时（15s），await 它会把整页切换卡住几秒。
  it('stopPlayback 不等待后端的暂停应答', () => {
    expect(app).toContain('void ShellService.StopPlayback(token).catch(() => {})')
    const stopFn = app.match(/async function stopPlayback[\s\S]*?\n\}/)
    expect(stopFn).not.toBeNull()
    expect(stopFn![0]).not.toContain('await ShellService.StopPlayback')
  })
})

describe('设置弹窗契约', () => {
  // 2026-11-19 回归：「关于我们」面板直接叠在设置页上面、跟后面的
  // 设置内容挤成一团——.settings-overlay/.settings-panel 根本没有基础
  // 样式，div 按普通块排在文档流里。约定：遮罩 fixed 盖满全屏、半透明
  // 底、面板居中；所有用这两个类的弹窗（关于/免责/删除确认/主题选择）
  // 都吃同一套。
  it('遮罩盖满全屏、面板居中，不再是文档流里的普通块', () => {
    expect(stylesheet).toMatch(/\.settings-overlay\s*\{[^}]*position:\s*fixed;/s)
    expect(stylesheet).toMatch(/\.settings-overlay\s*\{[^}]*inset:\s*0;/s)
    expect(stylesheet).toMatch(/\.settings-overlay\s*\{[^}]*display:\s*flex;/s)
    expect(stylesheet).toMatch(/\.settings-overlay\s*\{[^}]*align-items:\s*center;/s)
    expect(stylesheet).toMatch(/\.settings-overlay\s*\{[^}]*justify-content:\s*center;/s)
    expect(stylesheet).toMatch(/\.settings-overlay\s*\{[^}]*z-index:\s*100;/s)
    // 遮罩要能挡住后面的设置内容（半透明深色底），不是透明。
    expect(stylesheet).toMatch(/\.settings-overlay\s*\{[^}]*background:\s*rgba\(0,\s*0,\s*0,\s*0\.[5-9]/s)
    expect(stylesheet).toMatch(/\.settings-panel\s*\{[^}]*max-width:/s)
    expect(stylesheet).toMatch(/\.settings-panel\s*\{[^}]*background:\s*var\(--panel\);/s)
    expect(app).toContain('v-if="showAbout" class="settings-overlay"')
  })
})

describe('错误提示自动消失契约', () => {
  // 2026-11-19 回归：检测接口失败后底部红条一直挂着，非得手动点 X。
  // 约定：底部错误条与成功提示 5 秒自动消失；带操作按钮的证书错误不自动清。
  it('错误条与成功提示走自动清空，证书豁免除外', () => {
    expect(app).toContain("import { autoClearMessage } from './ephemeralMessage'")
    expect(app).toMatch(/autoClearMessage\(errMsg/)
    expect(app).toMatch(/autoClearMessage\(translateModelMsg/)
    // 证书错误带一键豁免按钮：skip 掉自动清，等用户自己处理。
    expect(app).toMatch(/autoClearMessage\(errMsg[\s\S]*?certExemptHost/)
  })
})

describe('空格键暂停契约', () => {
  // 2026-11-19：播点播和直播时按空格暂停、再按继续；焦点在输入框时不触发；
  // 暂停时画面定住并显示暂停提示。播放器自己的 keydown 只在获焦时生效，
  // 所以空格由 App 的全局监听接管（播放器内不再处理空格，避免按一次变两次）。
  it('App 全局监听空格，输入框内不触发，只在有播放计划时暂停', () => {
    expect(app).toContain("document.addEventListener('keydown', handleGlobalSpaceKey)")
    expect(app).toContain("document.removeEventListener('keydown', handleGlobalSpaceKey)")
    expect(app).toMatch(/function handleGlobalSpaceKey[\s\S]*?tagName === 'INPUT'/)
    expect(app).toMatch(/function handleGlobalSpaceKey[\s\S]*?tagName === 'TEXTAREA'/)
    expect(app).toMatch(/function handleGlobalSpaceKey[\s\S]*?isContentEditable/)
    expect(app).toMatch(/function handleGlobalSpaceKey[\s\S]*?vodPagePlaybackPlan\.value/)
    expect(app).toMatch(/function handleGlobalSpaceKey[\s\S]*?livePagePlaybackPlan\.value/)
    expect(app).toMatch(/function handleGlobalSpaceKey[\s\S]*?togglePlay\(\)/)
    // 播放器容器不再自己处理空格（否则全局监听会让它按一次变两次）。
    const playbackView = readFileSync('src/components/PlaybackView.vue', 'utf8')
    const onKeydown = playbackView.match(/function onKeydown[\s\S]*?\n\}/)?.[0] ?? ''
    expect(onKeydown).not.toContain("case ' ':")
  })

  it('mpv 后端走已有的 ShellService Pause/Resume，不另起一套', () => {
    expect(app).toMatch(/ShellService\.Pause\(\)/)
    expect(app).toMatch(/ShellService\.Resume\(\)/)
  })

  it('暂停时播放器里显示暂停提示', () => {
    const playbackView = readFileSync('src/components/PlaybackView.vue', 'utf8')
    expect(playbackView).toContain('pause-badge')
    expect(playbackView).toContain('已暂停')
  })
})

describe('Cloudflare 点播源下拉框契约', () => {
  // 2026-11-19：设置里 Cloudflare 点播源改成下拉框，选项从目录地址自动拉取；
  // 目录拉不到时用内置默认源，并允许手动填地址；目录地址本身能在设置里改。
  it('设置里是下拉框，不是单个地址输入框', () => {
    expect(app).toContain('cfVodSourceChoice')
    expect(app).toMatch(/ThemedSelect[\s\S]*?cfVodSourceOptions/)
    expect(app).not.toMatch(/Cloudflare 点播源 <input/)
  })

  it('目录地址默认 live.ciweihaha.win/vod-sources.json，可改、可刷新', () => {
    expect(app).toContain('cfVodDirectoryUrl')
    expect(app).toMatch(/cfVodDirectoryUrl[\s\S]*?DEFAULT_CF_VOD_DIRECTORY_URL/)
    expect(app).toContain('refreshCfVodDirectory')
    expect(app).toContain('saveCfVodDirectoryUrl')
    expect(app).toMatch(/ShellService\.SetCfVodDirectoryURL\(/)
    expect(app).toMatch(/ShellService\.FetchCfVodDirectory\(\)/)
  })

  it('目录拉不到时不报错、用默认源兜底，并允许手动填地址', () => {
    expect(app).toMatch(/resolveCfVodSources\(cfVodDirectorySources\.value, cfVodUrl\.value\)/)
    expect(app).toContain('CF_VOD_SOURCE_MANUAL')
    expect(app).toMatch(/cfVodSourceChoice === CF_VOD_SOURCE_MANUAL/)
  })

  it('选了下拉框的源就保存并拉取', () => {
    expect(app).toMatch(/function onCfVodSourceSelect[\s\S]*?saveCfVodUrl\(\)/)
  })
})

describe('详情上下两栏严格等高契约', () => {
  // 2026-11-19：详情页右边简介栏必须和左边播放区完全一样高，顶对齐、底对齐，
  // 不是大概齐平。实现：上半区 grid 两列拉伸对齐，信息栏不再把行撑高，
  // 内容超高时在栏内滚动。
  it('上半区两列拉伸对齐，信息栏受播放区高度约束、超高时栏内滚动', () => {
    expect(stylesheet).toMatch(/\.vod-detail-top\s*\{[^}]*align-items:\s*stretch;/s)
    expect(stylesheet).toMatch(/\.vod-detail-info\s*\{[^}]*min-height:\s*0;/s)
    expect(stylesheet).toMatch(/\.vod-detail-info\s*\{[^}]*overflow-y:\s*auto;/s)
  })
})

describe('更多类似内容收窄契约', () => {
  // 2026-11-19：类似内容那一栏不要搞那么高，卡片收窄、行距收紧。
  it('类似内容卡片比首页行小一圈、行内边距收紧', () => {
    // 卡片宽是钉死的 120px !important（第六轮：grid 拉伸会忽略普通 width，
    // 实测卡片曾被撑到 151px）；断言放宽到允许 !important，别把钉死当违规。
    expect(stylesheet).toMatch(/\.vod-similar \.netflix-row \.content-card\s*\{[^}]*width:\s*120px(\s*!important)?;/s)
    expect(stylesheet).toMatch(/\.vod-similar \.netflix-row\.content-card-grid\s*\{[^}]*gap:\s*0\.4rem;/s)
  })
})

describe('直播页铺满屏幕高度契约', () => {
  // 2026-11-19：直播页太窄、播放区下面空一大块。宽屏下单行网格占满容器高度，
  // 播放区拉满、左右两栏跟着拉满到底部，不留空渐变区。窄屏仍上下叠放滚动。
  it('宽屏下直播网格单行占满高度，播放器随列拉伸', () => {
    expect(stylesheet).toMatch(/@media \(min-width: 901px\)[\s\S]*?\.layout\.live\s*\{[^}]*grid-template-rows:\s*minmax\(0, 1fr\);/s)
    expect(stylesheet).toMatch(/@media \(min-width: 901px\)[\s\S]*?\.layout\.live \.player\s*\{[^}]*display:\s*flex;/s)
    expect(stylesheet).toMatch(/@media \(min-width: 901px\)[\s\S]*?\.layout\.live \.player \.playback-view\s*\{[^}]*flex:\s*1 1 auto;/s)
    expect(stylesheet).toMatch(/@media \(min-width: 901px\)[\s\S]*?\.layout\.live \.player \.playback-view\s*\{[^}]*aspect-ratio:\s*auto;/s)
  })
})

describe('点播页站点下拉框取消契约', () => {
  // 2026-11-19：首页（点播页）顶部不再显示源下拉框（聚合万能源…），只显示内容；
  // 源切换留在设置里。线路下拉框保留（那是同一站点内的线路，不是源）。
  it('点播工具栏没有站点下拉框，只留线路下拉框', () => {
    expect(app).not.toContain('aria-label="站点"')
    expect(app).toContain('aria-label="线路"')
    expect(app).not.toContain('vodSiteOptions')
    expect(app).not.toContain('selectSite')
  })
})
