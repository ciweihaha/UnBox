<script setup lang="ts">
import { ref, computed, nextTick, onMounted, onBeforeUnmount, watch } from 'vue'
import { Events, Browser } from '@wailsio/runtime'
import { ShellService, type SourceInfo, type Section, type VodItem, type VodListPage, type EpisodeInfo, type VodMedia, type SourceRecord, type VodHistoryInfo, type VodFavoriteInfo, type UpdateInfo, type VodSourceStatus } from '../bindings/github.com/unbox/unbox/internal/shell'
import PlaybackView, { type PlaybackPlan, type PlaybackState } from './components/PlaybackView.vue'
import PreloadView from './components/PreloadView.vue'
import VodDetailHeader from './components/VodDetailHeader.vue'
import { clampEpisodePage, episodePageIndex, episodePageRanges, paginateEpisodes } from './episodes'
import ContentCardList from './components/ContentCardList.vue'
import ThemedSelect from './components/ThemedSelect.vue'
import VodFilterPanel from './components/VodFilterPanel.vue'
import { contentCardStyleLabel, contentCardStyleOptions, normalizeContentCardStyle, type ContentCardStyle } from './contentCardStyle'
import { contentCardSubtitle, historyCardItem, posterGradient, type ContentCardItem } from './contentCardItem'
import { imgError, scrollxEnter, scrollxLeave } from './contentListDom'
import { createPlaybackSettings, PLAYBACK_SETTING_ITEMS, type PlaybackSettingKey, type PlaybackSettingsApi } from './playbackSettings'
import { createSubtitleTranslator, type TranslateSubtitleApi } from './subtitleTranslate'
import { createTranslateSettings, type FreeTranslateProvider, type TranslateProvider, type TranslateSettingsApi } from './translateSettings'
import { createLiveSubtitleSettings, liveSubtitleStatusText, type LiveSubtitleSettingsApi } from './liveSubtitleSettings'
import { VodAutomation, nextEpisodeInSource, sameNameEpisodeOnSource, type VodAutomationHost } from './playbackAutomation'
import { resolveSkipAction, type SkipRuntime, type VodSkipMarks } from './vodSkip'
import { initializeHomeState } from './startup'
import { isMpvUnavailableError, mpvInstallHint, playbackPlanForMode, resolvePlaybackFallback, shouldPauseStalePlayback, shouldRecordVodProgress, shouldShowMpvInstallPrompt, type ActivePlaybackSession, type PlaybackScope, type PlaybackStatus } from './playbackScope'
import { createVodSearchCache, isCurrentVodCategoryRequest, isVodSearchCacheValid, nextVodCategoryRequest, nextVodSearchRequest, pickResumeSeek, removeVodFavorite, removeVodHistory, removeVodSearchHistory, resolveVodSelection, resumeSeekFor, shouldShowVodNoResults, upsertVodSearchHistory, vodBackTarget, vodResumeView, vodSearchQueryForReturn, type VodDetailOrigin, type VodSearchCache, type VodView } from './vodNavigation'
import { buildVodSourceDeadInfo, vodSourceDeadBackLabel, vodSourceDeadBackTarget, type VodSourceDeadInfo } from './vodSourceDead'
import { appendVodItems, hasNextVodPage, nextVodPage, VOD_LIST_PREFETCH_PAGES } from './vodPagination'
import { countLeadingStrong, sortVodSearchResults } from './vodSearchRelevance'
import { appliedFilter, availableFilterGroups, hasActiveFilter, initialSelections } from './vodFilters'
import { certErrorHost } from './tlsExempt'
import { TOP_NAV_CATEGORIES, topNavKeyForCategory, type TopNavCategory } from './topNav'
import { CF_VOD_SOURCE_MANUAL, DEFAULT_CF_VOD_DIRECTORY_URL, parseCfVodDirectory, resolveCfVodSources, isValidCfVodURL, vodSourceKindLabel, type CfVodDirectorySource } from './vodSource'
import { titleBarColourForTheme } from './titleBarTheme'
import { autoClearMessage } from './ephemeralMessage'
import DOMPurify from 'dompurify'


interface ChannelInfo { ID: string; Name: string; Group: string; Logo: string; Favorited: boolean }
interface Progress { Stage: string; Message: string; Done: number; Total: number }


const platform = ref('…')
const playerReady = ref(false)
const groups = ref<string[]>([])
const channels = ref<ChannelInfo[]>([])
const favorites = ref<ChannelInfo[]>([])
const activeGroup = ref('*')
// 收藏视图开关：开着时频道列表只显示已收藏频道；分类下拉框仍显示当前分组，
// 不再像以前那样把 activeGroup 改成 '__fav__'（下拉框里没有这一项，会显示错）。
const liveFavOnly = ref(false)
// 直播分类下拉框选项：全部 + 各分组。
const liveGroupOptions = computed(() => [
  { name: '全部', value: '*' },
  ...groups.value.filter((g) => g !== '*').map((g) => ({ name: g, value: g })),
])
const query = ref('')
const liveNowPlaying = ref('')
// 直播切频道的瞬态 toast：取代播放器上方「正在切换频道…」常驻文案。
const liveToast = ref('')
let liveToastTimer: ReturnType<typeof setTimeout> | null = null
function flashLiveToast(message: string): void {
  liveToast.value = message
  if (liveToastTimer !== null) clearTimeout(liveToastTimer)
  liveToastTimer = setTimeout(() => { liveToast.value = '' }, 2000)
}
const vodNowPlaying = ref('')
const importSummary = ref('')
const errMsg = ref('')
const importProgress = ref<Progress | null>(null)
const importingKind = ref<'' | 'vod' | 'live'>('')
const importing = computed(() => importingKind.value !== '')
const mode = ref<'home' | 'vod' | 'live' | 'search' | 'favorites' | 'settings'>('home')
const sources = ref<SourceInfo[]>([])
const activeSite = ref('')
const activeLine = ref('')
const activeSource = ref('')
const detailSite = ref('')
const vodCategories = ref<Section[]>([])
const vodActiveCat = ref('')
// 点播筛选：draft 是 chip 行当前的选择；active 是已应用、当前列表正在使用的。
// chip 点选即应用，draft 与 active 始终同步（旧版弹层「确定」才提交的流程已取消）。
const vodFilterDraft = ref<Record<string, string>>({})
const vodFilterActive = ref<Record<string, string> | null>(null)
const vodListItems = ref<VodItem[]>([])
const vodSearchItems = ref<VodItem[]>([])
// 搜索结果列表的组件实例引用：详情页返回时用它恢复上次选中的滚动位置。
const searchListRef = ref<InstanceType<typeof ContentCardList> | null>(null)
// 进详情前记住选中的条目标号；返回搜索结果页时 scrollToIndex 定位到它。
const searchSelectedIndex = ref(-1)
// 同时记住列表的滚动偏移：按条目居中定位会和离开时的现场差半屏，
// 恢复偏移才是「原样回去」。
const searchScrollTop = ref(0)
// 渐进搜索结果按小窗口合并渲染（见 flushPendingSearchResults），避免逐事件重渲染。
let searchResultPending: VodItem[] = []
let searchResultTimer: ReturnType<typeof setTimeout> | null = null
const vodDetail = ref<VodMedia | null>(null)
const vodView = ref<VodView>('list')
const vodDetailOrigin = ref<VodDetailOrigin>('list')
const vodSearchCache = ref<VodSearchCache<VodItem> | null>(null)
const episodePage = ref(0)
const currentEpisodeID = ref('')
const episodePagination = ref<HTMLElement | null>(null)
const vodQuery = ref('')
const vodSearchHistory = ref<string[]>([])
const searchHistoryExpanded = ref(false)
const searchHistoryInline = ref<HTMLElement | null>(null)
const searchHistoryDropdownStyle = ref<Record<string, string>>({})
const vodFavorites = ref<VodFavoriteInfo[]>([])
const vodFavorited = ref(false)
const vodPage = ref(0)
const vodHasMore = ref(false)
const vodListEnd = ref(false)
// 列表是否真实滑到了最底：分割线只有「全部内容划到底」后才出现，往上划就消失。
const vodListAtBottom = ref(false)
const vodListLoading = ref(false)
const vodListRef = ref<InstanceType<typeof ContentCardList> | null>(null)
let vodCategoryRequest = 0
let vodListRequest = 0
const vodCategoryLoading = ref(false)
const vodCategoryError = ref('')
const vodListError = ref('')
const livePlaybackPlan = ref<PlaybackPlan | null>(null)
const vodPlaybackPlan = ref<PlaybackPlan | null>(null)
const playbackOwner = ref<'live' | 'vod' | null>(null)
const activePlayback = ref<ActivePlaybackSession | null>(null)
const livePlaybackToken = ref(0)
const vodPlaybackToken = ref(0)
const livePlaybackStatus = ref<PlaybackStatus>('idle')
const vodPlaybackStatus = ref<PlaybackStatus>('idle')
const livePlaybackError = ref('')
const vodPlaybackError = ref('')
// 当前点播进度（秒）：换源续播与预载判断都用它，Web 由 progress 事件更新、mpv 由 playback:event 更新。
const vodPlaybackPosition = ref(0)
// 起播加载中：从发起播放到画面真正出现（首个 playing 事件）之间为真。
const vodPlayerLoading = ref(false)
// 跳过片头片尾：标记按「站点+影片」持久化，同剧各集共用；runtime 记录本集已跳过的动作。
const vodSkipMarks = ref<VodSkipMarks>({ IntroEnd: 0, OutroStart: 0 })
const vodPlaybackDuration = ref(0)
const vodSkipNotice = ref('')
let vodSkipRuntime: SkipRuntime = { introSkipped: false, outroSkipped: false }
let skipNoticeTimer: ReturnType<typeof setTimeout> | null = null
const preloadPlan = ref<PlaybackPlan | null>(null)
let preloadGeneration = 0
let nextPlaybackToken = 0
const mpvReady = ref(false)
const mpvNeeded = ref(false)
const mpvInstallMode = ref('')
const installMessage = ref('')
const vodSources = ref<SourceRecord[]>([])
const liveSources = ref<SourceRecord[]>([])
const insecureTLSHosts = ref<string[]>([])
const insecureTLSInput = ref('')
const insecureTLSError = ref('')
const showTLSHistory = ref(false)
const vodSourceUrl = ref('')
const liveSourceUrl = ref('')
const showVodHistory = ref(false)
const showLiveHistory = ref(false)
const homeHistory = ref<VodHistoryInfo[]>([])
const currentVod = ref<{ site: string; vodID: string } | null>(null)
const pendingSeek = ref(0)
// 延迟续播目标：进详情时记下上次看的集数与进度，等用户点播放才套用。
const vodResume = ref<{ EpID: string; Progress: number } | null>(null)
// 详情加载中标题：详情请求还在跑时非空，底部提示条显示「正在加载《…》…」，
// 避免 15 秒超时期间页面看起来像卡死。同时兼作并发入口闸门。
const vodDetailLoading = ref('')
// 点播源失效弹窗：详情请求失败（站点连不上 / 超时）时弹出，给出换站搜索的出口。
const vodSourceDead = ref<{ info: VodSourceDeadInfo; origin: VodDetailOrigin } | null>(null)

// ---- 证书过期的一键豁免 ----
// 错误里带 x509 且该主机还没在豁免名单里时，在底部错误条 / 失效弹窗上给一键按钮，
// 免去「复制站点 → 切到设置页 → 粘贴 → 回点播页重试」的长链路。
let tlsRetry: (() => Promise<void>) | null = null
const certExemptHost = computed(() => {
  const host = certErrorHost(errMsg.value)
  return host && !insecureTLSHosts.value.includes(host) ? host : ''
})
const deadCertExemptHost = computed(() => {
  const host = certErrorHost(vodSourceDead.value?.info.message ?? '')
  return host && !insecureTLSHosts.value.includes(host) ? host : ''
})

// 底部错误条与成功提示 5 秒自动消失，不用用户手动点 X。
// 带一键豁免按钮的证书错误不自动清：用户还没点按钮就消失，等于把修复入口藏起来了。
autoClearMessage(errMsg, 5000, () => certExemptHost.value !== '')

// exemptCertHost 记住是哪台主机就把哪台加进豁免名单，然后重跑刚才失败的操作。
async function exemptCertHost(host: string) {
  if (!host) return
  try {
    insecureTLSHosts.value = (await ShellService.SetInsecureTLSHost(host, true)) ?? []
    errMsg.value = ''
    vodSourceDead.value = null
    const retry = tlsRetry
    tlsRetry = null
    if (retry) await retry()
  } catch (e) {
    handleError(e)
  }
}

const searchProgress = ref<Progress | null>(null)
const searchThreads = ref(1)
const searchCacheTTL = ref(30)
const showThreads = ref(false)
const showCacheTTL = ref(false)
const showContentCardStyle = ref(false)
const showTheme = ref(false)
const searching = ref(false)
const searchRequest = ref(0)
const activeSearchQuery = ref('')
const completedSearchQuery = ref('')
const activeSearchID = ref(0)
const searchFloorID = ref(0)
const updateInfo = ref<UpdateInfo | null>(null)
const updateMsg = ref('')
const currentVersion = ref('')
const internalVersion = ref('')
const showAbout = ref(false)
const showDisclaimer = ref(false)
// 详情简介展开态：默认限 3 行，点「展开简介」放开。
const descExpanded = ref(false)
// 换一部剧时简介回到默认的 3 行收起态。
watch(vodDetail, () => { descExpanded.value = false })

const themeOptions = [
  { id: 'default', label: 'Netflix 风格' },
  { id: 'aurora', label: '多彩渐变' },
  { id: 'wood', label: '木纹' },
  { id: 'ocean', label: '海洋' },
  { id: '8bit', label: '8bit' },
]
const currentTheme = ref('default')
const contentCardStyle = ref<ContentCardStyle>('grid')

// 播放设置：设置页展示的三个开关，点播会话协调器后续读取同一份运行时状态。
const playbackSettingsStore = createPlaybackSettings(
  ShellService as unknown as PlaybackSettingsApi,
  (e) => handleError(e),
)
const playbackSettings = playbackSettingsStore.settings

function togglePlaybackSetting(key: PlaybackSettingKey, value: boolean) {
  void playbackSettingsStore.set(key, value)
}

// 字幕翻译：英译中，默认关；只走免费接口（微软 Azure / Cloudflare）。
// 失败时显示原文、不挡播放；两个播放器共用同一个翻译器（缓存共享）。
const translateSettingsStore = createTranslateSettings(
  ShellService as unknown as TranslateSettingsApi,
  (e) => handleError(e),
)
const translateSettings = translateSettingsStore.settings
const subtitleTranslator = createSubtitleTranslator(
  ShellService as unknown as TranslateSubtitleApi,
)
function translateSubtitleText(text: string): Promise<string> {
  return subtitleTranslator.translate(text)
}
function setTranslateProvider(provider: TranslateProvider) {
  void translateSettingsStore.setProvider(provider)
}
function setTranslateFreeProvider(provider: FreeTranslateProvider) {
  void translateSettingsStore.setFreeProvider(provider)
}
const translateFreeAPIKey = computed({
  get: () => translateSettings.value.FreeAPIKey,
  set: (v: string) => { void translateSettingsStore.setFreeAPIKey(v) },
})
const translateAzureRegion = computed({
  get: () => translateSettings.value.AzureRegion,
  set: (v: string) => { void translateSettingsStore.setAzureRegion(v) },
})
const translateCFAccountID = computed({
  get: () => translateSettings.value.CFAccountID,
  set: (v: string) => { void translateSettingsStore.setCFAccountID(v) },
})
const translateCFToken = computed({
  get: () => translateSettings.value.CFToken,
  set: (v: string) => { void translateSettingsStore.setCFToken(v) },
})
const translateModelMsg = ref('')
// 成功提示也 5 秒自动消失（和底部错误条同一套）。
autoClearMessage(translateModelMsg)
async function checkTranslateModel() {
  const ok = await translateSettingsStore.checkModel()
  translateModelMsg.value = ok ? '免费接口就绪，可以开字幕翻译' : ''
}

