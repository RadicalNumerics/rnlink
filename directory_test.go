package golink

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/xsrftoken"
)

func TestLegacyDirectoryMigration(t *testing.T) {
	file := filepath.Join(t.TempDir(), "old.db")
	old, err := sql.Open("sqlite", file)
	if err != nil {
		t.Fatal(err)
	}
	_, err = old.Exec(`CREATE TABLE Links (ID TEXT PRIMARY KEY, Short TEXT NOT NULL, Long TEXT NOT NULL, Created INTEGER NOT NULL, LastEdit INTEGER NOT NULL, Owner TEXT NOT NULL);
 INSERT INTO Links VALUES ('gpus','gpus','https://example.com/gpus',1700000000,1700000001,'owner@example.com')`)
	if err != nil {
		t.Fatal(err)
	}
	old.Close()
	for attempt := 0; attempt < 2; attempt++ {
		db, err := NewSQLiteDB(file)
		if err != nil {
			t.Fatal(err)
		}
		link, err := db.Load("gpus")
		if err != nil {
			t.Fatal(err)
		}
		if link.Long != "https://example.com/gpus" || link.Owner != "owner@example.com" || link.Created.Unix() != 1700000000 || link.CreatedBy != "" {
			t.Fatalf("migration changed legacy metadata: %+v", link)
		}
		if attempt == 0 {
			if link.Description != "" {
				t.Fatal("expected empty legacy description")
			}
			link.Description = "GPU capacity"
			if err := db.Save(link); err != nil {
				t.Fatal(err)
			}
		} else if link.Description != "GPU capacity" {
			t.Fatal("migration lost description on reopen")
		}
		db.db.Close()
	}
}

