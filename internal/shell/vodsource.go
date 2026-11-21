// Package shell 的点播源来源切换：本地读取与 Cloudflare 在线拉取并列。
//
// 本地源是现有的导入路径（文件/订阅地址）；Cloudflare 源是另外一个入口：
// 用户在设置页填一个 CF 地址（如 https://live.ciweihaha.win/vod.json，
// TVBox 格式点播配置），UnBox 按需从该地址拉取并解析（走已有的
// ImportVodSource/TVBox 解析），不用每次改本地文件。两种来源都能用、能切换，
// 切换时重新拉取一次；启动时只恢复快照、不自动联网（与 RestoreSubscription 一致）。
//
// v0.9.16 起 CF 源改成下拉框：选项从点播源目录地址拉取
// （默认 https://live.ciweihaha.win/vod-sources.json，见 DefaultCfVodDirectoryURL），
// 目录里有新增的源就自动出现；目录拉不到时用内置默认源顶着，也能手动填地址。
package shell

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/unbox/unbox/internal/config"
)

// 点播源两种来源。
const (
	VodSourceLocal      = "local"
	VodSourceCloudflare = "cloudflare"
)

// DefaultCfVodDirectoryURL 是点播源目录的默认地址：用户在 Cloudflare 上
// 维护的源清单（名字 + 地址），下拉框的选项从这里自动拉取。
// 目录文件还没挂上时拉取会 404，前端用内置默认源顶着、不报错。
const DefaultCfVodDirectoryURL = "https://live.ciweihaha.win/vod-sources.json"

// 点播源在 store.kv 里的键。
const (
	vodSourceKindKey     = "vod.source.kind"
	vodCFURLKey          = "vod.cf.url"
	vodLocalRefKey       = "vod.local.ref"
	vodCFUpdatedAtKey    = "vod.cf.updatedAt"
	vodCFDirectoryURLKey = "vod.cf.dirurl"
)

// CfVodDirectorySource 是点播源目录里的一项：名字 + 地址。
// Sites 是该源能提供的支持格式点播站点数（拉取目录时顺手验证）：
// >=0 表示已验证；-1 表示没验证出来（拉不到/解析失败），未知、保留不隐藏；
// 0 表示已证实没有可用站点，不进下拉框。
type CfVodDirectorySource struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Sites int    `json:"sites"`
}

// VodSourceStatus 是点播源当前状态（供设置页回显）。
type VodSourceStatus struct {
	Kind          string // "local" | "cloudflare"
	LocalRef      string // 上次导入的本地点播源地址
	CloudflareURL string // 已填的 Cloudflare 点播源地址
	DirectoryURL  string // 点播源目录地址（下拉框的选项来源）；空表示用默认地址
	UpdatedAt     int64  // CF 源上次拉取的 Unix 秒；0 表示未拉取过
	Sites         int    // 当前已加载的点播站点数
}

// GetVodSourceStatus 返回点播源当前状态；未设置时默认本地源。
func (s *ShellService) GetVodSourceStatus() VodSourceStatus {
	st := VodSourceStatus{Kind: VodSourceLocal}
	if s.store != nil {
		if v, ok, err := s.store.GetKV(vodSourceKindKey); err == nil && ok && v == VodSourceCloudflare {
			st.Kind = VodSourceCloudflare
		}
		if v, ok, err := s.store.GetKV(vodLocalRefKey); err == nil && ok {
			st.LocalRef = v
		}
		if v, ok, err := s.store.GetKV(vodCFURLKey); err == nil && ok {
			st.CloudflareURL = v
		}
		if v, ok, err := s.store.GetKV(vodCFDirectoryURLKey); err == nil && ok {
			st.DirectoryURL = v
		}
		if v, ok, err := s.store.GetKV(vodCFUpdatedAtKey); err == nil && ok {
			if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
				st.UpdatedAt = n
			}
		}
	}
	s.mu.RLock()
	st.Sites = len(s.vods)
	s.mu.RUnlock()
	return st
}

