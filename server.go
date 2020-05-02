package main

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"reflect"
	"strings"
	"text/template"

	// Markdown is complicated enough that I'm willing to pay the cost of using
	// an external lib. I would normally prefer a smaller one, but this one
	// does a lot of good things out of the box including typographic
	// transforms like smart-quotes.
	"gitlab.com/golang-commonmark/markdown"
)

type Post struct {
	Title   string
	Author  string
	Date    string
	Content []byte
}

const metadataDelimiter = "+++"

func loadPost(r io.Reader) (Post, error) {
	bs, err := ioutil.ReadAll(r)
	if err != nil {
		return Post{}, err
	}

	// Remove CRLF
	bs = bytes.ReplaceAll(bs, []byte("\r\n"), []byte("\n"))

	s := strings.TrimSpace(string(bs[:len(metadataDelimiter)+1]))
	if s == metadataDelimiter {
		bs = bs[len(metadataDelimiter):]
	} else {
		return Post{},
			fmt.Errorf("Post doesn't begin with a metadata block (\"%s\"), got: '%s'", metadataDelimiter, s)
	}

	idx := bytes.Index(bs, []byte(metadataDelimiter))
	if idx == -1 {
		return Post{},
			fmt.Errorf("Post doesn't have an end to the metadata block (\"%s\")", metadataDelimiter)
	}

	var post Post
	metadata := make(map[string]string)
	for _, l := range strings.Split(string(bs[:idx]), "\n") {
		if l != "" {
			kvpair := strings.Split(l, "=")
			if len(kvpair) != 2 {
				return Post{},
					fmt.Errorf("Malformated metadata, not a key-value pair: %s", kvpair)
			}
			metadata[strings.TrimSpace(kvpair[0])] = strings.TrimSpace(kvpair[1])
		}
	}

	{ // struct with metadata
		ty := reflect.ValueOf(post).Type()

		for i := 0; i < ty.NumField(); i++ {
			if n := ty.Field(i).Name; n != "Content" {
				if val, ok := metadata[n]; ok {
					reflect.ValueOf(&post).Elem().FieldByName(n).SetString(val)
				}
			}
		}
	}

	post.Content = bs[idx+len(metadataDelimiter):]

	return post, nil
}

func (p Post) savePost(w io.Writer) error {
	v := reflect.ValueOf(p)
	ty := v.Type()

	metadataLines := []string{metadataDelimiter}

	for i := 0; i < ty.NumField(); i++ {
		if ty.Field(i).Name != "Content" {
			// Assumes string for now -- might have to use interface{} in
			// the future...

			metadataLines = append(metadataLines,
				fmt.Sprintf("%s = %s",
					ty.Field(i).Name, v.Field(i).Interface().(string)))
		}
	}
	metadataLines = append(metadataLines, metadataDelimiter)

	w.Write([]byte(strings.Join(metadataLines, "\n")))
	w.Write(p.Content)

	return nil
}

func (p Post) renderPost(w io.Writer) {
	tmpl := template.Must(template.ParseFiles("template.html"))
	md := markdown.New(markdown.XHTMLOutput(true))

	err := tmpl.Execute(w, struct {
		Title   string
		Author  string
		Date    string
		Content string
	}{p.Title, p.Author, p.Date, md.RenderToString(p.Content)})
	if err != nil {
		log.Fatal(err)
	}
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
		if r.Method == "GET" {
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
		} else if r.Method == "POST" {
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

			p, err := loadPost(bytes.NewReader([]byte(r.FormValue("contents"))))
			if err != nil {
				http.Error(w, "error: "+err.Error(), 500)
				return
			}

			source, err := os.Create(path.Join("raw", postName))
			if err != nil {
				http.Error(w, "error: "+err.Error(), 500)
				return
			}
			defer source.Close()
			p.savePost(source)

			out, err := os.Create(path.Join("build", strings.TrimSuffix(postName, ".md")+".html"))
			defer out.Close()

			if err != nil {
				http.Error(w, "error: "+err.Error(), 500)
				return
			}

			p.renderPost(out)

			http.Redirect(w, r, "/_admin/"+postName, http.StatusMovedPermanently)
		}
	}
}

func build() {
	files, err := ioutil.ReadDir("raw")
	if err != nil {
		log.Fatal(err)
	}

	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".md") {
			continue
		}

		in, err := os.Open(path.Join("raw", f.Name()))
		defer in.Close()

		if err != nil {
			log.Fatal(err)
		}

		p, err := loadPost(in)
		if err != nil {
			log.Fatal(f.Name()+": ", err)
		}

		out, err := os.Create(path.Join("build", strings.TrimSuffix(f.Name(), ".md")+".html"))
		defer out.Close()

		if err != nil {
			log.Fatal(err)
		}

		p.renderPost(out)
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