// 直播实时字幕：Whisper 听写英文 + 字幕翻译设置的免费接口翻中文，默认关。
// 后端在切台时自动启停；前端只负责收字幕事件、抓音频、清残留。
const liveSubtitleSettingsStore = createLiveSubtitleSettings(
  ShellService as unknown as LiveSubtitleSettingsApi,
  (e) => handleError(e),
)
const liveSubtitleSettings = liveSubtitleSettingsStore.settings
const liveSubtitleText = ref('')
const liveSubtitleStatusMsg = ref('')
const liveAudioPusher = {
  pushLiveAudioChunk: (b64: string) => ShellService.PushLiveAudioChunk(b64),
}
function toggleLiveSubtitleSetting(value: boolean) {
  void liveSubtitleSettingsStore.setEnabled(value)
  if (!value) liveSubtitleText.value = ''
}
async function refreshLiveSubtitleStatus() {
  const st = await liveSubtitleSettingsStore.status()
  if (!st) return
  // 开关只表示“已启用”：引擎只在播英文直播（网页播放）时才启动，
  // 设置页里没在播，“开着却未启动”是正常待机——文案必须写明原因。
  liveSubtitleStatusMsg.value = liveSubtitleStatusText(st)
}

const cardContextMenu = ref<{
  kind: 'history' | 'favorite'
  item: VodHistoryInfo | VodFavoriteInfo
} | null>(null)
const deleteConfirmation = ref<{
  kind: 'history' | 'favorite'
  item: VodHistoryInfo | VodFavoriteInfo
} | null>(null)
let lastProgressSave = 0

const showMpvInstallPrompt = computed(() => shouldShowMpvInstallPrompt(platform.value, mpvReady.value, mpvNeeded.value))

const activeEpisodes = computed(() => (vodDetail.value?.Episodes ?? []).filter(ep => ep.Source === activeSource.value))
const episodePages = computed(() => paginateEpisodes(activeEpisodes.value))
const episodeRanges = computed(() => episodePageRanges(activeEpisodes.value.length))
const visibleEpisodes = computed(() => episodePages.value[episodePage.value] ?? [])
const visibleVodItems = computed(() => vodView.value === 'search' ? vodSearchItems.value : vodListItems.value)
const activePlaybackPlan = computed(() => playbackPlanForMode(mode.value, {
  live: livePlaybackPlan.value,
  vod: vodPlaybackPlan.value,
}, playbackOwner.value ?? undefined))
// 空格键暂停/继续：mpv 没有 <video> 元素，暂停状态由这里维护，
// 经已有的 ShellService Pause/Resume 走 mpv 自己的播放控制。
const mpvPaused = ref(false)
const vodPlayerRef = ref<InstanceType<typeof PlaybackView> | null>(null)
const livePlayerRef = ref<InstanceType<typeof PlaybackView> | null>(null)

async function toggleMpvPause() {
  try {
    if (mpvPaused.value) {
      await ShellService.Resume()
      mpvPaused.value = false
    } else {
      await ShellService.Pause()
      mpvPaused.value = true
    }
  } catch (e) { handleError(e) }
}

// 全局空格键：播点播和直播时都接管（播放器只在获焦时收到按键，
// 覆盖不了整个窗口）；焦点在输入框/可编辑区时不触发，别跟打字冲突；
// 弹窗（点播源失效）开着时也不触发。
function handleGlobalSpaceKey(event: KeyboardEvent) {
  if (event.key !== ' ' || vodSourceDead.value) return
  const target = event.target as HTMLElement | null
  const tagName = target?.tagName
  if (tagName === 'INPUT' || tagName === 'TEXTAREA' || tagName === 'SELECT' || target?.isContentEditable) return
  if (mode.value === 'vod' && vodPagePlaybackPlan.value) {
    event.preventDefault()
    vodPlayerRef.value?.togglePlay()
    return
  }
  if (mode.value === 'live' && livePagePlaybackPlan.value) {
    event.preventDefault()
    livePlayerRef.value?.togglePlay()
  }
}

const livePagePlaybackPlan = computed(() => mode.value === 'live' && playbackOwner.value === 'live' ? livePlaybackPlan.value : null)
const vodPagePlaybackPlan = computed(() => mode.value === 'vod' && vodView.value === 'detail' && playbackOwner.value === 'vod' ? vodPlaybackPlan.value : null)
// 切了片子/频道，暂停状态跟着新计划走，不留旧的。
watch([vodPagePlaybackPlan, livePagePlaybackPlan], () => { mpvPaused.value = false })
const showVodNoResults = computed(() => shouldShowVodNoResults(vodQuery.value, completedSearchQuery.value, searching.value, vodSearchItems.value.length))

function resetEpisodePage() {
  episodePage.value = 0
  currentEpisodeID.value = ''
}

function selectEpisodePage(page: number) {
  episodePage.value = clampEpisodePage(page, activeEpisodes.value.length)
}

// selectEpisodeSource 切换线路。正在播放时走与自动换源一致的流程：立即播放
// 目标线路的同名当前集并续接进度；没有同名剧集就只提示、不改动当前播放。
async function selectEpisodeSource(source: string) {
  if (source === activeSource.value) return
  const playing = isCurrentPlayback('vod', vodPlaybackToken.value) && currentEpisodeID.value !== ''
  if (!playing) {
    activeSource.value = source
    resetEpisodePage()
    return
  }
  const same = sameNameEpisodeOnSource(vodDetail.value?.Episodes ?? [], source, vodNowPlaying.value)
  if (!same) {
    errMsg.value = `线路「${source}」没有同名剧集，未切换`
    return
  }
  errMsg.value = ''
  try {
    const played = await doPlayEpisode(
      detailSite.value || activeSite.value,
      same.ID,
      same.Name,
      source,
      vodPlaybackPosition.value,
    )
    if (played) await applyPendingSeek()
  } catch (e) {
    if (isCurrentPlayback('vod', vodPlaybackToken.value)) setVodPlaybackError(String(e))
  }
}

function scrollEpisodePages(direction: number) {
  const element = episodePagination.value
  if (!element) return
  element.scrollBy({ left: direction * Math.max(element.clientWidth * 0.75, 160), behavior: 'smooth' })
}

const vodSites = computed(() => sources.value.filter(s => s.Kind === 'vod'))
const vodLines = computed(() => {
  const names: string[] = []
  for (const s of vodSites.value) {
    const ln = s.Line || ''
    if (!names.includes(ln)) names.push(ln)
  }
  return names
})
function sitesOfLine(line: string) {
  return vodSites.value.filter(s => (s.Line || '') === line)
}
const vodLineOptions = computed(() => vodLines.value.map(line => ({ name: line || '默认线路', value: line })))
function siteName(site: string) {
  if (!site) return ''
  return sources.value.find(s => s.ID === site)?.Name ?? site
}

// openVodSourceDead 详情请求失败时弹「无法访问点播源」弹窗。
// 不进底部红条——站点连不上是常态，不是程序故障。
function openVodSourceDead(site: string, vodTitle: string, origin: VodDetailOrigin, e: unknown) {
  vodSourceDead.value = { info: buildVodSourceDeadInfo(siteName(site), vodTitle, e), origin }
}

// 四处内容列表共用 ContentCardList，各自把原始数据归一成 ContentCardItem。
// 数组与源数据一一对应：组件只回传下标，父级用它取回自己的对象。
// 继续观看行另带已播比例：卡片下方画红色进度条（Netflix 风格）。
const homeCardItems = computed<ContentCardItem[]>(() => homeHistory.value.map(h => ({
  ...historyCardItem(
    h.Site + h.VodID,
    h.VodLogo,
    h.VodTitle,
    h.SiteName || h.Site,
    h.EpName,
    fmtProgress(h.Progress),
  ),
  progressRatio: h.Duration ? Math.min(1, Math.max(0, h.Progress / h.Duration)) : undefined,
})))

// 首页 Hero 取观看记录的第一条：焦点大图 + 继续播放入口。
const heroItem = computed(() => homeHistory.value[0] ?? null)
const heroProgress = computed(() => {
  const h = heroItem.value
  if (!h || !h.Duration) return 0
  return Math.min(1, Math.max(0, h.Progress / h.Duration))
})

// 海报加载失败时显示渐变封面：imgError 只会隐藏坏图，v-else 只在无 Logo
// 时渲染；加载失败不处理会留一个空横幅/空海报框。失败状态随当前影片切换复位。
const heroLogoFailed = ref(false)
watch(heroItem, () => { heroLogoFailed.value = false })

const detailLogoFailed = ref(false)
watch(vodDetail, () => { detailLogoFailed.value = false })

// 顶部导航在首页横幅上的滚动态：横幅顶部通透，下滑后变实底。
const navScrolled = ref(false)
function onHomeScroll(event: Event) {
  navScrolled.value = (event.target as HTMLElement).scrollTop > 24
}

// 顶部导航高亮：点播页按当前分类名归属到电影 / 电视 / 综艺 / 儿童。
const navActive = computed(() => {
  if (mode.value === 'home') return 'home'
  if (mode.value === 'live') return 'live'
  if (mode.value === 'favorites') return 'favorites'
  if (mode.value === 'vod') {
    const title = vodCategories.value.find(c => c.ID === vodActiveCat.value)?.Title ?? ''
    return topNavKeyForCategory(title)
  }
  return ''
})

// 顶部导航的点播分类入口：按分类名找真实分类（首个匹配词为主名，其余为别名）；
// 找不到就只进点播列表，不编分类。
async function goTopNavCategory(cat: TopNavCategory) {
  await goVodCategoryByTitle(cat.search[0], ...cat.search.slice(1))
}

// 首页海报行只取点播列表（vodListItems），不用 visibleVodItems：
// 用户刚搜完回首页时 visibleVodItems 是搜索结果，行会错显示搜索结果。
// 为你精选取前 10 条（Top 10 行已删，不再需要与它错开取 11–20 条）。
// 列表不足 10 条时有多少显示多少。
const homeListCardItems = computed<ContentCardItem[]>(() => vodListItems.value.map(vodCardItem))
const featuredRowItems = computed(() => homeListCardItems.value.slice(0, 10))
const favoriteRowItems = computed(() => favoriteCardItems.value.slice(0, 10))

// 详情页的「更多类似内容」：不再只推当前列表里的几条，改成拿当前片名去
// 全网搜同类（后台静默查所有点播站，不要 Key）；搜不到就显示空状态，
// 不硬推不相关的。打开详情后触发，返回/换详情时作废上一轮。
const similarNetworkItems = ref<VodItem[]>([])
const similarLoading = ref(false)
let similarRequest = 0
const similarItems = computed<ContentCardItem[]>(() => similarNetworkItems.value.map(vodCardItem))

async function loadSimilarVod() {
  const request = ++similarRequest
  const detail = vodDetail.value
  if (!detail) {
    similarNetworkItems.value = []
    similarLoading.value = false
    return
  }
  similarLoading.value = true
  try {
    const items = (await ShellService.VodSimilar(detail.Title, detail.ID)) ?? []
    if (request !== similarRequest) return
    similarNetworkItems.value = items as VodItem[]
  } catch {
    if (request !== similarRequest) return
    similarNetworkItems.value = []
  } finally {
    if (request === similarRequest) similarLoading.value = false
  }
}

// 首页分类快捷入口：进点播后选中该分类（复用点播页自己的选分类逻辑）。
async function goVodCategory(id: string) {
  await switchMode('vod')
  await selectVodCategory(id)
}

// 顶部导航的电影 / 电视剧入口：按分类名找真实分类（站点命名不一，
// 「连续剧」「剧集」都算电视剧）；找不到就只进点播列表，不编分类。
async function goVodCategoryByTitle(title: string, ...fallbacks: string[]) {
  await switchMode('vod')
  const names = [title, ...fallbacks]
  const hit = vodCategories.value.find(c => names.some(n => (c.Title ?? '').includes(n)))
  if (hit) await selectVodCategory(hit.ID)
}

// 横幅「更多信息」：用观看记录的真实站点 + ID 打开详情；返回时回首页。
async function openHeroDetail() {
  const h = heroItem.value
  if (!h) return
  await openVodDetail({
    ID: h.VodID,
    Title: h.VodTitle,
    Logo: h.VodLogo,
    Group: '',
    Site: h.Site,
  })
  vodDetailOrigin.value = 'home'
}

// 详情弹窗里点「更多类似内容」：进新详情，但返回目标保持进弹窗前那一页
// （搜索进来仍回搜索结果，不丢现场）。
async function openSimilarVodDetail(item: VodItem) {
  const origin = vodDetailOrigin.value
  await openVodDetail(item)
  vodDetailOrigin.value = origin
}

// 横向海报行的左右箭头：按轨道可视宽度的 85% 平滑滚动。
function scrollRow(event: MouseEvent, direction: number) {
  const wrap = (event.currentTarget as HTMLElement).parentElement
  const track = wrap?.querySelector('ul')
  if (!track) return
  track.scrollBy({ left: direction * track.clientWidth * 0.85, behavior: 'smooth' })
}

const favoriteCardItems = computed<ContentCardItem[]>(() => vodFavorites.value.map(f => ({
  key: f.Site + f.VodID,
  logo: f.Logo,
  title: f.Title,
  site: siteName(f.Site) || f.Site,
  detail: f.Group || '',
})))

// 点播条目（列表页与搜索结果共用）。没有站点名时把分类顶到站点位，与列表模式
// 原先的 vodItemSub 输出保持一致；两者都缺时留空，卡片徽标不渲染。
function vodCardItem(it: VodItem): ContentCardItem {
  const site = it.Site ? siteName(it.Site) : ''
  return {
    key: it.ID + (it.Site || ''),
    logo: it.Logo,
    title: it.Title,
    site: site || it.Group || '',
    detail: site ? (it.Group || '') : '',
  }
}

const vodCardItems = computed<ContentCardItem[]>(() => visibleVodItems.value.map(vodCardItem))

// 搜索结果中开头连续「强相关」的条数 = 强弱分界线的落点。排序在每次写入
// vodSearchItems 时完成（强相关在前），这里只数前缀。
const searchStrongCount = computed(() => countLeadingStrong(vodSearchItems.value, activeSearchQuery.value))

// 当前激活分类的筛选组与入口显隐：有非空筛选项才显示筛选按钮。
const activeVodCatObj = computed(() => vodCategories.value.find(c => c.ID === vodActiveCat.value))

// 首页分类栏只留电影 / 电视剧 / 动漫 / 综艺四项（第 6 项）：分类是源站
// 下发的（vodCategories），各源命名不一（电影片、连续剧、剧集、动画……），
// 按名包含匹配；每组只取第一个命中的分类，避免同一类出两颗芯片。
const homeMainCatGroups = [
  ['电影'],
  ['电视剧', '连续剧', '剧集'],
  ['动漫', '动画'],
  ['综艺'],
]
const homeMainCats = computed(() =>
  homeMainCatGroups
    .map(names => vodCategories.value.find(c => names.some(n => (c.Title ?? '').includes(n))))
    .filter((c): c is Section => !!c),
)
const vodFilterGroups = computed(() => availableFilterGroups(activeVodCatObj.value))

async function refresh() {
  try {
    platform.value = await ShellService.Platform()
    playerReady.value = await ShellService.PlayerReady()
    currentVersion.value = await ShellService.CurrentVersion()
    internalVersion.value = await ShellService.InternalVersion()
    await refreshMpvStatus()
    await initializeHomeState(refreshHome)
  } catch (e) {
    handleError(e)
  }
}

// handleError 统一处理前端错误：底部错误条回显。
function handleError(e: unknown) {
  errMsg.value = String(e)
}

// notePlaybackFailure 播放失败时的共同处理：如果这次失败是因为缺 mpv，
// 就打开安装提示。
//
// HEVC 的 HLS、RTMP 会**直接**路由到 mpv，不经过 Web，因此不会
// 触发 Web→mpv 的降级回调；只靠降级置位的话，这类失败永远不会提示安装 mpv。
function notePlaybackFailure(message: string) {
  if (isMpvUnavailableError(message)) mpvNeeded.value = true
}

// setVodPlaybackError 记下播放失败原因，界面回显。
function setVodPlaybackError(message: string) {
  vodPlaybackError.value = message
  if (!message) return
  notePlaybackFailure(message)
}

// setLivePlaybackError 直播同款：界面回显（播放器下方）。
function setLivePlaybackError(message: string) {
  livePlaybackError.value = message
  if (!message) return
  notePlaybackFailure(message)
}

async function openAbout() {
  try { internalVersion.value = await ShellService.InternalVersion() } catch { /* 忽略 */ }
  showAbout.value = true
}

// openURL 用系统默认浏览器打开外部链接（下载新版本）。
function openURL(url: string) {
  Browser.OpenURL(url).catch(() => {})
}

async function refreshMpvStatus() {
  const s = await ShellService.MPVStatus()
  mpvReady.value = s.Available
  mpvInstallMode.value = s.InstallMode
  if (s.Available) mpvNeeded.value = false
}

async function installMpv() {
  errMsg.value = ''; installMessage.value = ''
  try {
    const r = await ShellService.InstallMPV()
    installMessage.value = r.Message
    if (r.Installed) { installMessage.value = ''; await refreshMpvStatus() }
  } catch (e) { handleError(e) }
}

