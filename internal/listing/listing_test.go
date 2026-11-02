package listing

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// apacheIndex mimics Apache's autoindex output (Options Indexes, no fancy).
const apacheIndex = `<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 3.2 Final//EN">
<html><head><title>Index of /</title></head><body>
<h1>Index of /</h1>
<ul><li><a href="/"> Parent Directory</a></li>
<li><a href="?C=N;O=D">Name</a></li>
<li><a href="GOD%20OF%20WAR%201/"> GOD OF WAR 1/</a></li>
<li><a href="Mega%20Man/"> Mega Man/</a></li>
<li><a href="b-game.pkg"> b-game.pkg</a></li>
<li><a href="A-Game.PKG"> A-Game.PKG</a></li>
<li><a href="notes.txt"> notes.txt</a></li>
<li><a href="http://other.host/x.pkg"> x.pkg</a></li>
<li><a href="b-game.pkg"> duplicate</a></li>
</ul></body></html>`

func TestParse(t *testing.T) {
	base, _ := url.Parse("http://nas:8080/")
	got := Parse(base, apacheIndex)

	want := []struct {
		name  string
		url   string
		isDir bool
	}{
		{"GOD OF WAR 1", "http://nas:8080/GOD%20OF%20WAR%201/", true},
		{"Mega Man", "http://nas:8080/Mega%20Man/", true},
		{"A-Game.PKG", "http://nas:8080/A-Game.PKG", false},
		{"b-game.pkg", "http://nas:8080/b-game.pkg", false},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		g := got[i]
		if g.Name != w.name || g.URL != w.url || g.IsDir != w.isDir || g.Size != UnknownSize {
			t.Errorf("entry %d = %+v, want %+v", i, g, w)
		}
	}
}

func TestParseSubdirectory(t *testing.T) {
	base, _ := url.Parse("http://nas:8080/GOD%20OF%20WAR%201/")
	body := `<a href="/">Parent Directory</a>
<a href="GOD%20OF%20WAR%20UP9000.pkg">GOD OF WAR UP9000.pkg</a>`
	got := Parse(base, body)
	if len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
	if got[0].Name != "GOD OF WAR UP9000.pkg" {
		t.Errorf("name = %q", got[0].Name)
	}
	if got[0].URL != "http://nas:8080/GOD%20OF%20WAR%201/GOD%20OF%20WAR%20UP9000.pkg" {
		t.Errorf("url = %q", got[0].URL)
	}
}

func TestListAndSizes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			fmt.Fprint(w, apacheIndex)
		case "/b-game.pkg":
			w.Header().Set("Content-Length", "4096")
		case "/A-Game.PKG":
			w.Header().Set("Content-Length", "123")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New(srv.Client())
	entries, err := c.List(context.Background(), srv.URL) // no trailing slash on purpose
	if err != nil {
		t.Fatal(err)
	}
	c.Sizes(context.Background(), entries)

	sizes := map[string]int64{}
	for _, e := range entries {
		sizes[e.Name] = e.Size
	}
	if sizes["b-game.pkg"] != 4096 || sizes["A-Game.PKG"] != 123 {
		t.Errorf("sizes = %v", sizes)
	}
	if sizes["Mega Man"] != UnknownSize {
		t.Errorf("directories must keep UnknownSize, got %d", sizes["Mega Man"])
	}
}

func TestListRejectsBadScheme(t *testing.T) {
	if _, err := New(http.DefaultClient).List(context.Background(), `\\192.168.0.121\paulo`); err == nil {
		t.Fatal("expected an error for a non-HTTP URL")
	}
}

func TestParent(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"http://nas:8080/a/b/", "http://nas:8080/a/", true},
		{"http://nas:8080/a", "http://nas:8080/", true},
		{"http://nas:8080/", "http://nas:8080/", false},
		{"http://nas:8080/GOD%20OF%20WAR/", "http://nas:8080/", true},
	}
	for _, c := range cases {
		got, ok := Parent(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("Parent(%q) = %q, %v; want %q, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}