// GetCfVodDirectoryURL 返回点播源目录地址；没改过时用默认地址。
func (s *ShellService) GetCfVodDirectoryURL() string {
	if s.store != nil {
		if v, ok, err := s.store.GetKV(vodCFDirectoryURLKey); err == nil && ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return DefaultCfVodDirectoryURL
}

// SetCfVodDirectoryURL 改点播源目录地址：必须是 http(s)，只保存不拉取
// （拉取在打开设置 / 点刷新目录时做）。
func (s *ShellService) SetCfVodDirectoryURL(rawURL string) error {
	rawURL = strings.TrimSpace(rawURL)
	if !isHTTPURL(rawURL) {
		return errors.New("点播源目录地址必须是 http(s) 地址")
	}
	s.setVodSourceKV(vodCFDirectoryURLKey, rawURL)
	return nil
}

// cfVodDirectoryValidateTimeout 是目录里单条源的验证超时：
// 只拉一份小配置、数支持格式的站点数，5 秒够了；超时按未知处理（保留）。
const cfVodDirectoryValidateTimeout = 5 * time.Second

// FetchCfVodDirectory 从目录地址拉取点播源清单：接受 {"sources": [...]}
// 或裸数组；只要带名字和 http(s) 地址的项，去重（按地址），其余丢弃。
// 每条源顺手验证一次（并行）：能拉到、解析出 0 个支持格式站点的源
// （如地址指向的不是 TVBox 配置、而是列表接口）不进下拉框；
// 拉不到 / 解析失败的按未知保留（不断网时不让下拉框变空）。
// 目录拉不到（404 / 没网 / 格式不对）时返回错误，前端用内置默认源顶着。
func (s *ShellService) FetchCfVodDirectory() ([]CfVodDirectorySource, error) {
	dirURL := s.GetCfVodDirectoryURL()
	client := s.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Get(dirURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("点播源目录拉取失败：HTTP " + strconv.Itoa(resp.StatusCode))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	list, err := parseCfVodDirectory(body)
	if err != nil {
		return nil, err
	}
	// 并行验证每条源的支持站点数，0 站点的剔掉。
	var wg sync.WaitGroup
	for i := range list {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			list[i].Sites = s.validateCfVodDirectorySource(context.Background(), list[i].URL)
		}(i)
	}
	wg.Wait()
	out := make([]CfVodDirectorySource, 0, len(list))
	for _, src := range list {
		if src.Sites == 0 {
			continue
		}
		out = append(out, src)
	}
	return out, nil
}

// validateCfVodDirectorySource 拉取并解析一条目录源，返回它能提供的
// 支持格式点播站点数；拉不到 / 解析失败返回 -1（未知，保留不隐藏）。
func (s *ShellService) validateCfVodDirectorySource(ctx context.Context, rawURL string) int {
	raw, err := config.NewFetcherWithTimeout(cfVodDirectoryValidateTimeout).Fetch(ctx, rawURL)
	if err != nil {
		return -1
	}
	cfgs, err := resolveConfigs(ctx, rawURL, raw)
	if err != nil {
		return 0
	}
	return config.CountSupportedSites(cfgs)
}

// parseCfVodDirectory 解析目录 JSON（纯函数，方便单测）。
func parseCfVodDirectory(body []byte) ([]CfVodDirectorySource, error) {
	var wrapped struct {
		Sources []CfVodDirectorySource `json:"sources"`
	}
	var list []CfVodDirectorySource
	if err := json.Unmarshal(body, &wrapped); err == nil && wrapped.Sources != nil {
		list = wrapped.Sources
	} else if err := json.Unmarshal(body, &list); err != nil {
		return nil, errors.New("点播源目录格式不对")
	}
	seen := make(map[string]struct{})
	out := make([]CfVodDirectorySource, 0, len(list))
	for _, src := range list {
		src.Name = strings.TrimSpace(src.Name)
		src.URL = strings.TrimSpace(src.URL)
		if src.Name == "" || !isHTTPURL(src.URL) {
			continue
		}
		if _, dup := seen[src.URL]; dup {
			continue
		}
		seen[src.URL] = struct{}{}
		out = append(out, src)
	}
	return out, nil
}

// SetCloudflareVodURL 填入 Cloudflare 点播源地址并立即拉取一次：
// 地址必须是 http(s)，拉取成功后才记为当前来源（失败不改动原来源）；
// 随后点「刷新」可随时从该地址重拉，不用改本地文件。
func (s *ShellService) SetCloudflareVodURL(rawURL string) (ImportResult, error) {
	rawURL = strings.TrimSpace(rawURL)
	if !isHTTPURL(rawURL) {
		return ImportResult{}, errors.New("Cloudflare 点播源地址必须是 http(s) 地址")
	}
	res, err := s.ImportVodSource(rawURL)
	if err != nil {
		return res, err
	}
	s.setVodSourceKind(VodSourceCloudflare)
	s.setVodSourceKV(vodCFURLKey, rawURL)
	s.setVodSourceKV(vodCFUpdatedAtKey, strconv.FormatInt(time.Now().Unix(), 10))
	return res, nil
}

// RefreshCloudflareVodSource 从已填的 Cloudflare 地址重新拉取一次点播源。
func (s *ShellService) RefreshCloudflareVodSource() (ImportResult, error) {
	st := s.GetVodSourceStatus()
	if strings.TrimSpace(st.CloudflareURL) == "" {
		return ImportResult{}, errors.New("还没填 Cloudflare 点播源地址")
	}
	res, err := s.ImportVodSource(st.CloudflareURL)
	if err != nil {
		return res, err
	}
	s.setVodSourceKind(VodSourceCloudflare)
	s.setVodSourceKV(vodCFUpdatedAtKey, strconv.FormatInt(time.Now().Unix(), 10))
	return res, nil
}

// UseLocalVodSource 切回本地点播源：重新读取之前导入过的本地地址。
func (s *ShellService) UseLocalVodSource() (ImportResult, error) {
	st := s.GetVodSourceStatus()
	if strings.TrimSpace(st.LocalRef) == "" {
		return ImportResult{}, errors.New("还没有导入过的本地点播源")
	}
	res, err := s.ImportVodSource(st.LocalRef)
	if err != nil {
		return res, err
	}
	s.setVodSourceKind(VodSourceLocal)
	return res, nil
}

// recordVodSourceRef 记下通用导入的来源：本地路径记为本地源；
// http(s) 地址不动来源（可能是任意订阅，不一定是 Cloudflare 源）。
func (s *ShellService) recordVodSourceRef(ref string) {
	if isHTTPURL(ref) {
		return
	}
	s.setVodSourceKind(VodSourceLocal)
	s.setVodSourceKV(vodLocalRefKey, ref)
}

func (s *ShellService) setVodSourceKind(kind string) {
	s.setVodSourceKV(vodSourceKindKey, kind)
}

func (s *ShellService) setVodSourceKV(key, value string) {
	if s.store == nil {
		return
	}
	if err := s.store.SetKV(key, value); err != nil {
		log.Printf("保存点播源设置失败 %s: %v", key, err)
	}
}

// isHTTPURL 判断一个源地址是不是 http(s) 在线地址。
func isHTTPURL(ref string) bool {
	u, err := url.Parse(strings.TrimSpace(ref))
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
