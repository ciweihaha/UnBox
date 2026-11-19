package store

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestVodSearchHistoryCRUD(t *testing.T) {
	s := openTemp(t)
	defer s.Close()
	if err := s.RecordVodSearch("  斗罗  "); err != nil {
		t.Fatalf("RecordVodSearch: %v", err)
	}
	if err := s.RecordVodSearch("庆余年"); err != nil {
		t.Fatalf("RecordVodSearch: %v", err)
	}
	if err := s.RecordVodSearch("斗罗"); err != nil {
		t.Fatalf("RecordVodSearch duplicate: %v", err)
	}
	history, err := s.ListVodSearchHistory(10)
	if err != nil || len(history) != 2 || history[0] != "斗罗" {
		t.Fatalf("ListVodSearchHistory = %#v, %v", history, err)
	}
	if err := s.DeleteVodSearch("斗罗"); err != nil {
		t.Fatalf("DeleteVodSearch: %v", err)
	}
	history, err = s.ListVodSearchHistory(10)
	if err != nil || len(history) != 1 || history[0] != "庆余年" {
		t.Fatalf("history after delete = %#v, %v", history, err)
	}
}

func TestVodFavoriteCRUD(t *testing.T) {
	s := openTemp(t)
	defer s.Close()
	if err := s.AddVodFavorite("site-a", "vod-1", "影片一", "logo", "电影"); err != nil {
		t.Fatalf("AddVodFavorite: %v", err)
	}
	if err := s.AddVodFavorite("site-a", "vod-1", "影片一（更新）", "logo2", "电视剧"); err != nil {
		t.Fatalf("AddVodFavorite duplicate: %v", err)
	}
	ok, err := s.IsVodFavorite("site-a", "vod-1")
	if err != nil || !ok {
		t.Fatalf("IsVodFavorite = %v, %v", ok, err)
	}
	favs, err := s.ListVodFavorites()
	if err != nil || len(favs) != 1 || favs[0].Title != "影片一（更新）" {
		t.Fatalf("ListVodFavorites = %#v, %v", favs, err)
	}
	if err := s.RemoveVodFavorite("site-a", "vod-1"); err != nil {
		t.Fatalf("RemoveVodFavorite: %v", err)
	}
	if ok, _ := s.IsVodFavorite("site-a", "vod-1"); ok {
		t.Fatal("favorite still exists after removal")
	}
}

func TestDeleteVodHistory(t *testing.T) {
	s := openTemp(t)
	defer s.Close()
	if err := s.UpsertVodHistory(VodHistory{Site: "site-a", VodID: "vod-1", VodTitle: "影片一"}); err != nil {
		t.Fatalf("UpsertVodHistory: %v", err)
	}
	if err := s.DeleteVodHistory("site-a", "vod-1"); err != nil {
		t.Fatalf("DeleteVodHistory: %v", err)
	}
	history, err := s.ListVodHistory(10)
	if err != nil || len(history) != 0 {
		t.Fatalf("history after delete = %#v, %v", history, err)
	}
}

func TestFavoriteCRUD(t *testing.T) {
	s := openTemp(t)
	defer s.Close()
	if err := s.AddFavorite("央视/CCTV-1", "CCTV-1", "央视", "http://x/1"); err != nil {
		t.Fatalf("AddFavorite: %v", err)
	}
	ok, err := s.IsFavorite("央视/CCTV-1")
	if err != nil || !ok {
		t.Fatalf("IsFavorite = %v, %v", ok, err)
	}
	favs, err := s.ListFavorites()
	if err != nil || len(favs) != 1 || favs[0].Name != "CCTV-1" {
		t.Fatalf("ListFavorites = %+v, %v", favs, err)
	}
	if err := s.RemoveFavorite("央视/CCTV-1"); err != nil {
		t.Fatalf("RemoveFavorite: %v", err)
	}
	if favs, _ = s.ListFavorites(); len(favs) != 0 {
		t.Fatalf("删除后仍有 %d 条", len(favs))
	}
}

func TestRecentKeepsLatestFirst(t *testing.T) {
	s := openTemp(t)
	defer s.Close()
	s.AddRecent("a", "a", "组", "http://a")
	s.AddRecent("b", "b", "组", "http://b")
	rs, err := s.ListRecent(1)
	if err != nil || len(rs) != 1 || rs[0].Name != "b" {
		t.Fatalf("ListRecent(1) = %+v, %v", rs, err)
	}
}

func TestGroupCRUD(t *testing.T) {
	s := openTemp(t)
	defer s.Close()
	s.AddGroup("我的分组")
	s.AddGroup("我的分组") // 幂等
	gs, err := s.ListGroups()
	if err != nil || len(gs) != 1 {
		t.Fatalf("ListGroups = %+v, %v", gs, err)
	}
}
func TestMigrateSourcesSchemaPreservesHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	// 建旧表（ref 单主键）并写入两条历史。
	if _, err := db.Exec(`CREATE TABLE sources (ref TEXT PRIMARY KEY, kind TEXT NOT NULL, at INTEGER NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO sources(ref, kind, at) VALUES('http://x/vod.json','vod',111),('http://x/live.m3u','live',222)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	got, err := s.ListSources()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("迁移后 sources = %+v，期望保留 2 条历史", got)
	}
	// 迁移后复合主键生效：同 ref 不同 kind 可共存。
	if err := s.AddSource("live", "http://x/vod.json"); err != nil {
		t.Fatalf("AddSource 同 ref 不同 kind 应成功: %v", err)
	}
}
func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return s
}
