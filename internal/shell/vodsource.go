// Package shell 的点播源来源切换：本地读取与 Cloudflare 在线拉取并列。
//
// 本地源是现有的导入路径（文件/订阅地址）；Cloudflare 源是另外一个入口：
// 用户在设置页填一个 CF 地址（如 https://live.ciweihaha.win/vod.json，
// TVBox 格式点播配置），UnBox 按需从该地址拉取并解析（走已有的
// ImportVodSource/TVBox 解析），不用每次改本地文件。两种来源都能用、能切换，
// 切换时重新拉取一次；启动时只恢复快照、不自动联网（与 RestoreSubscription 一致）。
package shell

import (
	"errors"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// 点播源两种来源。
const (
	VodSourceLocal      = "local"
	VodSourceCloudflare = "cloudflare"
)

// 点播源在 store.kv 里的键。
const (
	vodSourceKindKey  = "vod.source.kind"
	vodCFURLKey       = "vod.cf.url"
	vodLocalRefKey    = "vod.local.ref"
	vodCFUpdatedAtKey = "vod.cf.updatedAt"
)

// VodSourceStatus 是点播源当前状态（供设置页回显）。
type VodSourceStatus struct {
	Kind          string // "local" | "cloudflare"
	LocalRef      string // 上次导入的本地点播源地址
	CloudflareURL string // 已填的 Cloudflare 点播源地址
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
