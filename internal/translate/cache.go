package translate

import "sync"

// maxCacheEntries 是翻译缓存上限；evictBatch 是满时一次淘汰的最旧条数。
const maxCacheEntries = 500
const evictBatch = 100

// translationCache 是字幕翻译的 FIFO 缓存：同一条只翻一次，满时淘汰最旧一批，
// 而不是整库清空——长片看到一半时整库清空，接下来几百条字幕要全部重新调接口。
type translationCache struct {
	mu    sync.Mutex
	cache map[string]string
	order []string
}

func newTranslationCache() *translationCache {
	return &translationCache{cache: make(map[string]string)}
}

func (c *translationCache) get(text string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	got, ok := c.cache[text]
	return got, ok
}

func (c *translationCache) put(text, translated string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.order) >= maxCacheEntries {
		for _, old := range c.order[:evictBatch] {
			delete(c.cache, old)
		}
		c.order = append([]string(nil), c.order[evictBatch:]...)
	}
	if _, ok := c.cache[text]; !ok {
		c.order = append(c.order, text)
	}
	c.cache[text] = translated
}