async function recheckMpv() {
  try {
    const s = await ShellService.RefreshMPV()
    mpvReady.value = s.Available
    if (s.Available) installMessage.value = ''
  } catch (e) { handleError(e) }
}

async function switchMode(m: 'home' | 'vod' | 'live' | 'search' | 'favorites' | 'settings') {
  if (m !== 'search' && searching.value) await invalidateSearch()
  if (m !== 'live' && activePlayback.value?.scope === 'live') await stopPlayback('live')
  if (m !== mode.value && activePlayback.value?.scope === 'vod') await stopPlayback('vod')
  if (vodView.value === 'detail') currentVod.value = null
  mode.value = m
  if (m === 'vod') {
    vodView.value = 'list'
    vodDetail.value = null
    await refreshVod()
  }
  else if (m === 'live') await reloadGroups()
  else if (m === 'search') {
    vodView.value = 'search'
    vodDetail.value = null
    await loadVodSearchHistory()
  }
  else if (m === 'favorites') {
    vodView.value = 'list'
    vodDetail.value = null
    await loadVodFavorites()
  }
  else if (m === 'home') { navScrolled.value = false; await refreshHome() }
  else if (m === 'settings') { await reloadSourceHistory(); await reloadInsecureTLSHosts(); await loadVodSourceStatus(); await loadSearchThreads(); await loadSearchCacheTTL(); await playbackSettingsStore.load(); await translateSettingsStore.load(); await liveSubtitleSettingsStore.load() }
}

async function refreshHome() {
  try {
    // 本地媒体库已删除：过去的本地观看记录播不了，不再进「继续观看」。
    const history = (await ShellService.ListVodHistory()) ?? []
    homeHistory.value = history.filter(item => item.Site !== 'local')
  } catch (e) { handleError(e) }
  // 首页海报行的数据预取：只在点播分类从未加载过时拉一次（复用点播页自己
  // 的加载逻辑，不新增取数路径）；源站连不上时不把错误顶到首页——分类页
  // 自己会再报错，进首页不打扰。
  if (!vodCategories.value.length && !vodCategoryLoading.value) {
    const previousError = errMsg.value
    await refreshVod()
    if (!vodCategories.value.length) errMsg.value = previousError
  }
  // 我的片单行取真实收藏：本地库查询，失败时 loadVodFavorites 自己报错。
  await loadVodFavorites()
}

async function loadVodSearchHistory() {
  try { vodSearchHistory.value = (await ShellService.ListVodSearchHistory()) ?? [] } catch (e) { handleError(e) }
}

async function loadVodFavorites() {
  try { vodFavorites.value = (await ShellService.ListVodFavorites()) ?? [] } catch (e) { handleError(e) }
}

async function deleteHomeHistory(h: VodHistoryInfo) {
  try {
    await ShellService.DeleteVodHistory(h.Site, h.VodID)
    homeHistory.value = removeVodHistory(homeHistory.value, h.Site, h.VodID)
  } catch (e) { handleError(e) }
}

async function deleteVodSearchHistory(query: string) {
  try {
    await ShellService.DeleteVodSearchHistory(query)
    vodSearchHistory.value = removeVodSearchHistory(vodSearchHistory.value, query)
  } catch (e) { handleError(e) }
}

async function useVodSearchHistory(query: string) {
  vodQuery.value = query
  searchHistoryExpanded.value = false
  await vodSearch()
}

// 下拉框与外显的历史区同宽同起点：宽度就是内联区当前的可见宽度，
// 末尾那个被裁掉一半的历史词也算在内。展开和窗口缩放时重算。
function syncSearchHistoryDropdown() {
  const box = searchHistoryInline.value
  if (!box) return
  searchHistoryDropdownStyle.value = {
    left: `${box.offsetLeft}px`,
    right: 'auto',
    width: `${box.clientWidth}px`,
  }
}

function syncSearchHistoryDropdownOnResize() {
  if (searchHistoryExpanded.value) syncSearchHistoryDropdown()
}

// 展开前先量一次，避免下拉框先按整行宽度渲染一帧再收窄。
function toggleSearchHistory() {
  if (!searchHistoryExpanded.value) syncSearchHistoryDropdown()
  searchHistoryExpanded.value = !searchHistoryExpanded.value
}

async function deleteVodFavorite(favorite: VodFavoriteInfo) {
  try {
    await ShellService.RemoveVodFavorite(favorite.Site, favorite.VodID)
    vodFavorites.value = removeVodFavorite(vodFavorites.value, favorite.Site, favorite.VodID)
    if (currentVod.value?.site === favorite.Site && currentVod.value.vodID === favorite.VodID) vodFavorited.value = false
  } catch (e) { handleError(e) }
}

function openCardContextMenu(kind: 'history' | 'favorite', item: VodHistoryInfo | VodFavoriteInfo, event: MouseEvent) {
  if (contentCardStyle.value !== 'grid') return
  event.preventDefault()
  cardContextMenu.value = { kind, item }
}

function closeCardContextMenu() {
  cardContextMenu.value = null
}

// activeDeleteKey 是当前展开了删除遮罩的条目 key。原先按对象身份比较，改成按 key
// 是因为列表交给 ContentCardList 渲染后，父级拿到的是下标而不是原对象引用。
const activeDeleteKey = computed(() => {
  const menu = cardContextMenu.value
  return menu ? menu.item.Site + menu.item.VodID : null
})

function requestDeleteContextCard() {
  if (!cardContextMenu.value) return
  deleteConfirmation.value = cardContextMenu.value
  cardContextMenu.value = null
}

function closeDeleteConfirmation() {
  deleteConfirmation.value = null
}

function handleCardContextMenuKeydown(event: KeyboardEvent) {
  if (event.key !== 'Escape') return
  closeCardContextMenu()
  closeDeleteConfirmation()
}

async function confirmDeleteContextCard() {
  const target = deleteConfirmation.value
  closeDeleteConfirmation()
  if (!target) return
  if (target.kind === 'history') {
    await deleteHomeHistory(target.item as VodHistoryInfo)
  } else {
    await deleteVodFavorite(target.item as VodFavoriteInfo)
  }
}

async function toggleVodFavorite() {
  const detail = vodDetail.value
  const site = detailSite.value || activeSite.value
  if (!detail || !site) return
  try {
    if (vodFavorited.value) {
      await ShellService.RemoveVodFavorite(site, detail.ID)
      vodFavorited.value = false
      vodFavorites.value = removeVodFavorite(vodFavorites.value, site, detail.ID)
    } else {
      await ShellService.AddVodFavorite(site, detail.ID, detail.Title, detail.Logo, detail.Group)
      vodFavorited.value = true
      await loadVodFavorites()
    }
  } catch (e) { handleError(e) }
}

function fmtProgress(sec: number) {
  if (sec <= 0) return ''
  const m = Math.floor(sec / 60)
  const s = Math.floor(sec % 60)
  return m > 0 ? `${m}分${s}秒` : `${s}秒`
}

async function resumeVod(h: VodHistoryInfo) {
  errMsg.value = ''
  if (vodDetailLoading.value) return // 上一个详情请求还没回，忽略连点
  vodSourceDead.value = null
  vodDetailLoading.value = h.VodTitle
  try {
    await stopPlayback('live')
    mode.value = 'vod'
    await loadSources()
    activeSite.value = h.Site
    detailSite.value = h.Site
    vodDetailOrigin.value = 'home'
    vodView.value = 'detail'
    vodResume.value = null
    // 先清空上一条记录留下的详情，加载期间与失败后都不残留旧内容。
    vodDetail.value = null
    const d = await ShellService.VodDetail(h.Site, h.VodID)
    d.Description = DOMPurify.sanitize(d.Description)
    vodDetail.value = d
    await applyVodResume(h)
  } catch (e) {
    if (certErrorHost(String(e))) tlsRetry = () => resumeVod(h)
    openVodSourceDead(h.Site, h.VodTitle, 'home', e)
  } finally {
    vodDetailLoading.value = ''
  }
}

// applyPendingSeek 给 mpv 后端补一次定位（网页播放器由 PlaybackView 的 seek-to 自行消费）。
async function applyPendingSeek() {
  if (pendingSeek.value > 0 && vodPlaybackPlan.value?.Backend === 'mpv') {
    await ShellService.Seek(pendingSeek.value)
  }
}

// applyVodResume 按一条点播观看记录立即续播（首页「继续观看」用）：优先历史线路，否则首条可用线路。
// 需在详情已加载后调用；无集数记录时仅切换线路、不自动播放。
async function applyVodResume(h: VodHistoryInfo) {
  const view = vodResumeView(vodDetail.value, h)
  activeSource.value = view.source
  resetEpisodePage()
  currentEpisodeID.value = view.episodeID
  episodePage.value = view.page
  if (h.EpID) {
    // 看完的集从头播，而不是一打开就走到结尾、立刻触发 ended。
    const seek = resumeSeekFor(h.Progress, h.Duration)
    pendingSeek.value = seek
    const played = await doPlayEpisode(h.Site, h.EpID, h.EpName, view.source, seek)
    if (played) await applyPendingSeek()
  }
}

// prepareVodResume 收藏页进详情用：切到上次线路、翻到那集所在页并记下续播目标，但不自动开播。
// 用户点某一集时才由 playEpisode 套用这里的进度。
async function prepareVodResume(site: string, vodID: string) {
  try {
    const h = await ShellService.GetVodHistory(site, vodID)
    if (!h || !h.VodID || !h.EpID) return
    const view = vodResumeView(vodDetail.value, h)
    activeSource.value = view.source || activeSource.value
    resetEpisodePage()
    currentEpisodeID.value = view.episodeID
    episodePage.value = view.page
    // 看完的集下次点开从头播（resumeSeekFor），而不是走到结尾。
    vodResume.value = { EpID: h.EpID, Progress: resumeSeekFor(h.Progress, h.Duration) }
  } catch (e) { handleError(e) }
}

// ---- 设置：源管理 ----
async function reloadSourceHistory() {
  try {
    const all = (await ShellService.ListSources()) ?? []
    vodSources.value = all.filter(s => s.Kind === 'vod')
    liveSources.value = all.filter(s => s.Kind === 'live')
    vodSourceUrl.value = vodSources.value[0]?.Ref ?? ''
    liveSourceUrl.value = liveSources.value[0]?.Ref ?? ''
  } catch (e) { handleError(e) }
}

// ---- 设置：点播源来源（本地 / Cloudflare）----
// 后端方法见 internal/shell/vodsource.go：SetCloudflareVodURL、
// RefreshCloudflareVodSource、UseLocalVodSource、GetVodSourceStatus。
const vodSourceStatus = ref<VodSourceStatus | null>(null)
const cfVodUrl = ref('')
// 点播源目录：下拉框的选项从这里自动拉取（默认地址见 vodSource.ts）；
// 目录拉不到（文件还没挂上 / 没网）时用内置默认源顶着，不报错。
const cfVodDirectoryUrl = ref(DEFAULT_CF_VOD_DIRECTORY_URL)
const cfVodDirectorySources = ref<CfVodDirectorySource[]>([])
const cfVodDirectoryMsg = ref('')
const cfVodSourceChoice = ref('')
// 下拉框选项：目录里的源 + “手动输入…”；resolveCfVodSources 保证
// 正在用的地址（哪怕不在目录里）也在列表里，免得选不中当前源。
const cfVodSources = computed(() => resolveCfVodSources(cfVodDirectorySources.value, cfVodUrl.value))
const cfVodSourceOptions = computed(() => [
  ...cfVodSources.value.map((src) => ({ name: src.name, value: src.url })),
  { name: '手动输入…', value: CF_VOD_SOURCE_MANUAL },
])

function syncCfVodSourceChoice() {
  const url = cfVodUrl.value.trim()
  cfVodSourceChoice.value = cfVodSources.value.some((src) => src.url === url) ? url : CF_VOD_SOURCE_MANUAL
}

async function refreshCfVodDirectory() {
  try {
    cfVodDirectorySources.value = parseCfVodDirectory(await ShellService.FetchCfVodDirectory())
    cfVodDirectoryMsg.value = ''
  } catch {
    // 目录拉不到不报错：内置默认源顶着，用户还能手动填地址。
    cfVodDirectorySources.value = []
    cfVodDirectoryMsg.value = '点播源目录暂时拉不到，先用内置的几个源'
  }
  syncCfVodSourceChoice()
}

async function saveCfVodDirectoryUrl() {
  const url = cfVodDirectoryUrl.value.trim()
  if (!isValidCfVodURL(url) || importing.value) return
  try {
    await ShellService.SetCfVodDirectoryURL(url)
    await refreshCfVodDirectory()
  } catch (e) { handleError(e) }
}

async function onCfVodSourceSelect(value: string) {
  cfVodSourceChoice.value = value
  if (value === CF_VOD_SOURCE_MANUAL) return
  cfVodUrl.value = value
  await saveCfVodUrl()
}

async function loadVodSourceStatus() {
  try {
    vodSourceStatus.value = await ShellService.GetVodSourceStatus()
    if (vodSourceStatus.value?.CloudflareURL) cfVodUrl.value = vodSourceStatus.value.CloudflareURL
    if (vodSourceStatus.value?.DirectoryURL) cfVodDirectoryUrl.value = vodSourceStatus.value.DirectoryURL
  } catch (e) { handleError(e) }
  // 打开设置或拉完源都顺手刷一下目录：有新增的源就自动出现在下拉框里。
  await refreshCfVodDirectory()
}

async function saveCfVodUrl() {
  const url = cfVodUrl.value.trim()
  if (!isValidCfVodURL(url) || importing.value) return
  errMsg.value = ''; importSummary.value = ''; importProgress.value = null
  importingKind.value = 'vod'
  try {
    const r = await ShellService.SetCloudflareVodURL(url)
    importSummary.value = `Cloudflare 点播源拉取成功：${r.Sites} 个站点`
    await loadVodSourceStatus()
    await reloadSourceHistory()
    activeSite.value = ''
    await refreshVod()
  } catch (e) {
    handleError(e)
  }
  finally { importingKind.value = ''; importProgress.value = null }
}

async function refreshCfVodSource() {
  if (importing.value) return
  errMsg.value = ''; importSummary.value = ''; importProgress.value = null
  importingKind.value = 'vod'
  try {
    const r = await ShellService.RefreshCloudflareVodSource()
    importSummary.value = `Cloudflare 点播源刷新成功：${r.Sites} 个站点`
    await loadVodSourceStatus()
    await reloadSourceHistory()
    activeSite.value = ''
    await refreshVod()
  } catch (e) {
    handleError(e)
  }
  finally { importingKind.value = ''; importProgress.value = null }
}

async function useLocalVodSource() {
  if (importing.value) return
  errMsg.value = ''; importSummary.value = ''
  try {
    const r = await ShellService.UseLocalVodSource()
    importSummary.value = `已切回本地源：${r.Sites} 个站点`
    await loadVodSourceStatus()
    await reloadSourceHistory()
    activeSite.value = ''
    await refreshVod()
  } catch (e) {
    handleError(e)
  }
}

// ---- 设置：证书豁免 ----
// 只有显式加入列表的主机才会在证书校验失败后降级重试；默认仍然严格校验。
async function reloadInsecureTLSHosts() {
  try {
    insecureTLSHosts.value = (await ShellService.ListInsecureTLSHosts()) ?? []
  } catch (e) { handleError(e) }
}

async function addInsecureTLSHost() {
  const host = insecureTLSInput.value.trim()
  if (!host) return
  insecureTLSError.value = ''
  try {
    insecureTLSHosts.value = (await ShellService.SetInsecureTLSHost(host, true)) ?? []
    insecureTLSInput.value = ''
    // 添加即展开已豁免列表：用户马上能看到刚加进去的站点，不用再点一次。
    showTLSHistory.value = true
  } catch (e) {
    insecureTLSError.value = String(e)
  }
}

async function removeInsecureTLSHost(host: string) {
  insecureTLSError.value = ''
  try {
    insecureTLSHosts.value = (await ShellService.SetInsecureTLSHost(host, false)) ?? []
  } catch (e) {
    insecureTLSError.value = String(e)
  }
}

async function importVodSource() {
  const ref = vodSourceUrl.value.trim()
  if (!ref || importing.value) return
  errMsg.value = ''; importSummary.value = ''; importProgress.value = null
  importingKind.value = 'vod'
  try {
    const r = await ShellService.ImportVodSource(ref)
    importSummary.value = `点播源导入成功：${r.Sites} 个站点`
    await reloadSourceHistory()
    activeSite.value = ''
    await refreshVod()
  } catch (e) {
    if (certErrorHost(String(e))) tlsRetry = importVodSource
    handleError(e)
  }
  finally { importingKind.value = ''; importProgress.value = null }
}

