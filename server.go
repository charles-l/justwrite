package main

import (
	"bytes"
	"compress/gzip"

	// Markdown is complicated enough that I'm willing to pay the cost of using
	// an external lib. I would normally prefer a smaller one, but this one
	// does a lot of good things out of the box including typographic
	// transforms like smart-quotes.
	"gitlab.com/golang-commonmark/markdown"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"text/template"
)

type Post struct {
	Title   string
	Author  string
	Date    string
	Content string
}

type gzipWriter struct {
	io.Writer
	http.ResponseWriter
}

func (w gzipWriter) Write(bs []byte) (int, error) {
	return w.Writer.Write(bs)
}

func gzipMiddleware(next http.Handler) http.Handler {
	// TODO: consider storing gzipped content, and only return
	// the uncompressed version if the Accept-Encoding doesn't include gzip?
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Set("Content-Encoding", "gzip")

		gz, err := gzip.NewWriterLevel(w, gzip.BestCompression)
		if err != nil {
			panic(err)
		}

		defer gz.Close()

		next.ServeHTTP(gzipWriter{gz, w}, r)
	})
}

type adminHandler struct{}

func (h *adminHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/html")

	upath := path.Clean(r.URL.Path)
	if !strings.HasPrefix(upath, "/") {
		upath = "/" + upath
		r.URL.Path = upath
	}

	components := strings.Split(strings.TrimPrefix(upath, "/_admin"), "/")

	log.Println(components, len(components))

	if len(components) == 1 {
		tmpl := template.Must(template.ParseFiles("post-list-template.html"))

		files, err := ioutil.ReadDir("./raw")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		var Posts []string
		for _, f := range files {
			Posts = append(Posts, f.Name())
		}

		tmplData := struct{ Posts []string }{Posts}
		if err = tmpl.Execute(w, tmplData); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	} else if len(components) == 2 {
		if r.Method == "GET" {
			tmpl := template.Must(template.ParseFiles("post-edit-template.html"))

			postName := components[1]
			contents, err := ioutil.ReadFile(path.Join("raw", postName))
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}

			tmplData := struct {
				Name     string
				Contents string
			}{postName, string(contents)}
			if tmpl.Execute(w, tmplData); err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
		} else if r.Method == "POST" {
			if err := r.ParseForm(); err != nil {
				http.Error(w, "error: "+err.Error(), 400)
				return
			}
			postName := components[1]
			ioutil.WriteFile(path.Join("raw", postName), []byte(r.FormValue("contents")), 0770)

			buildPost(postName)

			http.Redirect(w, r, "/_admin/"+postName, http.StatusMovedPermanently)
		}
	}
}

func buildPost(name string) Post {
	tmpl := template.Must(template.ParseFiles("template.html"))
	md := markdown.New(markdown.XHTMLOutput(true))
	p := path.Join("raw", name)

	bs, err := ioutil.ReadFile(p)
	if err != nil {
		log.Fatal(err)
	}

	idx := bytes.Index(bs, []byte("---"))
	var metadataLines []string
	if idx == -1 {
		// TODO: warn the user that there's no metadata
		idx = 0
	} else {
		metadataLines = strings.Split(string(bs[0:idx]), "\n")
		idx += 3
		bs = bs[idx:len(bs)]
	}

	var post Post
	for _, l := range metadataLines {
		if l == "" {
			continue
		}

		kvpair := strings.Split(l, ":")
		if len(kvpair) != 2 {
			log.Fatal("malformated metadata for ", p, ", not a key-value pair ", kvpair)
		}
		key := strings.TrimSpace(kvpair[0])
		value := strings.TrimSpace(kvpair[1])
		if key == "title" {
			post.Title = value
		} else if key == "author" {
			post.Author = value
		}
	}

	post.Content = md.RenderToString(bs)

	f, err := os.Create(path.Join("build", strings.TrimSuffix(name, ".md")+".html"))
	defer f.Close()
	if err != nil {
		log.Fatal(err)
	}

	err = tmpl.Execute(f, post)
	if err != nil {
		log.Fatal(err)
	}

	log.Println(post)

	return post
}

func build() {
	files, err := ioutil.ReadDir("./raw")
	if err != nil {
		log.Fatal(err)
	}

	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".md") {
			continue
		}

		buildPost(f.Name())
	}
}

func main() {
	build()

	http.Handle("/_admin/", &adminHandler{})

	fs := http.FileServer(http.Dir("./build"))
	http.Handle("/", gzipMiddleware(fs))

	log.Println("Listening on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