func TestDirectoryMetadataSaveAndSnapshot(t *testing.T) {
	s := newTestServer(t)
	submit := func(fields url.Values, tokenName string) *httptest.ResponseRecorder {
		t.Helper()
		fields.Set("xsrf", xsrftoken.Generate(s.xsrfKey, "foo@example.com", tokenName))
		r := httptest.NewRequest("POST", "/", strings.NewReader(fields.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		s.serveSave(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("save: %d %s", w.Code, w.Body.String())
		}
		return w
	}
	fields := url.Values{"short": {"gpus"}, "long": {"https://example.com"}, "description": {" GPU capacity "}, "createdBy": {"forged@example.com"}}
	submit(fields, newShortName)
	link, err := s.db.Load("gpus")
	if err != nil {
		t.Fatal(err)
	}
	created := link.Created
	if link.Description != "GPU capacity" || link.CreatedBy != "foo@example.com" {
		t.Fatalf("metadata: %+v", link)
	}
	fields.Del("description")
	submit(fields, "gpus")
	link, _ = s.db.Load("gpus")
	if link.Description != "GPU capacity" {
		t.Fatal("older client erased description")
	}
	fields.Set("description", "Updated description")
	fields.Set("owner", "bar@example.com")
	submit(fields, "gpus")
	link, _ = s.db.Load("gpus")
	if link.CreatedBy != "foo@example.com" || !link.Created.Equal(created) || link.Owner != "bar@example.com" {
		t.Fatalf("ownership transfer changed creator/date: %+v", link)
	}
	snapshot, err := json.Marshal(link)
	if err != nil {
		t.Fatal(err)
	}
	restored := newTestServer(t)
	if err := restored.restoreSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	got, err := restored.db.Load("gpus")
	if err != nil {
		t.Fatal(err)
	}
	if got.Description != "Updated description" || got.CreatedBy != link.CreatedBy {
		t.Fatalf("snapshot lost metadata: %+v", got)
	}
}

func TestHomeDirectory(t *testing.T) {
	s := newTestServer(t)
	for i := 0; i < 14; i++ {
		short := fmt.Sprintf("link%02d", i)
		if err := s.db.Save(&Link{Short: short, Long: "https://example.com", Description: "<script>alert(1)</script>", Created: time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC), CreatedBy: "creator@example.com"}); err != nil {
			t.Fatal(err)
		}
		s.stats.clicks[short] = i
	}
	s.stats.clicks["deleted"] = 1000
	w := httptest.NewRecorder()
	s.serveHome(w, httptest.NewRequest("GET", "/", nil), "")
	body := w.Body.String()
	if w.Code != http.StatusOK {
		t.Fatal(w.Code)
	}
	if strings.Count(body, `class="popular-row"`) != 10 {
		t.Fatal("expected exactly 10 popular rows")
	}
	popular := strings.Split(strings.Split(body, `<tbody id="popular-links"`)[1], `</tbody>`)[0]
	if strings.Contains(popular, `href="/link00/"`) || strings.Contains(popular, "deleted") {
		t.Fatal("popular grid includes low-ranked or deleted links")
	}
	if strings.Index(popular, `href="/link13/"`) > strings.Index(popular, `href="/link12/"`) {
		t.Fatal("popular order incorrect")
	}
	for i := 0; i < 14; i++ {
		if !strings.Contains(body, fmt.Sprintf(`class="directory-path" href="/link%02d/"`, i)) {
			t.Fatalf("link %d missing from all links", i)
		}
	}
	for _, value := range []string{"creator@example.com", "Nov 25, 2026", "&lt;script&gt;", `name="description"`} {
		if !strings.Contains(body, value) {
			t.Errorf("missing %s", value)
		}
	}
	if strings.Contains(body, "<script>alert") {
		t.Fatal("description HTML was not escaped")
	}
	s.readonly = true
	w = httptest.NewRecorder()
	s.serveHome(w, httptest.NewRequest("GET", "/", nil), "")
	if strings.Contains(w.Body.String(), `method="POST"`) {
		t.Fatal("read-only page contains create form")
	}
}

func TestBackfillCreator(t *testing.T) {
	s := newTestServer(t)
	for _, link := range []*Link{{Short: "legacy", Owner: "current@example.com"}, {Short: "recorded", CreatedBy: "original@example.com"}} {
		if err := s.db.Save(link); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.db.BackfillCreator(""); err == nil {
		t.Fatal("empty creator accepted")
	}
	if err := s.db.BackfillCreator("known@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := s.db.BackfillCreator("different@example.com"); err != nil {
		t.Fatal(err)
	}
	legacy, _ := s.db.Load("legacy")
	recorded, _ := s.db.Load("recorded")
	if legacy.CreatedBy != "known@example.com" || recorded.CreatedBy != "original@example.com" {
		t.Fatal("backfill overwrote a recorded creator")
	}
}

func TestDirectoryTrailingSlash(t *testing.T) {
	s := newTestServer(t)
	if err := s.db.Save(&Link{Short: "gpus", Long: "https://example.com/gpus/"}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/gpus", "/gpus/"} {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != http.StatusFound || w.Header().Get("Location") != "https://example.com/gpus/" {
			t.Fatalf("%s: %d %s", path, w.Code, w.Header().Get("Location"))
		}
	}
	for _, path := range []string{"/", "/.all", "/.detail/gpus"} {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if path != "/.detail/gpus" && !strings.Contains(w.Body.String(), `href="/gpus/"`) {
			t.Fatalf("%s lacks trailing slash link", path)
		}
	}
}

func TestDescriptionRequired(t *testing.T) {
	for _, existing := range []bool{false, true} {
		for _, value := range []string{"", "  \n "} {
			t.Run(fmt.Sprintf("existing=%v/value=%q", existing, value), func(t *testing.T) {
				s := newTestServer(t)
				tokenName := newShortName
				if existing {
					s.db.Save(&Link{Short: "required", Long: "https://example.com/original", Description: "Original description"})
					tokenName = "required"
				}
				fields := url.Values{"short": {"required"}, "long": {"https://example.com/changed"}, "description": {value}, "xsrf": {xsrftoken.Generate(s.xsrfKey, "foo@example.com", tokenName)}}
				r := httptest.NewRequest("POST", "/", strings.NewReader(fields.Encode()))
				r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				w := httptest.NewRecorder()
				s.serveSave(w, r)
				if w.Code != http.StatusBadRequest {
					t.Fatalf("got %d, want 400", w.Code)
				}
				if existing {
					link, _ := s.db.Load("required")
					if link.Description != "Original description" || link.Long != "https://example.com/original" {
						t.Fatal("invalid request mutated link")
					}
				} else if link, _ := s.db.Load("required"); link != nil {
					t.Fatal("invalid request created link")
				}
			})
		}
	}
}