async function importLiveSource() {
  const ref = liveSourceUrl.value.trim()
  if (!ref || importing.value) return
  errMsg.value = ''; importSummary.value = ''; importProgress.value = null
  importingKind.value = 'live'
  try {
    const r = await ShellService.ImportLiveSource(ref)
    importSummary.value = r.Channels > 0 ? `直播源导入成功：${r.Channels} 频道` : `直播源导入成功：${r.LiveSources} 个源`
    await reloadSourceHistory()
  } catch (e) {
    if (certErrorHost(String(e))) tlsRetry = importLiveSource
    handleError(e)
  }
  finally { importingKind.value = ''; importProgress.value = null }
}

async function reimportSource(kind: string, ref: string) {
  if (kind === 'vod') { vodSourceUrl.value = ref; await importVodSource() }
  else { liveSourceUrl.value = ref; await importLiveSource() }
  showVodHistory.value = false
  showLiveHistory.value = false
}

async function deleteSource(kind: string, ref: string) {
  try {
    await ShellService.DeleteSource(kind, ref)
    await reloadSourceHistory()
    // 删掉的源已从后端卸掉：刷新对应视图，否则列表里还留着已删源的站点/分组。
    if (kind === 'vod') await refreshVod()
    else await reloadGroups()
  } catch (e) { handleError(e) }
}

async function reloadGroups() {
  groups.value = ['*', ...((await ShellService.Groups()) ?? [])]
  await reloadChannels()
}

async function reloadChannels() {
  // 收藏视图下列表就是已收藏频道，不按分类去后端拉。
  if (liveFavOnly.value) {
    channels.value = [...favorites.value]
    return
  }
  channels.value = (await ShellService.Channels(activeGroup.value === '*' ? '' : activeGroup.value, 0)) ?? []
}

function selectLiveGroup(id: string) {
  liveFavOnly.value = false
  activeGroup.value = id
  void reloadChannels()
}

async function toggleLiveFavorites() {
  if (liveFavOnly.value) {
    liveFavOnly.value = false
    await reloadChannels()
    return
  }
  await loadFavorites()
}

async function doSearch() {
  if (!query.value) { await reloadChannels(); return }
  channels.value = (await ShellService.Search(query.value)) ?? []
}

async function loadLive() {
  errMsg.value = ''; importProgress.value = null
  try {
    const n = await ShellService.LoadLive()
    if (n === 0) importSummary.value = '没有可用的直播频道'
    await reloadGroups()
  } catch (e) { handleError(e) }
}

async function play(c: ChannelInfo) {
  errMsg.value = ''
  currentVod.value = null
  const token = beginPlayback('live')
  livePlaybackPlan.value = null
  livePlaybackToken.value = 0
  livePlaybackStatus.value = 'preparing'
  livePlaybackError.value = ''
  liveNowPlaying.value = c.Name
  // 切台清掉上一条直播字幕，免得新台还顶着旧台的句子。
  liveSubtitleText.value = ''
  // 切频道反馈走瞬态 toast（不再常驻播放器上方）；收藏视图下不改分类下拉框，
  // 免得下拉框显示的分组和列表里实际的收藏内容对不上。
  flashLiveToast(`正在切换频道：${c.Name}`)
  if (!liveFavOnly.value && c.Group && groups.value.includes(c.Group)) activeGroup.value = c.Group
  try {
    const plan = await ShellService.PrepareChannelWithToken(c.ID, token) as unknown as PlaybackPlan
    if (!isCurrentPlayback('live', token)) { await pauseStalePlayback(token); return }
    livePlaybackPlan.value = plan
    livePlaybackToken.value = token
    livePlaybackStatus.value = 'playing'
  } catch (e) {
    if (isCurrentPlayback('live', token)) {
      livePlaybackPlan.value = null
      livePlaybackToken.value = 0
      livePlaybackStatus.value = 'error'
      setLivePlaybackError(String(e))
    }
  }
}

async function toggleFav(c: ChannelInfo) {
  try {
    if (c.Favorited) await ShellService.RemoveFavorite(c.ID)
    else await ShellService.AddFavorite(c.ID)
    c.Favorited = !c.Favorited
    favorites.value = (await ShellService.ListFavorites()) ?? []
  } catch (e) { handleError(e) }
}

async function loadFavorites() {
  favorites.value = (await ShellService.ListFavorites()) ?? []
  // 收藏作为筛选态：右侧频道列表切换为已收藏频道，☆ 开关高亮；分类下拉框不动。
  channels.value = [...favorites.value]
  liveFavOnly.value = true
}

async function loadSources() {
  sources.value = (await ShellService.Sources()) ?? []
  const last = activeSite.value || await ShellService.LastVodSite()
  const selection = resolveVodSelection(vodSites.value, last)
  activeSite.value = selection.site
  activeLine.value = selection.line
}

async function refreshVod() {
  await loadSources()
  await reloadVodCategories()
}

async function selectLine(line: string) {
  activeLine.value = line
  const first = sitesOfLine(line)[0]
  if (first) {
    activeSite.value = first.ID
    vodDetail.value = null
    vodView.value = 'list'
    vodPage.value = 0
    await reloadVodCategories()
    await ShellService.SetLastVodSite(first.ID)
  }
}

async function reloadVodCategories() {
  const request = nextVodCategoryRequest(vodCategoryRequest)
  vodCategoryRequest = request
  vodCategoryLoading.value = true
  vodCategoryError.value = ''
  // 重新拉取就是一次新的尝试：上一轮留在底部错误条里的提示与一键豁免按钮一并清掉。
  errMsg.value = ''
  vodCategories.value = []
  vodActiveCat.value = ''
  // 换站点/线路/重新导入都会走到这里：筛选值是站点专属的，必须一并清空，
  // 否则会把旧站点的维度 key 发给新站点（CMS 可能直接返回空列表）。
  resetVodFilter()
  resetVodListState()
  if (!activeSite.value) {
    // 没有可用站点（如删掉了最后一个点播源）：只清空展示状态，不调后端，
    // 否则空站点会报「点播站点不存在」刷屏。
    vodCategoryLoading.value = false
    vodListEnd.value = true
    vodListAtBottom.value = true
    return
  }
  try {
    const categories = (await ShellService.VodCategories(activeSite.value)) ?? []
    if (!isCurrentVodCategoryRequest(request, vodCategoryRequest)) return
    vodCategories.value = categories
    vodActiveCat.value = categories[0]?.ID ?? ''
    if (vodActiveCat.value) {
      await reloadVodList()
    } else {
      // 站点一个分类都没有：没有可滚动内容，直接按「已到底」处理，显示分割线。
      vodListEnd.value = true
      vodListAtBottom.value = true
    }
  } catch (e) {
    if (isCurrentVodCategoryRequest(request, vodCategoryRequest)) {
      vodCategoryError.value = String(e)
      if (certErrorHost(String(e))) tlsRetry = reloadVodCategories
      handleError(e)
    }
  } finally {
    if (isCurrentVodCategoryRequest(request, vodCategoryRequest)) vodCategoryLoading.value = false
  }
}

function resetVodListState() {
  vodListRequest++
  vodListLoading.value = false
  vodPage.value = 0
  vodListItems.value = []
  vodHasMore.value = false
  vodListEnd.value = false
  vodListAtBottom.value = false
}

async function reloadVodList(reset = true) {
  if (!activeSite.value || !vodActiveCat.value) return
  // 新一轮加载清掉上一轮的失败原因：成功后状态区不再留旧报错。
  vodListError.value = ''
  if (reset) resetVodListState()
  else if (vodListLoading.value) return
  const request = ++vodListRequest
  const page = nextVodPage(vodPage.value)
  vodListLoading.value = true
  let loadedOk = false
  try {
    const result = await ShellService.VodListFiltered(activeSite.value, vodActiveCat.value, page, vodFilterActive.value) as VodListPage
    if (request !== vodListRequest) return
    const items = result?.Items ?? []
    const previousCount = vodListItems.value.length
    vodListItems.value = reset ? items : appendVodItems(vodListItems.value, items)
    const addedCount = reset ? items.length : vodListItems.value.length - previousCount
    vodPage.value = result?.Page > 0 ? result.Page : page
    vodHasMore.value = hasNextVodPage(result, addedCount)
    vodListEnd.value = !vodHasMore.value
    loadedOk = true
    // 追加/换页后旧位置不再算最底：先立即置 false 避免分割线闪一下，
    // DOM 更新后按实际滚动位置重新量取。
    vodListAtBottom.value = false
    void nextTick(() => { vodListAtBottom.value = vodListRef.value?.isAtBottom() ?? false })
  } catch (e) {
    if (request === vodListRequest) {
      vodListEnd.value = false
      // 列表失败的原因留在内容区内明示（底部错误条容易错过），并给重试入口。
      vodListError.value = String(e)
      if (certErrorHost(String(e))) tlsRetry = () => reloadVodList(reset)
      handleError(e)
    }
  } finally {
    if (request === vodListRequest) {
      vodListLoading.value = false
      if (reset && loadedOk && vodHasMore.value) void prefetchVodPages()
    }
  }
}

// 换分类后默认多拉两页（共 3 页约 60 条）：每分类只给一页太少，用户要「多放
// 一些」。后台静默追加，不挡首屏；切分类/筛选/没更多页时立刻停。
async function prefetchVodPages() {
  const cat = vodActiveCat.value
  for (let i = 0; i < VOD_LIST_PREFETCH_PAGES; i++) {
    if (vodActiveCat.value !== cat || !vodHasMore.value || vodListLoading.value) return
    await reloadVodList(false)
  }
}

async function loadNextVodPage() {
  if (!vodHasMore.value || vodListLoading.value) return
  await reloadVodList(false)
}

// 分割线跟随真实滚动位置：滑到最底才显示，往回滚立刻隐藏。
function onVodListScroll(atBottom: boolean) {
  vodListAtBottom.value = atBottom
}

function onVodListWheel(event: WheelEvent) {
  if (event.deltaY <= 0 || !vodHasMore.value || vodListLoading.value) return
  const list = event.currentTarget as HTMLElement
  if (list.scrollTop + list.clientHeight < list.scrollHeight - 2) return
  event.preventDefault()
  void loadNextVodPage()
}

function scrollVodListTop() {
  vodListRef.value?.scrollToTop()
}

async function selectVodCategory(id: string) {
  if (id === vodActiveCat.value && vodListItems.value.length > 0) return
  vodActiveCat.value = id
  resetVodFilter()
  await reloadVodList()
}

// 应用筛选：写入 active、重拉第 1 页。
// 这里传整份选择（含空值「全部」），与 TVBox 一致：脚本能区分
// 「用户选了全部」和「没有筛选」——后者 extend 里根本没有该键。
function applyVodFilter() {
  vodFilterActive.value = appliedFilter(vodFilterDraft.value)
  resetVodListState()
  void reloadVodList()
}

// chip 点选即应用：写草稿、记为已应用、重拉第 1 页（与旧版点「确定」同一条路）。
function onFilterChipPick(v: Record<string, string>) {
  vodFilterDraft.value = v
  applyVodFilter()
}

// 清除筛选：彻底去掉已应用的筛选（active 置 null，即「没有筛选」），
// 而不是套用各维度第一项——有的站点第一项不是「全部」（如某个具体年份），
// 套第一项等于偷偷换了一个限制性筛选，清除按钮还会一直亮着。
// 草稿仍回到第一项，供面板显示用。
function clearVodFilter() {
  vodFilterDraft.value = initialSelections(vodFilterGroups.value)
  vodFilterActive.value = null
  resetVodListState()
  void reloadVodList()
}

// 切分类/切站点清空筛选选择与已应用值。
function resetVodFilter() {
  vodFilterActive.value = null
  vodFilterDraft.value = {}
}

// preserveItems 为 true 时不清空现有列表：详情返回且缓存过期时用旧列表占位，
// 新结果到达后再替换，避免列表闪空、滚动位置跳到顶部。
// flushPendingSearchResults 把积压的渐进搜索结果合并渲染：批量站点秒回时，
// 逐事件排序+重渲染会占死事件循环（进度数字也变卡），这里压成 120ms 一批。
function flushPendingSearchResults() {
  searchResultTimer = null
  if (!searchResultPending.length) return
  const batch = searchResultPending
  searchResultPending = []
  vodSearchItems.value = sortVodSearchResults([...vodSearchItems.value, ...batch], activeSearchQuery.value)
  vodSearchCache.value = createVodSearchCache(activeSearchQuery.value, vodSearchItems.value, Date.now(), searchCacheTTL.value * 60 * 1000)
}

async function vodSearch(preserveItems = false) {
  const searchQuery = vodQuery.value.trim()
  const request = nextVodSearchRequest(searchRequest.value)
  searchRequest.value = request
  activeSearchQuery.value = searchQuery
  completedSearchQuery.value = ''
  searchFloorID.value = activeSearchID.value
  activeSearchID.value = 0
  // 新会话：清掉上一轮积压的渐进结果，避免旧的 flush 把过期数据写进新列表。
  if (searchResultTimer !== null) {
    clearTimeout(searchResultTimer)
    searchResultTimer = null
  }
  searchResultPending = []
  if (vodView.value === 'detail') await stopPlayback('vod')
  searchProgress.value = null
  if (!searchQuery) {
    searching.value = false
    try { await ShellService.CancelSearch() } catch { /* 忽略 */ }
    vodSearchItems.value = []
    vodSearchCache.value = null
    return
  }
  searching.value = true
  currentVod.value = null
  mode.value = 'search'
  vodView.value = 'search'
  vodDetail.value = null
  searchSelectedIndex.value = -1 // 新搜索会话：清掉上次的返回定位
  searchScrollTop.value = 0
  if (!preserveItems) vodSearchItems.value = []
  void ShellService.RecordVodSearch(searchQuery).then(() => {
    vodSearchHistory.value = upsertVodSearchHistory(vodSearchHistory.value, searchQuery)
  }).catch(() => {})
  try {
    const items = (await ShellService.VodSearchAllWithToken(searchQuery, request)) ?? []
    if (searchRequest.value !== request || activeSearchQuery.value !== searchQuery) return
    // 站点后端模糊搜索会混入弱相关结果；按相关度稳定排序，强相关整体在前。
    vodSearchItems.value = sortVodSearchResults(items, searchQuery)
    // 缓存排序后的列表：详情页返回恢复时保持强相关在前的顺序与分界线一致。
    vodSearchCache.value = createVodSearchCache(searchQuery, vodSearchItems.value, Date.now(), searchCacheTTL.value * 60 * 1000)
    completedSearchQuery.value = searchQuery
  } catch (e) {
    if (searchRequest.value === request) handleError(e)
  }
  finally {
    flushPendingSearchResults()
    if (searchRequest.value === request) searching.value = false
  }
}

async function cancelSearch() {
  await invalidateSearch()
  vodSearchCache.value = createVodSearchCache(activeSearchQuery.value, vodSearchItems.value, Date.now(), searchCacheTTL.value * 60 * 1000)
}

async function invalidateSearch() {
  searchRequest.value = nextVodSearchRequest(searchRequest.value)
  activeSearchID.value = 0
  searching.value = false
  if (searchResultTimer !== null) {
    clearTimeout(searchResultTimer)
    searchResultTimer = null
  }
  searchResultPending = []
  try { await ShellService.CancelSearch() } catch { /* 忽略 */ }
}

async function openVodDetail(item: VodItem) {
  errMsg.value = ''
  vodResume.value = null
  if (vodDetailLoading.value) return // 上一个详情请求还没回，忽略连点
  // 站点与来源在 try 外先定好：失败分支要靠它弹失效弹窗。
  const site = item.Site || activeSite.value
  const origin: VodDetailOrigin = mode.value === 'favorites'
    ? 'favorites'
    : mode.value === 'search' || vodView.value === 'search' ? 'search' : 'list'
  vodDetailOrigin.value = origin
  // 从搜索结果进详情：记下选中条目与滚动偏移，返回时原样恢复。
  // 必须在任何 await 之前取（后面 cancelSearch 会动搜索结果区）。
  if (origin === 'search') {
    searchSelectedIndex.value = vodSearchItems.value.indexOf(item)
    searchScrollTop.value = searchListRef.value?.getScrollTop() ?? 0
  }
  vodSourceDead.value = null
  vodDetailLoading.value = item.Title
  // 清掉上一条留下的详情，失败后 openVodFavorite 的 `!vodDetail.value` 守卫才靠得住。
  // 同类也一起清掉，上一轮后台查询作废。
  vodDetail.value = null
  similarRequest++
  similarNetworkItems.value = []
  similarLoading.value = false
  try {
    if (searching.value) await cancelSearch()
    detailSite.value = site
    const d = await ShellService.VodDetail(site, item.ID)
    d.Description = DOMPurify.sanitize(d.Description)
    vodDetail.value = d
    mode.value = 'vod'
    vodView.value = 'detail'
    activeSource.value = d.Sources?.[0] ?? ''
    resetEpisodePage()
    vodFavorited.value = await ShellService.IsVodFavorite(site, d.ID)
    // 详情落地后后台全网找同类，不挡详情显示。
    void loadSimilarVod()
  } catch (e) {
    if (certErrorHost(String(e))) tlsRetry = () => openVodDetail(item)
    openVodSourceDead(site, item.Title, origin, e)
  } finally {
    vodDetailLoading.value = ''
  }
}

async function openVodFavorite(favorite: VodFavoriteInfo) {
  await openVodDetail({
    ID: favorite.VodID,
    Title: favorite.Title,
    Logo: favorite.Logo,
    Group: favorite.Group,
    Site: favorite.Site,
  })
  if (!vodDetail.value) return // openVodDetail 内部已 handleError 且不重抛，守卫失败加载
  await prepareVodResume(favorite.Site, favorite.VodID)
}

// vodSourceDeadSearch 关掉失效弹窗，把剧名填进搜索框并发起全站搜索，
// 让用户换一个能播的站点（搜索页本身就在所有站点里查）。
function vodSourceDeadSearch() {
  vodQuery.value = vodSourceDead.value?.info.vodTitle ?? ''
  vodSourceDead.value = null
  void vodSearch()
}

// vodSourceDeadBack 关掉失效弹窗：从首页记录进来的，详情加载前已经切到详情页，
// 需要回首页；收藏/搜索/列表路径在详情落地前没离开原页面，关掉弹窗即可。
async function vodSourceDeadBack() {
  const origin = vodSourceDead.value?.origin ?? 'list'
  vodSourceDead.value = null
  if (vodSourceDeadBackTarget(origin) !== 'home') return
  vodDetailOrigin.value = 'home'
  await backFromVodDetail()
}

// restoreSearchScroll 把搜索结果列表还原成进详情前的现场：优先按滚动偏移，
// 偏移为 0（列表本来就在顶部，或没记到）时按选中条目居中兜底。
function restoreSearchScroll() {
  const index = searchSelectedIndex.value
  const top = searchScrollTop.value
  if (index < 0 && top <= 0) return
  if (top > 0) searchListRef.value?.restoreScroll(top, index)
  else searchListRef.value?.scrollToIndex(index)
}

async function backFromVodDetail() {
  const target = vodBackTarget(vodDetailOrigin.value)
  await stopPlayback('vod')
  currentVod.value = null
  vodDetail.value = null
  vodResume.value = null
  similarRequest++
  similarNetworkItems.value = []
  similarLoading.value = false
  if (target === 'home') {
    mode.value = 'home'
    await refreshHome()
    return
  }
  if (target === 'search') {
    const cached = vodSearchCache.value
    if (cached) {
      // 无论缓存是否过期，先拿上一次的结果占位并滚回上次选中的影片，
      // 避免返回时列表闪空、跳到顶部。缓存仍有效就直接用，过期则后台静默刷新。
      vodQuery.value = cached.query
      vodSearchItems.value = [...cached.items]
      mode.value = 'search'
      vodView.value = 'search'
      await nextTick()
      restoreSearchScroll()
      if (isVodSearchCacheValid(cached)) return
      await vodSearch(true)
      await nextTick()
      restoreSearchScroll()
      return
    }
    vodQuery.value = vodSearchQueryForReturn(vodSearchCache.value, vodQuery.value.trim())
    await vodSearch()
    return
  }
  if (target === 'favorites') {
    mode.value = 'favorites'
    await loadVodFavorites()
    return
  }
  vodView.value = 'list'
}

async function backFromVodSearch() {
  if (searching.value) {
    await invalidateSearch()
  }
  vodDetail.value = null
  await switchMode('vod')
}

async function doPlayEpisode(site: string, epID: string, epName: string, source: string, seekSeconds = 0, automated = false) {
  await stopPlayback('live')
  // 新一次点播播放：重置自动任务代际、已尝试线路与健康计时，并取消旧预载。
  // 自动化发起的播放（换源/自动切集）不重置 attempted——否则换源循环每次都被
  // 清空、永远到不了「全部线路尝试完」的终止点（表现为无限自动换源）。
  if (!automated) vodAutomation.beginSession()
  void releasePreload()
  const token = beginPlayback('vod')
  vodPlayerLoading.value = true
  // 切集/换源时不预先清空播放计划：清空会卸载 <video>，导致全屏被中断、画面黑屏，
  // 新流就绪后还得用户手动点一次播放。新计划就绪后直接替换；失败时下面的 catch 会清空。
  if (vodPlaybackPlan.value?.Backend !== 'web') vodPlaybackPlan.value = null
  vodPlaybackToken.value = 0
  vodPlaybackStatus.value = 'preparing'
  vodPlaybackError.value = ''
  vodNowPlaying.value = epName
  vodPlaybackPosition.value = seekSeconds
  vodPlaybackDuration.value = 0
  vodSkipRuntime = { introSkipped: false, outroSkipped: false }
  pendingSeek.value = seekSeconds
  activeSource.value = source
  void loadVodSkipMarks(site)
  let plan: PlaybackPlan
  try {
    plan = await ShellService.PrepareVodWithToken(site, epID, token) as unknown as PlaybackPlan
  } catch (e) {
    if (isCurrentPlayback('vod', token)) {
      vodPlaybackPlan.value = null
      vodPlaybackToken.value = 0
      vodPlayerLoading.value = false
      vodPlaybackStatus.value = 'error'
      // 播放失败只在播放器下方呈现，不再 rethrow 弹顶部全局横幅。
      setVodPlaybackError(String(e))
      // 这条线路在准备阶段就起不来（例如指向平台页面）：与播放中出错一致，
      // 自动换下一个同名剧集；自动化发起的尝试由 switchSource 自己续着走。
      if (!automated) vodAutomation.prepareFailed(token)
    }
    return false
  }
  if (!isCurrentPlayback('vod', token)) { await pauseStalePlayback(token); return false }
  vodPlaybackPlan.value = plan
  vodPlaybackToken.value = token
  vodPlaybackStatus.value = 'playing'
  currentEpisodeID.value = epID
  episodePage.value = episodePageIndex(activeEpisodes.value, epID)
  if (vodDetail.value) {
    currentVod.value = { site, vodID: vodDetail.value.ID }
    await ShellService.RecordVodHistory(site, vodDetail.value.ID, vodDetail.value.Title, vodDetail.value.Logo, epID, epName, source)
  }
  vodAutomation.armHealth(token)
  void schedulePreload(token, site, epID, source)
  return true
}

async function playEpisode(ep: EpisodeInfo) {
  errMsg.value = ''
  const seekTo = pickResumeSeek(vodResume.value, ep.ID) // 命中上次那集→从原进度续播
  if (seekTo > 0) vodResume.value = null
  pendingSeek.value = seekTo
  try {
    const played = await doPlayEpisode(detailSite.value || activeSite.value, ep.ID, ep.Name, ep.Source, seekTo)
    if (played) await applyPendingSeek()
  } catch (e) {
    if (isCurrentPlayback('vod', vodPlaybackToken.value)) setVodPlaybackError(String(e))
  }
}

async function fallbackToMpv(scope: PlaybackScope, id: string, token: number, position: number) {
  mpvNeeded.value = true
  if (scope === 'live') {
    livePlaybackStatus.value = 'preparing'
    livePlaybackError.value = ''
  } else {
    vodPlaybackStatus.value = 'preparing'
    vodPlaybackError.value = ''
  }
  try {
    // 直播从头接流（position 传给 mpv 会退化成一次无意义 seek）；点播从已看位置续播，
    // 并取历史续播目标的较大值——网页播放器若在到达续播点前就失败，不能让进度回零。
    const start = scope === 'live' ? 0 : Math.max(position, pendingSeek.value)
    const applied = await resolvePlaybackFallback(
      scope,
      async () => await ShellService.FallbackToMPVWithToken(id, token, start) as unknown as PlaybackPlan,
      () => isCurrentPlayback(scope, token) &&
        (scope === 'live' ? mode.value === 'live' : (mode.value === 'vod' && vodView.value === 'detail')),
      (target, plan) => {
        if (target === 'live') {
          livePlaybackPlan.value = plan
          livePlaybackStatus.value = 'playing'
        } else {
          vodPlaybackPlan.value = plan
          vodPlaybackStatus.value = 'playing'
        }
      },
    )
    if (!applied) await pauseStalePlayback(token)
    else {
      try { await refreshMpvStatus() } catch { /* 播放已回退，状态检测失败不影响播放 */ }
    }
  }
  catch (e) {
    if (isCurrentPlayback(scope, token)) {
      if (scope === 'live') {
        livePlaybackPlan.value = null
        livePlaybackToken.value = 0
        livePlaybackStatus.value = 'error'
        setLivePlaybackError(String(e))
      } else {
        vodPlaybackPlan.value = null
        vodPlaybackToken.value = 0
        vodPlaybackStatus.value = 'error'
        setVodPlaybackError(String(e))
      }
    }
  }
}

async function stopPlayback(scope: PlaybackScope) {
  const ownsPlayer = activePlayback.value?.scope === scope
  const token = ownsPlayer ? activePlayback.value!.token : 0
  if (scope === 'live') {
    livePlaybackPlan.value = null
    livePlaybackToken.value = 0
    liveNowPlaying.value = ''
    livePlaybackStatus.value = 'idle'
    livePlaybackError.value = ''
  } else {
    // 切页/停止：让自动任务失效并取消下一集预载。
    vodAutomation.stop()
    void releasePreload()
    vodPlayerLoading.value = false
    vodPlaybackPlan.value = null
    vodPlaybackToken.value = 0
    vodNowPlaying.value = ''
    currentVod.value = null
    vodPlaybackPosition.value = 0
    vodPlaybackStatus.value = 'idle'
    vodPlaybackError.value = ''
  }
  if (ownsPlayer) {
    activePlayback.value = null
    playbackOwner.value = null
    // 不 await：这一句是 mpv 的 IPC 往返，主循环忙（开流、缓冲、慢源）时要等到
    // 命令应答超时（15s），页面切换就被卡在后面几秒不动。
    // 后端先同步作废 token 再暂停，所以这里放手也不会让旧播放串味：
    // 迟到的 StopPlayback 遇到更新的 token 会自己失效。
    void ShellService.StopPlayback(token).catch(() => {})
  }
}

async function pauseStalePlayback(token: number) {
  if (!shouldPauseStalePlayback(activePlayback.value)) return
  try { await ShellService.StopPlayback(token) } catch { /* 过期播放尚未建立时无需处理 */ }
}

function beginPlayback(scope: PlaybackScope): number {
  const token = ++nextPlaybackToken
  activePlayback.value = { scope, token }
  playbackOwner.value = scope
  return token
}

// vodAutomation 是点播自动切集/自动换源的唯一协调器：它只做决策与编排，
// 实际播放仍走 doPlayEpisode，两边的 token/代际互相校验。
const vodAutomation = new VodAutomation({
  isCurrent: (token) => isCurrentPlayback('vod', token),
  settings: () => playbackSettings.value,
  currentEpisodeID: () => currentEpisodeID.value,
  currentEpisodeName: () => vodNowPlaying.value,
  activeSource: () => activeSource.value,
  currentSourceEpisodes: () => activeEpisodes.value,
  allEpisodes: () => vodDetail.value?.Episodes ?? [],
  sources: () => vodDetail.value?.Sources ?? [],
  position: () => vodPlaybackPosition.value,
  playEpisode: async (episode, seek, automated) => {
    const site = detailSite.value || activeSite.value
    const played = await doPlayEpisode(site, episode.ID, episode.Name, episode.Source, seek, automated)
    if (played) await applyPendingSeek()
    return played
  },
  onAllSourcesFailed: () => {
    vodPlaybackStatus.value = 'error'
    setVodPlaybackError('所有线路均无法播放当前剧集')
  },
} satisfies VodAutomationHost)

// onVodPlaybackSignal 接收 PlaybackView 的标准化信号；组件只上报，决策在这里。
function onVodPlaybackSignal(token: number, state: PlaybackState, message?: string) {
  if (!isCurrentPlayback('vod', token)) return
  if (state === 'ended') {
    void vodAutomation.ended(token)
    return
  }
  if (state === 'error' && message) setVodPlaybackError(message)
  if (state === 'playing' || state === 'error') vodPlayerLoading.value = false
  if (state === 'ready') applySkip(token)
  vodAutomation.signal(token, state)
}

// onPlaybackEvent 消费后端桥接的 mpv 事件：只接受与当前点播 token 一致的事件，
// 旧会话的事件一律丢弃。
function onPlaybackEvent(payload: unknown) {
  const data = payload as { Token?: number; Kind?: string; Position?: number; Duration?: number } | null | undefined
  const token = data?.Token
  if (!token || token !== vodPlaybackToken.value || !isCurrentPlayback('vod', token)) return
  switch (data?.Kind) {
    case 'position':
      vodPlaybackPosition.value = data.Position ?? vodPlaybackPosition.value
      applySkip(token)
      break
    case 'duration':
      // mpv 总时长：片尾跳过与片尾标记显示都用它；未知时保持 0。
      if (data.Duration && data.Duration > 0) vodPlaybackDuration.value = data.Duration
      break
    case 'playing':
    case 'buffering':
    case 'error':
      vodAutomation.signal(token, data.Kind)
      break
    case 'ended':
      void vodAutomation.ended(token)
      break
  }
}

// loadVodSkipMarks 读取当前影片的跳过标记；失败按未标记处理，不影响播放。
async function loadVodSkipMarks(site: string) {
  const vodID = vodDetail.value?.ID
  if (!site || !vodID) return
  try {
    vodSkipMarks.value = await ShellService.GetVodSkipMarks(site, vodID) as unknown as VodSkipMarks
  } catch { vodSkipMarks.value = { IntroEnd: 0, OutroStart: 0 } }
}

// showSkipNotice 在画面上短暂提示跳过结果。
function showSkipNotice(text: string) {
  vodSkipNotice.value = text
  if (skipNoticeTimer !== null) clearTimeout(skipNoticeTimer)
  skipNoticeTimer = setTimeout(() => { vodSkipNotice.value = ''; skipNoticeTimer = null }, 1500)
}

// applySkip 用统一判定决定是否跳片头/片尾；Web 走 seek-to，mpv 直接 Seek。
function applySkip(token: number) {
  if (!isCurrentPlayback('vod', token)) return
  const action = resolveSkipAction(vodPlaybackPosition.value, vodPlaybackDuration.value, vodSkipMarks.value, vodSkipRuntime)
  if (!action) return
  const isWeb = vodPlaybackPlan.value?.Backend === 'web'
  if (action === 'intro') {
    vodSkipRuntime = { ...vodSkipRuntime, introSkipped: true }
    const target = vodSkipMarks.value.IntroEnd
    if (isWeb) pendingSeek.value = target
    else void ShellService.Seek(target).catch(() => {})
    showSkipNotice('已跳过片头')
    return
  }
  vodSkipRuntime = { ...vodSkipRuntime, outroSkipped: true }
  // 跳到结尾以触发 ended → 自动切集。总时长未知（HLS 常见 Infinity）时给一个
  // 超大目标值，由播放器钳到结尾，而不是用当前位置（那等于没跳）。
  // mpv 以前总时长未知，只能 Seek(1e9)；现在 duration 事件到了就用真实总时长。
  const endTarget = vodPlaybackDuration.value > 0 ? vodPlaybackDuration.value : 1e9
  if (isWeb) pendingSeek.value = endTarget
  else void ShellService.Seek(endTarget).catch(() => {})
  showSkipNotice('已跳过片尾')
}

// 标记动作：记录当前位置并持久化；片尾标记当集即生效。
async function markSkipAt(kind: 'intro' | 'outro', position: number) {
  const site = currentVod.value?.site || detailSite.value || activeSite.value
  const vodID = currentVod.value?.vodID || vodDetail.value?.ID
  if (!site || !vodID) return
  const next = { ...vodSkipMarks.value }
  if (kind === 'intro') next.IntroEnd = position
  else next.OutroStart = position
  vodSkipMarks.value = next
  vodSkipRuntime = kind === 'intro' ? { ...vodSkipRuntime, introSkipped: true } : { ...vodSkipRuntime, outroSkipped: false }
  showSkipNotice(kind === 'intro' ? '已标记片头结束' : '已标记片尾开始')
  try { await ShellService.SetVodSkipMarks(site, vodID, next) } catch (e) { handleError(e) }
}

// resetSkipMark 把某一项重置为默认：片头 = 00:00、片尾 = 总时长，都表示不跳过。
async function resetSkipMark(kind: 'intro' | 'outro') {
  const site = currentVod.value?.site || detailSite.value || activeSite.value
  const vodID = currentVod.value?.vodID || vodDetail.value?.ID
  if (!site || !vodID) return
  const next = { ...vodSkipMarks.value }
  if (kind === 'intro') next.IntroEnd = 0
  else next.OutroStart = 0
  vodSkipMarks.value = next
  vodSkipRuntime = kind === 'intro' ? { ...vodSkipRuntime, introSkipped: false } : { ...vodSkipRuntime, outroSkipped: false }
  showSkipNotice(kind === 'intro' ? '片头已重置为 00:00' : '片尾已重置为片尾')
  try { await ShellService.SetVodSkipMarks(site, vodID, next) } catch (e) { handleError(e) }
}

// releasePreload 释放当前下一集预载；切集、换源、停止与卸载都会调用。
async function releasePreload() {
  preloadGeneration++
  const plan = preloadPlan.value
  preloadPlan.value = null
  if (!plan?.ID) return
  try { await ShellService.ReleasePreload(plan.ID) } catch { /* 预载释放失败不影响当前播放 */ }
}

// schedulePreload 在开启预载且当前线路存在下一集时后台预载下一集。
// 预载的任何失败、超时或取消都只影响预载本身。
async function schedulePreload(token: number, site: string, epID: string, source: string) {
  if (!playbackSettings.value.PreloadNext) return
  const next = nextEpisodeInSource(activeEpisodes.value, source, epID)
  if (!next) return
  const generation = ++preloadGeneration
  try {
    const plan = await ShellService.PreloadVod(site, next.ID) as unknown as PlaybackPlan
    if (generation !== preloadGeneration || !isCurrentPlayback('vod', token)) {
      if (plan?.ID) { try { await ShellService.ReleasePreload(plan.ID) } catch { /* 过期预载已无意义 */ } }
      return
    }
    preloadPlan.value = plan
  } catch { /* 预载失败不影响当前播放 */ }
}

function isCurrentPlayback(scope: PlaybackScope, token: number): boolean {
  const active = activePlayback.value
  return !!active && active.scope === scope && active.token === token
}

async function onVodProgress(token: number, time: number, duration: number) {
  if (!isCurrentPlayback('vod', token)) return
  // 进度始终记录到运行时状态，供换源续播使用；持久化仍按 10 秒节流。
  vodPlaybackPosition.value = time
  if (duration > 0) vodPlaybackDuration.value = duration
  applySkip(token)
  if (!shouldRecordVodProgress(mode.value, vodView.value) || !currentVod.value) return
  const now = Date.now()
  if (now - lastProgressSave < 10000) return
  lastProgressSave = now
  try { await ShellService.UpdateVodProgress(currentVod.value.site, currentVod.value.vodID, time, duration) }
  catch { /* 进度保存失败不阻断 */ }
}

async function pollMpvProgress() {
  const token = vodPlaybackToken.value
  if (!isCurrentPlayback('vod', token) || !shouldRecordVodProgress(mode.value, vodView.value) || activePlaybackPlan.value?.Backend !== 'mpv') return
  try {
    const pos = await ShellService.Position()
    if (currentVod.value) {
      await ShellService.UpdateVodProgress(currentVod.value.site, currentVod.value.vodID, pos, 0)
    }
  } catch { /* 忽略 */ }
}

async function loadSearchThreads() {
  try { searchThreads.value = await ShellService.SearchThreads() } catch (e) { handleError(e) }
}

async function loadSearchCacheTTL() {
  try { searchCacheTTL.value = await ShellService.SearchCacheTTLMinutes() } catch (e) { handleError(e) }
}

function openThreads() {
  showThreads.value = true
}

function openCacheTTL() {
  showCacheTTL.value = true
}

async function chooseThreads(n: number) {
  searchThreads.value = n
  try { await ShellService.SetSearchThreads(n) } catch (e) { handleError(e) }
  showThreads.value = false
}

async function chooseCacheTTL(n: number) {
  searchCacheTTL.value = n
  try { await ShellService.SetSearchCacheTTLMinutes(n) } catch (e) { handleError(e) }
  showCacheTTL.value = false
}

async function applyTheme(name: string) {
  currentTheme.value = name
  document.body.dataset.theme = name
  try { await ShellService.SetTheme(name) } catch (e) { handleError(e) }
  void syncTitleBarColour()
  showTheme.value = false
}

// 标题栏跟背景色：窗口顶部那条原生标题栏改成当前主题的背景色，别白一条。
// 失败不打扰（纯点缀；Windows 10 老系统不支持改标题栏颜色）。
async function syncTitleBarColour() {
  try { await ShellService.SetTitleBarColour(titleBarColourForTheme(currentTheme.value)) } catch { /* 忽略 */ }
}

async function applyContentCardStyle(style = contentCardStyle.value) {
  contentCardStyle.value = normalizeContentCardStyle(style)
  try { await ShellService.SetContentCardStyle(contentCardStyle.value) } catch (e) { handleError(e) }
  showContentCardStyle.value = false
}

async function loadTheme() {
  try {
    const theme = await ShellService.GetTheme()
    currentTheme.value = theme || 'default'
    document.body.dataset.theme = currentTheme.value
    void syncTitleBarColour()
  } catch (e) { handleError(e) }
}

async function loadContentCardStyle() {
  try {
    contentCardStyle.value = normalizeContentCardStyle(await ShellService.GetContentCardStyle())
  } catch (e) { handleError(e) }
}

async function checkUpdate() {
  updateMsg.value = '检查中…'
  try {
    updateInfo.value = await ShellService.CheckUpdate()
    updateMsg.value = updateInfo.value.HasUpdate ? `发现新版本 ${updateInfo.value.LatestVersion}` : '已是最新版本'
  } catch (e) {
    updateMsg.value = '检查更新失败'
    handleError(e)
  }
}

function openContentCardStyle() {
  showContentCardStyle.value = true
}

function openTheme() {
  showTheme.value = true
}

onMounted(() => {
  refresh()
  loadTheme()
  loadContentCardStyle()
  void playbackSettingsStore.load()
  // 直播实时字幕开关默认是关的，启动时就要从后端读，否则首次播放前端不抓音频、
  // 一直没有字幕（以前只在进设置页时加载）。
  void liveSubtitleSettingsStore.load()
  document.addEventListener('click', closeCardContextMenu)
  document.addEventListener('keydown', handleCardContextMenuKeydown)
  document.addEventListener('keydown', handleGlobalSpaceKey)
  window.addEventListener('resize', syncSearchHistoryDropdownOnResize)
  Events.On('import:progress', (ev: any) => { importProgress.value = ev.data as Progress })
  Events.On('search:start', (ev: any) => {
    const data = ev.data as { ID: number; Token: number; Query: string }
    if (searching.value && data.Token === searchRequest.value && data.Query === activeSearchQuery.value && data.ID > searchFloorID.value) activeSearchID.value = data.ID
  })
  Events.On('search:progress', (ev: any) => {
    const data = ev.data as Progress & { ID?: number; Token?: number; Query?: string }
    if (searching.value && data.Token === searchRequest.value && data.ID !== undefined && data.ID > searchFloorID.value && data.Query === activeSearchQuery.value) {
      if (activeSearchID.value === 0) activeSearchID.value = data.ID
      if (data.ID === activeSearchID.value) searchProgress.value = data
    }
  })
  Events.On('search:result', (ev: any) => {
    const data = ev.data as { ID: number; Token: number; Query: string; Items: VodItem[] }
    if (searching.value && data.Token === searchRequest.value && data.ID > searchFloorID.value && data.Query === activeSearchQuery.value) {
      if (activeSearchID.value === 0) activeSearchID.value = data.ID
      if (data.ID !== activeSearchID.value) return
      // 积压进缓冲，~120ms 合并渲染一次：站点批量秒回时逐事件排序+重渲染
      // 数千张卡片会占死事件循环，连进度数字都跟着变卡。
      searchResultPending.push(...(data.Items ?? []))
      if (searchResultTimer === null) searchResultTimer = setTimeout(flushPendingSearchResults, 120)
    }
  })
  // mpv 播放事件由后端带 token 桥接过来；onPlaybackEvent 只接受当前点播会话的事件。
  Events.On('playback:event', (ev: any) => onPlaybackEvent(ev?.data))
  // 直播实时字幕：后端推译文，前端只显示；切台时后端会清残留，这里也清。
  Events.On('live-subtitle', (ev: any) => {
    const data = ev.data as { Text?: string; LatencyMs?: number } | undefined
    if (typeof data?.Text === 'string' && data.Text.trim() !== '') {
      liveSubtitleText.value = data.Text
    }
  })
  setInterval(pollMpvProgress, 10000)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', closeCardContextMenu)
  document.removeEventListener('keydown', handleCardContextMenuKeydown)
  document.removeEventListener('keydown', handleGlobalSpaceKey)
  window.removeEventListener('resize', syncSearchHistoryDropdownOnResize)
  vodAutomation.stop()
  if (liveToastTimer !== null) clearTimeout(liveToastTimer)
  void releasePreload()
})

</script>

<template>
  <main class="container">
    <header class="topnav" :class="{ solid: mode !== 'home' || navScrolled }">
      <div class="logo" :title="platform + ' · 播放器' + (playerReady ? '就绪' : '未就绪')" @click="switchMode('home')">
        <img class="brand-logo" src="/appicon.png" alt="UnBox" />
        <span class="logo-text">UNBOX</span>
      </div>

      <nav class="nav-links" aria-label="主导航">
        <button :class="{ active: navActive === 'home' }" @click="switchMode('home')">首页</button>
        <button v-for="cat in TOP_NAV_CATEGORIES" :key="cat.key" :class="{ active: navActive === cat.key }" @click="goTopNavCategory(cat)">{{ cat.label }}</button>
        <button :class="{ active: navActive === 'live' }" @click="switchMode('live')">直播</button>
        <button :class="{ active: navActive === 'favorites' }" @click="switchMode('favorites')">我的片单</button>
      </nav>

      <div class="nav-tools">
        <button class="icon-btn" type="button" aria-label="搜索" title="搜索" @click="switchMode('search')">
          <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M15.5 14h-.79l-.28-.27a6.5 6.5 0 1 0-.7.7l.27.28v.79l5 4.99L20.49 19zm-6 0A4.5 4.5 0 1 1 14 9.5 4.5 4.5 0 0 1 9.5 14z" /></svg>
        </button>
        <button class="icon-btn" type="button" aria-label="设置" title="设置" @click="switchMode('settings')">
          <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M19.14 12.94a7.07 7.07 0 0 0 0-1.88l2.03-1.58a.5.5 0 0 0 .12-.64l-1.92-3.32a.5.5 0 0 0-.59-.22l-2.39.96a7.3 7.3 0 0 0-1.62-.94l-.36-2.54a.5.5 0 0 0-.5-.42h-3.84a.5.5 0 0 0-.5.42l-.36 2.54a7.3 7.3 0 0 0-1.62.94l-2.39-.96a.5.5 0 0 0-.59.22L2.65 8.84a.5.5 0 0 0 .12.64l2.03 1.58a7.07 7.07 0 0 0 0 1.88l-2.03 1.58a.5.5 0 0 0-.12.64l1.92 3.32c.14.24.42.34.59.22l2.39-.96c.49.38 1.03.7 1.62.94l.36 2.54c.05.24.25.42.5.42h3.84c.25 0 .45-.18.5-.42l.36-2.54a7.3 7.3 0 0 0 1.62-.94l2.39.96c.22.1.45 0 .59-.22l1.92-3.32a.5.5 0 0 0-.12-.64zM12 15.6A3.6 3.6 0 1 1 15.6 12 3.6 3.6 0 0 1 12 15.6z" /></svg>
        </button>
        <button class="avatar" type="button" title="设置" aria-label="设置" @click="switchMode('settings')">N</button>
      </div>
    </header>

    <div class="app-main">
    <div v-if="showMpvInstallPrompt" class="mpv-install" aria-live="polite">
      <span>{{ mpvInstallHint(platform) }}</span>
      <button @click="installMpv">{{ mpvInstallMode === 'download' ? '下载并安装 mpv' : '显示安装命令' }}</button>
      <button v-if="mpvInstallMode && mpvInstallMode !== 'download'" @click="recheckMpv">我已安装，重新检测</button>
      <p v-if="installMessage" class="install-cmd">{{ installMessage }}</p>
    </div>

    <!-- 首页：全幅横幅 + 横向海报行（全部来自真实数据） -->
    <section v-if="mode === 'home'" class="home" @scroll="onHomeScroll">
      <div v-if="heroItem" class="home-hero">
        <div class="home-hero-bg" aria-hidden="true">
          <img v-if="heroItem.VodLogo && !heroLogoFailed" :src="heroItem.VodLogo" alt="" @error="heroLogoFailed = true" />
          <span v-else class="home-hero-fallback" :style="{ background: posterGradient(heroItem.VodTitle) }"></span>
        </div>
        <div class="home-hero-body">
          <div class="home-hero-info">
            <p class="home-hero-kicker">继续观看</p>
            <h2>{{ heroItem.VodTitle }}</h2>
            <p v-if="homeCardItems[0]" class="home-hero-meta">{{ contentCardSubtitle(homeCardItems[0]) }}</p>
            <div v-if="heroItem.Duration" class="home-hero-progress"><span :style="{ width: (heroProgress * 100) + '%' }"></span></div>
            <div class="home-hero-actions">
              <button class="home-hero-play" type="button" @click="resumeVod(heroItem)">▶ 继续播放{{ heroItem.EpName ? ' ' + heroItem.EpName : '' }}</button>
              <button class="home-hero-more" type="button" @click="openHeroDetail">更多信息</button>
            </div>
          </div>
        </div>
      </div>

      <div v-if="homeMainCats.length" class="home-cats">
        <span class="home-cats-title">分类</span>
        <button v-for="c in homeMainCats" :key="c.ID" type="button" class="home-cat-chip" @click="goVodCategory(c.ID)">{{ c.Title }}</button>
      </div>

      <div class="home-rows">
        <section v-if="homeHistory.length" class="netflix-row-section">
          <h2>继续观看 <span class="chev">›</span></h2>
          <div class="netflix-row-wrap">
            <button class="netflix-row-arrow prev" type="button" aria-label="向左滚动" @click="scrollRow($event, -1)">‹</button>
            <ContentCardList
              list-class="home-list home-continue netflix-row netflix-row-wide"
              :items="homeCardItems"
              :style="'grid'"
              deletable
              :active-delete-key="activeDeleteKey"
              @select="(i) => resumeVod(homeHistory[i])"
              @contextmenu="(i, event) => openCardContextMenu('history', homeHistory[i], event)"
              @remove="(i) => deleteHomeHistory(homeHistory[i])"
              @request-delete="requestDeleteContextCard"
            />
            <button class="netflix-row-arrow next" type="button" aria-label="向右滚动" @click="scrollRow($event, 1)">›</button>
          </div>
        </section>

        <section v-if="featuredRowItems.length" class="netflix-row-section">
          <h2>为你精选 <span class="chev">›</span></h2>
          <div class="netflix-row-wrap">
            <button class="netflix-row-arrow prev" type="button" aria-label="向左滚动" @click="scrollRow($event, -1)">‹</button>
            <ContentCardList
              list-class="netflix-row"
              :items="featuredRowItems"
              :style="'grid'"
              @select="(i) => openVodDetail(vodListItems[i])"
            />
            <button class="netflix-row-arrow next" type="button" aria-label="向右滚动" @click="scrollRow($event, 1)">›</button>
          </div>
        </section>

        <section v-if="favoriteRowItems.length" class="netflix-row-section">
          <h2>我的片单 <span class="chev">›</span></h2>
          <div class="netflix-row-wrap">
            <button class="netflix-row-arrow prev" type="button" aria-label="向左滚动" @click="scrollRow($event, -1)">‹</button>
            <ContentCardList
              list-class="netflix-row"
              :items="favoriteRowItems"
              :style="'grid'"
              @select="(i) => openVodFavorite(vodFavorites[i])"
            />
            <button class="netflix-row-arrow next" type="button" aria-label="向右滚动" @click="scrollRow($event, 1)">›</button>
          </div>
        </section>

        <p v-if="!homeHistory.length && !vodListItems.length && !vodFavorites.length" class="home-empty">暂无观看记录，去「点播」看看吧</p>
      </div>
    </section>

    <!-- 点播搜索 -->
    <section v-if="mode === 'search'" class="search-page">
      <div class="search-page-head">
        <button class="vod-back" type="button" @click="backFromVodSearch">← 点播</button>
        <form class="search" @submit.prevent="vodSearch()">
          <!-- 进度浮在输入框右端（绝对定位）：不参与布局，搜索时输入框尺寸不变，
               既不会挤压输入框，也不会让下面的列表上下跳。 -->
          <span class="search-field">
            <input v-model="vodQuery" placeholder="搜索影片" :class="{ 'with-inline-progress': !!searchProgress }" />
            <span v-if="searchProgress" class="search-inline-progress" :title="searchProgress.Message">{{ searchProgress.Message }}</span>
          </span>
          <button type="submit">搜索</button>
          <button v-if="searching" type="button" @click="cancelSearch">取消</button>
        </form>
      </div>
      <div v-if="vodSearchHistory.length" class="search-history">
        <span class="search-history-title">历史搜索</span>
        <span ref="searchHistoryInline" class="search-history-inline">
          <span v-for="term in vodSearchHistory" :key="term" class="search-history-item">
            <button type="button" @click="useVodSearchHistory(term)">{{ term }}</button>
            <button type="button" class="row-delete" title="删除搜索词" @click="deleteVodSearchHistory(term)">×</button>
          </span>
        </span>
        <button type="button" class="search-history-toggle" @click="toggleSearchHistory" :title="searchHistoryExpanded ? '折叠' : '展开全部'"><span class="caret">›</span></button>
        <div v-if="searchHistoryExpanded" class="history-dropdown" :style="searchHistoryDropdownStyle">
          <span v-for="term in vodSearchHistory" :key="'d-' + term" class="search-history-item">
            <button type="button" @click="useVodSearchHistory(term)">{{ term }}</button>
            <button type="button" class="row-delete" title="删除搜索词" @click="deleteVodSearchHistory(term)">×</button>
          </span>
        </div>
      </div>
      <p v-if="showVodNoResults" class="home-empty">暂无搜索结果</p>
      <p v-if="completedSearchQuery && !searching && vodSearchItems.length" class="search-count">找到 {{ vodSearchItems.length }} 条结果</p>
      <p v-if="searching && !vodSearchItems.length" class="search-loading">正在搜索…</p>
      <section class="vod-main search-results">
        <ContentCardList
          ref="searchListRef"
          :items="vodCardItems"
          :style="'grid'"
          :divider-after="searchStrongCount"
          divider-label="弱相关结果"
          @select="(i) => openVodDetail(vodSearchItems[i])"
        />
      </section>
    </section>

    <!-- 点播收藏 -->
    <section v-if="mode === 'favorites'" class="favorites-page">
      <h2>点播收藏</h2>
      <p v-if="!vodFavorites.length" class="home-empty">暂无点播收藏</p>
      <ContentCardList
        v-else
        list-class="favorites-list"
        :items="favoriteCardItems"
        :style="contentCardStyle"
        deletable
        :active-delete-key="activeDeleteKey"
        @select="(i) => openVodFavorite(vodFavorites[i])"
        @contextmenu="(i, event) => openCardContextMenu('favorite', vodFavorites[i], event)"
        @remove="(i) => deleteVodFavorite(vodFavorites[i])"
        @request-delete="requestDeleteContextCard"
      />
    </section>

    <!-- 直播 -->
    <section v-if="mode === 'live'" class="layout live">
      <section class="channels">
        <div v-if="groups.length <= 1" class="load-live">
          <p>直播源尚未加载</p>
          <button @click="loadLive">加载直播</button>
          <span v-if="importProgress && importProgress.Stage === 'live'" class="progress">{{ importProgress.Message }}</span>
        </div>
        <form class="search" @submit.prevent="doSearch"><input v-model="query" placeholder="搜索频道" /><button type="submit">搜索</button></form>
        <div class="live-controls">
          <ThemedSelect :model-value="activeGroup" :options="liveGroupOptions" aria-label="分类" @update:model-value="selectLiveGroup" />
          <button type="button" class="live-fav-toggle" :class="{ active: liveFavOnly }" :aria-pressed="liveFavOnly" title="只看收藏" @click="toggleLiveFavorites">{{ liveFavOnly ? '★' : '☆' }}</button>
        </div>
        <ul>
          <li v-for="c in channels" :key="c.ID" class="channel" @click="play(c)">
            <span class="name" :title="c.Name"><span class="name-text">{{ c.Name }}</span></span>
            <button :title="c.Favorited ? '取消收藏' : '收藏'" :aria-label="c.Favorited ? '取消收藏' : '收藏'" @click.stop="toggleFav(c)">{{ c.Favorited ? '★' : '☆' }}</button>
          </li>
        </ul>
      </section>

      <aside class="player">
        <p v-if="liveNowPlaying" class="now">正在播放：{{ liveNowPlaying }}</p>
        <p v-if="livePlaybackStatus === 'error'" class="playback-error" aria-live="assertive">频道播放失败：{{ livePlaybackError }}</p>
        <PlaybackView ref="livePlayerRef" :plan="livePagePlaybackPlan" :mpv-paused="mpvPaused" :translate-enabled="translateSettings.Enabled" :translate-text="translateSubtitleText" :live-subtitle="liveSubtitleText" :live-subtitle-enabled="liveSubtitleSettings.Enabled" :live-audio-pusher="liveAudioPusher" @fallback="(id, position) => fallbackToMpv('live', id, livePlaybackToken, position)" @toggle-pause="toggleMpvPause" />
      </aside>

      <div v-if="liveToast" class="app-toast" role="status" aria-live="polite">{{ liveToast }}</div>
    </section>

    <!-- 点播 -->
    <section v-if="mode === 'vod'" class="vod">
      <div v-if="vodView === 'list'" class="vod-toolbar">
        <ThemedSelect v-if="vodLines.length > 1" v-model="activeLine" :options="vodLineOptions" aria-label="线路" @update:model-value="selectLine" />
      </div>

      <!-- 点播筛选 chip 行：点选即应用，不再弹层确认。 -->
      <div v-if="vodView === 'list' && vodFilterGroups.length" class="vod-filter-bar">
        <VodFilterPanel :model-value="vodFilterDraft" :groups="vodFilterGroups" @update:model-value="onFilterChipPick" />
        <button v-if="hasActiveFilter(vodFilterActive)" type="button" class="vod-filter-clear" @click="clearVodFilter">清除筛选</button>
      </div>

      <div v-if="vodCategoryLoading" class="app-toast" role="status" aria-live="polite">正在加载分类…</div>
      <!-- 顶部只留状态；完整错误文本统一在底部错误条展示一次，避免上下重复。 -->
      <p v-else-if="vodCategoryError" class="vod-state vod-state-error" aria-live="assertive">分类加载失败</p>

      <div class="vod-layout">
        <section class="vod-main">
          <!-- 点播列表三态都在内容区内明示，不再静默空白：加载中显示进度，
               失败显示真实原因并给重试入口，空结果也给重试（源站可能刚恢复）。 -->
          <p v-if="vodListLoading && !vodListItems.length" class="vod-state" role="status" aria-live="polite">正在加载…</p>
          <div v-else-if="vodListError && !vodListItems.length" class="vod-state vod-state-error" role="alert">
            <span>加载失败：{{ vodListError }}</span>
            <button type="button" class="vod-list-more" @click="reloadVodList()">重新加载</button>
          </div>
          <p v-else-if="!vodListItems.length && vodListEnd" class="vod-state" role="status">该分类暂无内容 <button type="button" class="vod-list-more" @click="reloadVodList()">重新加载</button></p>
          <!-- 注意：不要用裸 <template> 包 ContentCardList + footer——实测裸 template
               在这里不渲染子节点（列表取到 60 条也空白），两者直接挂在 section 下。 -->
          <ContentCardList
            ref="vodListRef"
            :items="vodCardItems"
            :style="contentCardStyle"
            @wheel="onVodListWheel"
            @at-bottom-change="onVodListScroll"
            @select="(i) => openVodDetail(visibleVodItems[i])"
          />
          <div class="vod-list-footer">
            <!-- 已加载完且真实滑到最底就显示；没有内容时也算到底（列表无需滚动）。
                 用 visibility 占位显示，避免出现/隐藏时改变 footer 高度导致滚动位置抖动闪烁。 -->
            <span v-if="vodListEnd" class="vod-list-end" :class="{ visible: vodListAtBottom }">已经到达最底部</span>
            <!-- 只有一页或没有内容时不出现按钮，只留分割线；翻过多页到底才保留禁用按钮。 -->
            <div v-if="vodHasMore || vodPage > 1" class="vod-list-actions">
              <button
                type="button"
                class="vod-list-more"
                :disabled="vodListLoading || vodListEnd"
                @click="loadNextVodPage"
              >{{ vodListEnd ? '已加载完' : vodListLoading ? '正在加载…' : '继续加载' }}</button>
              <button v-if="vodListItems.length" type="button" class="vod-list-top" @click="scrollVodListTop">回到顶部</button>
            </div>
          </div>
        </section>
      </div>

    </section>

    <!-- 设置 -->
    <section v-if="mode === 'settings'" class="settings-page">
      <h2 class="settings-title">设置</h2>
      <p v-if="importProgress && importProgress.Stage !== 'live'" class="progress">{{ importProgress.Message }}</p>
      <p v-if="importSummary" class="ok">{{ importSummary }}</p>

      <section class="src-section">
        <h3>点播源</h3>
        <form class="src-add" @submit.prevent="importVodSource">
          <input v-model="vodSourceUrl" placeholder="粘贴点播源地址" :disabled="importing" />
          <button type="submit" :disabled="importing">{{ importingKind === 'vod' ? '导入中…' : '导入' }}</button>
          <span class="history-wrap">
            <button type="button" @click="showVodHistory = !showVodHistory">{{ showVodHistory ? '收起历史' : '历史配置' }}</button>
          </span>
          <div v-if="showVodHistory && vodSources.length" class="history-dropdown">
            <span v-for="s in vodSources" :key="'vod-' + s.Ref" class="src-history-item">
              <span class="src-ref" :class="{ current: s.Ref === vodSources[0]?.Ref }" @click="reimportSource('vod', s.Ref)" @mouseenter="scrollxEnter" @mouseleave="scrollxLeave"><span class="scrollx-inner">{{ s.Ref }}</span></span>
              <button type="button" class="src-del" @click="deleteSource('vod', s.Ref)">删除</button>
            </span>
          </div>
        </form>
        <div class="src-add">
          <label>Cloudflare 点播源
            <ThemedSelect :model-value="cfVodSourceChoice" :options="cfVodSourceOptions" aria-label="Cloudflare 点播源" @update:model-value="onCfVodSourceSelect" />
          </label>
          <button type="button" :disabled="importing" @click="refreshCfVodDirectory">刷新目录</button>
          <button type="button" :disabled="importing" @click="refreshCfVodSource">刷新源</button>
        </div>
        <div v-if="cfVodSourceChoice === CF_VOD_SOURCE_MANUAL" class="src-add">
          <label>手动填地址 <input v-model="cfVodUrl" type="text" placeholder="https://live.ciweihaha.win/vod.json" :disabled="importing" /></label>
          <button type="button" :disabled="importing || !isValidCfVodURL(cfVodUrl)" @click="saveCfVodUrl">保存并拉取</button>
        </div>
        <div class="src-add">
          <label>点播源目录地址 <input v-model="cfVodDirectoryUrl" type="text" placeholder="https://live.ciweihaha.win/vod-sources.json" :disabled="importing" /></label>
          <button type="button" :disabled="importing || !isValidCfVodURL(cfVodDirectoryUrl)" @click="saveCfVodDirectoryUrl">保存目录地址</button>
        </div>
        <p class="home-empty">下拉框里的源从目录地址自动拉取，有新增的源就自动出现；选了就直接拉取，不用改本地文件。目录拉不到时用内置的几个源，也能手动填地址。</p>
        <p v-if="cfVodDirectoryMsg" class="home-empty">{{ cfVodDirectoryMsg }}</p>
        <p v-if="vodSourceStatus" class="home-empty">当前来源：{{ vodSourceKindLabel(vodSourceStatus.Kind) }} · {{ vodSourceStatus.Sites }} 个站点<span v-if="vodSourceStatus.Kind === 'cloudflare' && vodSourceStatus.UpdatedAt"> · 上次拉取 {{ new Date(vodSourceStatus.UpdatedAt * 1000).toLocaleString() }}</span></p>
        <div class="src-add">
          <button type="button" :disabled="importing || !vodSourceStatus?.LocalRef" @click="useLocalVodSource">用本地源</button>
          <button type="button" :disabled="importing || !vodSourceStatus?.CloudflareURL" @click="refreshCfVodSource">用 Cloudflare 源</button>
        </div>
      </section>

      <section class="src-section">
        <h3>直播源</h3>
        <form class="src-add" @submit.prevent="importLiveSource">
          <input v-model="liveSourceUrl" placeholder="粘贴直播源地址（M3U/TXT/订阅）" :disabled="importing" />
          <button type="submit" :disabled="importing">{{ importingKind === 'live' ? '导入中…' : '导入' }}</button>
          <span class="history-wrap">
            <button type="button" @click="showLiveHistory = !showLiveHistory">{{ showLiveHistory ? '收起历史' : '历史配置' }}</button>
          </span>
          <div v-if="showLiveHistory && liveSources.length" class="history-dropdown">
            <span v-for="s in liveSources" :key="'live-' + s.Ref" class="src-history-item">
              <span class="src-ref" :class="{ current: s.Ref === liveSources[0]?.Ref }" @click="reimportSource('live', s.Ref)" @mouseenter="scrollxEnter" @mouseleave="scrollxLeave"><span class="scrollx-inner">{{ s.Ref }}</span></span>
              <button type="button" class="src-del" @click="deleteSource('live', s.Ref)">删除</button>
            </span>
          </div>
        </form>
      </section>

      <section class="src-section">
        <h3>搜索</h3>
        <div class="src-add">
          <button @click="openThreads">搜索线程：{{ searchThreads }} 个</button>
          <button @click="openCacheTTL">搜索缓存：{{ searchCacheTTL }} 分钟</button>
        </div>
      </section>

      <section class="src-section">
        <h3>证书豁免</h3>
        <p class="settings-hint">部分源站证书过期或使用自签证书，加入列表后会在证书校验失败时跳过校验重试一次。仅对信任的站点开启。</p>
        <form class="src-add" @submit.prevent="addInsecureTLSHost">
          <input v-model="insecureTLSInput" placeholder="例如 haiwaikan.com" />
          <button type="submit">添加</button>
          <button type="button" @click="showTLSHistory = !showTLSHistory">{{ showTLSHistory ? '收起历史' : '豁免历史' }}</button>
          <div v-if="showTLSHistory && insecureTLSHosts.length" class="history-dropdown">
            <span v-for="host in insecureTLSHosts" :key="'tls-' + host" class="src-history-item">
              <span class="src-ref">{{ host }}</span>
              <button type="button" class="src-del" @click="removeInsecureTLSHost(host)">移除</button>
            </span>
          </div>
        </form>
        <p v-if="insecureTLSError" class="src-import-error">{{ insecureTLSError }}</p>
      </section>

      <section class="src-section">
        <h3>个性化</h3>
        <div class="settings-personalize">
          <button type="button" class="settings-choice" @click="openContentCardStyle">内容展示样式：{{ contentCardStyleLabel(contentCardStyle) }}</button>
          <button type="button" class="settings-choice" @click="openTheme">主题：{{ themeOptions.find(t => t.id === currentTheme)?.label || '默认' }}</button>
        </div>
      </section>

      <section class="src-section">
        <h3>播放设置</h3>
        <div class="settings-personalize">
          <button
            v-for="item in PLAYBACK_SETTING_ITEMS"
            :key="item.key"
            type="button"
            class="settings-choice"
            :class="{ active: playbackSettings[item.key] }"
            :title="item.hint"
            :aria-pressed="playbackSettings[item.key]"
            @click="togglePlaybackSetting(item.key, !playbackSettings[item.key])">{{ item.label }}：{{ playbackSettings[item.key] ? '开' : '关' }}</button>
        </div>
      </section>

      <section class="src-section">
        <h3>字幕翻译</h3>
        <p class="home-empty">把英文字幕实时翻成中文（接口失败时显示原文，不影响播放）。免费接口要自己去微软免费开个 Key 贴进来，不收费；也可以用 Cloudflare 免费层。</p>
        <div class="settings-personalize">
          <button type="button" class="settings-choice" :class="{ active: translateSettings.Provider === 'free' }" title="用免费接口翻译" :aria-pressed="translateSettings.Provider === 'free'" @click="setTranslateProvider('free')">免费接口</button>
          <button type="button" class="settings-choice" :class="{ active: translateSettings.Provider === 'off' }" title="不翻译，显示英文原文" :aria-pressed="translateSettings.Provider === 'off'" @click="setTranslateProvider('off')">关闭</button>
          <button type="button" class="settings-choice" @click="checkTranslateModel">检测接口</button>
        </div>
        <div v-if="translateSettings.Provider === 'free'" class="settings-personalize">
          <button type="button" class="settings-choice" :class="{ active: translateSettings.FreeProvider === 'azure' }" @click="setTranslateFreeProvider('azure')">微软 Azure</button>
          <button type="button" class="settings-choice" :class="{ active: translateSettings.FreeProvider === 'cloudflare' }" @click="setTranslateFreeProvider('cloudflare')">Cloudflare</button>
        </div>
        <div v-if="translateSettings.Provider === 'free' && translateSettings.FreeProvider === 'azure'" class="src-add">
          <label>API Key <input v-model="translateFreeAPIKey" type="password" placeholder="Azure Translator 的 Key（F0 免费层）" /></label>
          <label>区域（可选） <input v-model="translateAzureRegion" type="text" placeholder="如 eastasia，留空也行" /></label>
        </div>
        <div v-if="translateSettings.Provider === 'free' && translateSettings.FreeProvider === 'cloudflare'" class="src-add">
          <label>账号 ID <input v-model="translateCFAccountID" type="text" placeholder="Cloudflare 账号 ID" /></label>
          <label>API 令牌 <input v-model="translateCFToken" type="password" placeholder="有 Workers AI 权限的 API 令牌" /></label>
        </div>
        <p v-if="translateSettings.Provider === 'free' && translateSettings.FreeProvider === 'azure'" class="home-empty">Key 在 Azure 门户免费开 Translator 资源（F0 层，每月 200 万字符免费）获取，只存在本机。</p>
        <p v-if="translateSettings.Provider === 'free' && translateSettings.FreeProvider === 'cloudflare'" class="home-empty">令牌在 Cloudflare 控制台 → API 令牌 → 创建令牌（选 Workers AI 模板）获取，只存在本机。免费层每天 10000 neurons，字幕短句够用。</p>
        <p v-if="translateModelMsg" class="home-empty">{{ translateModelMsg }}</p>
      </section>

      <section class="src-section">
        <h3>直播实时字幕</h3>
        <p class="home-empty">看直播时把英文说话实时听写并翻成中文显示（Whisper 听写 + 字幕翻译里选的免费接口翻译；首次使用会下载识别程序与模型并缓存，不打进安装包）。</p>
        <div class="settings-personalize">
          <button type="button" class="settings-choice" :class="{ active: liveSubtitleSettings.Enabled }" title="开启后，看英文直播时画面下方实时出中文字幕" :aria-pressed="liveSubtitleSettings.Enabled" @click="toggleLiveSubtitleSetting(!liveSubtitleSettings.Enabled)">直播实时字幕：{{ liveSubtitleSettings.Enabled ? '开' : '关' }}</button>
          <button type="button" class="settings-choice" @click="refreshLiveSubtitleStatus">查看状态</button>
        </div>
        <div v-if="liveSubtitleSettings.Enabled" class="src-add">
          <label>识别模型 <input v-model="liveSubtitleSettings.WhisperModel" type="text" placeholder="base.en" @change="() => liveSubtitleSettingsStore.setWhisperModel(liveSubtitleSettings.WhisperModel)" /></label>
          <label>窗口（秒） <input v-model.number="liveSubtitleSettings.WindowSeconds" type="number" min="1" max="5" step="0.5" @change="() => liveSubtitleSettingsStore.setWindowSeconds(liveSubtitleSettings.WindowSeconds)" /></label>
        </div>
        <p v-if="liveSubtitleStatusMsg" class="home-empty">{{ liveSubtitleStatusMsg }}</p>
      </section>

      <section class="src-section">
        <h3>关于</h3>
        <div class="src-add about-actions">
          <button @click="checkUpdate">检查更新</button>
          <button @click="openAbout">关于我们</button>
          <button @click="showDisclaimer = true">免责条款</button>
        </div>
        <p v-if="updateMsg" class="home-empty">{{ updateMsg }}</p>
        <div v-if="updateInfo?.HasUpdate && updateInfo.URL" class="src-add">
          <button @click="openURL(updateInfo.URL)">点击下载新版本</button>
        </div>
      </section>

    </section>
    </div>

    <!-- 点播详情整页：上半部分左是播放区、右是海报+介绍，下半部分是选集网格 -->
    <div v-if="mode === 'vod' && vodView === 'detail' && vodDetail" class="vod-detail-page">
      <div class="vod-detail">
        <VodDetailHeader @back="backFromVodDetail" />
        <div class="vod-detail-top">
          <div class="vod-detail-player">
            <p v-if="vodNowPlaying" class="vod-now-playing">正在播放：{{ vodNowPlaying }}</p>
            <p v-if="vodPlaybackStatus === 'error'" class="playback-error" aria-live="assertive">剧集播放失败：{{ vodPlaybackError }}</p>
            <PlaybackView ref="vodPlayerRef" :plan="vodPagePlaybackPlan" :seek-to="pendingSeek" :loading="vodPlayerLoading" :skip-marks="vodSkipMarks" :mpv-paused="mpvPaused" @mark-intro="(p) => markSkipAt('intro', p)" @mark-outro="(p) => markSkipAt('outro', p)" @reset-intro="resetSkipMark('intro')" @reset-outro="resetSkipMark('outro')" :suppress-fallback="playbackSettings.AutoSwitchSource" :translate-enabled="translateSettings.Enabled" :translate-text="translateSubtitleText" @fallback="(id, position) => fallbackToMpv('vod', id, vodPlaybackToken, position)" @playback="(state, message) => onVodPlaybackSignal(vodPlaybackToken, state, message)" @progress="(time, duration) => onVodProgress(vodPlaybackToken, time, duration)" @toggle-pause="toggleMpvPause" />
            <PreloadView :plan="preloadPlan" />
            <div v-if="vodSkipNotice" class="skip-notice">{{ vodSkipNotice }}</div>
          </div>
          <div class="vod-detail-info">
            <div class="vod-detail-poster-row">
              <span class="vod-detail-poster">
                <img v-if="vodDetail.Logo && !detailLogoFailed" :src="vodDetail.Logo" class="poster" referrerpolicy="no-referrer" @error="detailLogoFailed = true" />
                <span v-else class="content-card-fallback" :style="{ background: posterGradient(vodDetail.Title) }" aria-hidden="true">{{ vodDetail.Title.slice(0, 1) }}</span>
              </span>
              <div class="vod-detail-poster-meta">
                <h2>{{ vodDetail.Title }}</h2>
                <p v-if="vodDetail.Type || vodDetail.Year || vodDetail.Area" class="meta">{{ [vodDetail.Type, vodDetail.Year, vodDetail.Area].filter(Boolean).join(' · ') }}</p>
                <div class="vod-detail-actions">
                  <p v-if="vodDetail.Remarks" class="vod-detail-remarks">{{ vodDetail.Remarks }}</p>
                  <button class="favorite-toggle" type="button" :aria-pressed="vodFavorited" @click="toggleVodFavorite">{{ vodFavorited ? '取消收藏' : '收藏' }}</button>
                </div>
              </div>
            </div>
            <div class="desc" :class="{ expanded: descExpanded }" v-html="vodDetail.Description"></div>
            <button v-if="vodDetail.Description" class="desc-toggle" type="button" @click="descExpanded = !descExpanded">{{ descExpanded ? '收起简介' : '展开简介' }}</button>
          </div>
        </div>
        <section class="vod-detail-episodes">
          <div v-if="(vodDetail.Sources ?? []).length" class="ep-src-tabs">
            <button v-for="src in vodDetail.Sources" :key="src" :class="{ active: src === activeSource }" @click="selectEpisodeSource(src)">{{ src }}</button>
          </div>
          <div v-if="episodeRanges.length > 1" class="ep-pagination-wrap" role="navigation" aria-label="剧集分页">
            <button class="ep-pagination-arrow" type="button" aria-label="向左滚动剧集分页" title="向左滚动" @click="scrollEpisodePages(-1)">‹</button>
            <div ref="episodePagination" class="ep-pagination">
              <button v-for="(range, index) in episodeRanges" :key="range.start" type="button"
                      :class="{ active: index === episodePage }" :aria-current="index === episodePage ? 'page' : undefined"
                      @click="selectEpisodePage(index)">{{ range.start }}~{{ range.end }}集</button>
            </div>
            <button class="ep-pagination-arrow" type="button" aria-label="向右滚动剧集分页" title="向右滚动" @click="scrollEpisodePages(1)">›</button>
          </div>
          <div class="ep-list">
            <button v-for="ep in visibleEpisodes" :key="ep.ID" :class="{ active: ep.ID === currentEpisodeID }"
                    @click="playEpisode(ep)">{{ ep.Name }}</button>
          </div>
        </section>
        <section class="vod-similar">
          <h3>更多类似内容</h3>
          <p v-if="similarLoading" class="home-empty">正在全网找同类…</p>
          <ContentCardList
            v-else-if="similarItems.length"
            list-class="netflix-row"
            :items="similarItems"
            :style="'grid'"
            @select="(i) => openSimilarVodDetail(similarNetworkItems[i])"
          />
          <p v-else class="home-empty">全网没找到同类内容</p>
        </section>
      </div>
    </div>

    <div v-if="showThreads" class="settings-overlay" @click.self="showThreads = false">
      <div class="settings-panel">
        <div class="settings-head">
          <h2>搜索线程</h2>
          <button @click="showThreads = false">✕</button>
        </div>
        <p class="threads-hint">全站搜索时同时请求的站点数，站点多时调大可加速</p>
        <div class="threads-options">
          <button v-for="n in [1, 4, 8, 16]" :key="n" :class="{ active: searchThreads === n }" @click="chooseThreads(n)">{{ n }}{{ n === 1 ? '（默认）' : '' }}</button>
        </div>
      </div>
    </div>

    <div v-if="showCacheTTL" class="settings-overlay" @click.self="showCacheTTL = false">
      <div class="settings-panel">
        <div class="settings-head">
          <h2>搜索缓存</h2>
          <button @click="showCacheTTL = false">✕</button>
        </div>
        <p class="threads-hint">从搜索结果进入详情页后返回时，缓存内保留搜索结果、避免重新搜索的时长</p>
        <div class="threads-options">
          <button v-for="n in [30, 240, 720, 1440]" :key="n" :class="{ active: searchCacheTTL === n }" @click="chooseCacheTTL(n)">{{ n === 30 ? '30 分钟（默认）' : n === 240 ? '4 小时' : n === 720 ? '12 小时' : '24 小时' }}</button>
        </div>
      </div>
    </div>

    <div v-if="showAbout" class="settings-overlay" @click.self="showAbout = false">
      <div class="settings-panel">
        <div class="settings-head">
          <h2>关于 UnBox</h2>
          <button @click="showAbout = false">✕</button>
        </div>
        <div class="about-body">
          <img class="about-logo" src="/appicon.png" alt="UnBox logo" />
          <p>UnBox 是一个跨平台（Windows / macOS / Linux）的 TVBox 兼容桌面播放器，支持 IPTV 直播与视频点播，一个安装包装好即用。</p>
          <p class="about-ver">当前版本：{{ currentVersion }}</p>
          <p class="about-ver">内部版本：{{ internalVersion }}</p>
          <p>技术栈：Go 1.27 + Wails v3 + Vue 3 + TypeScript。完全开源。</p>
        </div>
      </div>
    </div>

    <div v-if="showDisclaimer" class="settings-overlay" @click.self="showDisclaimer = false">
      <div class="settings-panel">
        <div class="settings-head">
          <h2>免责条款</h2>
          <button @click="showDisclaimer = false">✕</button>
        </div>
        <div class="disclaimer">
          <p>UnBox 是一个开源的「空壳」播放器，本身不提供、不存储、不制作任何影视内容，也不内置任何内容源。</p>
          <p>本软件仅为用户提供技术性的播放能力。用户自行导入的内容源（订阅、M3U、TVBox 源等）及其所指向的资源，均由第三方提供，与本软件无关。</p>
          <p>请遵守当地法律法规，仅将本软件用于访问您拥有合法权利或已获授权的内容。用户因使用本软件而产生的任何法律后果由用户自行承担，本软件及作者不承担任何责任。</p>
        </div>
      </div>
    </div>

    <div v-if="deleteConfirmation" class="settings-overlay" @click.self="closeDeleteConfirmation">
      <div class="settings-panel delete-confirmation" role="dialog" aria-modal="true" aria-labelledby="delete-confirmation-title">
        <div class="settings-head">
          <h2 id="delete-confirmation-title">确认删除</h2>
          <button type="button" aria-label="关闭" @click="closeDeleteConfirmation">✕</button>
        </div>
        <p>{{ deleteConfirmation.kind === 'history' ? '确定删除这条观看记录吗？' : '确定删除这条收藏吗？' }}</p>
        <div class="delete-confirmation-actions">
          <button type="button" @click="closeDeleteConfirmation">取消</button>
          <button type="button" class="danger" @click="confirmDeleteContextCard">删除</button>
        </div>
      </div>
    </div>

    <div v-if="showContentCardStyle" class="settings-overlay" @click.self="showContentCardStyle = false">
      <div class="settings-panel settings-choice-panel">
        <div class="settings-head">
          <h2>内容展示样式</h2>
          <button type="button" aria-label="关闭" @click="showContentCardStyle = false">✕</button>
        </div>
        <p class="settings-choice-hint">控制点播列表、搜索结果和收藏页的内容展示方式（首页海报行固定为卡片）</p>
        <div class="settings-choice-options">
          <button v-for="style in contentCardStyleOptions" :key="style.value" type="button" :class="{ active: contentCardStyle === style.value }" @click="applyContentCardStyle(style.value)">{{ style.label }}</button>
        </div>
      </div>
    </div>

    <div v-if="showTheme" class="settings-overlay" @click.self="showTheme = false">
      <div class="settings-panel settings-choice-panel">
        <div class="settings-head">
          <h2>主题</h2>
          <button type="button" aria-label="关闭" @click="showTheme = false">✕</button>
        </div>
        <div class="theme-grid">
          <button v-for="t in themeOptions" :key="t.id" type="button" :class="{ active: currentTheme === t.id }" :data-theme="t.id" @click="applyTheme(t.id)">{{ t.label }}</button>
        </div>
      </div>
    </div>

    <p v-if="vodDetailLoading" class="detail-loading" role="status" aria-live="polite">正在加载《{{ vodDetailLoading }}》…</p>

    <div v-if="vodSourceDead" class="settings-overlay" @click.self="vodSourceDeadBack">
      <div class="settings-panel vod-source-dead" role="dialog" aria-modal="true" aria-labelledby="vod-source-dead-title">
        <div class="settings-head">
          <h2 id="vod-source-dead-title">无法访问点播源</h2>
          <button type="button" aria-label="关闭" @click="vodSourceDeadBack">✕</button>
        </div>
        <p>站点「{{ vodSourceDead.info.siteName }}」暂时无法连接，无法打开《{{ vodSourceDead.info.vodTitle }}》。</p>
        <p class="vod-source-dead-cause">原因：{{ vodSourceDead.info.message }}</p>
        <p class="vod-source-dead-hint">{{ deadCertExemptHost ? '该站点的 HTTPS 证书已过期或不受信任，加入豁免后会跳过证书校验重试一次。' : '可以从其他站点找这部剧继续看，或者换个站点重新收藏。' }}</p>
        <div class="vod-source-dead-actions">
          <button type="button" @click="vodSourceDeadBack">{{ vodSourceDeadBackLabel(vodSourceDead.origin) }}</button>
          <button v-if="deadCertExemptHost" type="button" class="primary" @click="exemptCertHost(deadCertExemptHost)">一键豁免并重试</button>
          <button type="button" :class="{ primary: !deadCertExemptHost }" @click="vodSourceDeadSearch">全站搜索该剧</button>
        </div>
      </div>
    </div>

    <div v-if="errMsg" class="error-box">
      <span class="error-text">{{ errMsg }}</span>
      <button v-if="certExemptHost" type="button" class="error-exempt" @click="exemptCertHost(certExemptHost)">一键豁免 {{ certExemptHost }} 并重试</button>
      <button class="error-close" title="关闭" @click="errMsg = ''">✕</button>
    </div>
  </main>
</template>
