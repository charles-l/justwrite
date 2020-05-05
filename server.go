package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"reflect"
	"regexp"
	"strings"
	"text/template"
	"time"

	// Markdown is complicated enough that I'm willing to pay the cost of using
	// an external lib. I would normally prefer a smaller one, but this one
	// does a lot of good things out of the box including typographic
	// transforms like smart-quotes.
	"gitlab.com/golang-commonmark/markdown"

	// http router
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
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
	bs = bytes.Replace(bs, []byte("\r\n"), []byte("\n"), -1)

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

	// TODO find a better name, because this metadata is related to the underlying files...

	return post, nil
}

// I would use a sum type here, but go doesn't have them...
// Now I have to worry about not attempting to use PublishedAt
// in an invalid way -_-
type PostMetadata struct {
	LastUpdated time.Time
	PublishedAt *time.Time
}

func (m PostMetadata) isPublishedUpToDate() bool {
	return m.PublishedAt != nil && m.LastUpdated.Before(*m.PublishedAt)
}

func postPublishedName(postName string) string {
	return strings.TrimSuffix(postName, ".md") + ".html"
}

func loadPostMetadata(postName string) (PostMetadata, error) {
	rawStat, err := os.Stat(path.Join("raw", postName))
	if err != nil {
		return PostMetadata{}, err
	}

	buildStat, err := os.Stat(path.Join("build", postPublishedName(postName)))
	if os.IsNotExist(err) {
		return PostMetadata{rawStat.ModTime(), nil}, nil
	} else if err != nil {
		return PostMetadata{}, err
	} else { // buildStat exists
		publishedDate := buildStat.ModTime()
		return PostMetadata{rawStat.ModTime(), &publishedDate}, nil
	}
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
	tmpl := template.Must(template.ParseFiles("post-template.html"))
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

func build() {
	files, err := ioutil.ReadDir("raw")
	if err != nil {
		log.Fatal(err)
	}

	var publishedPosts []struct {
		Path string
		Post Post
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

		// TODO don't pass the contents along too -- they're not needed at this point
		publishedPosts = append(publishedPosts, struct {
			Path string
			Post Post
		}{postPublishedName(f.Name()), p})

		out, err := os.Create(path.Join("build", postPublishedName(f.Name())))
		defer out.Close()

		if err != nil {
			log.Fatal(err)
		}

		p.renderPost(out)
	}

	out, err := os.Create(path.Join("build", "index.html"))
	defer out.Close()

	if err != nil {
		log.Fatal(err)
	}

	tmpl := template.Must(template.ParseFiles("index-template.html"))
	tmpl.Execute(out, publishedPosts)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil || !os.IsNotExist(err)
}

func main() {
	build()

	router := chi.NewRouter()
	router.Use(middleware.NewCompressor(5, "text/html", "text/css").Handler)
	router.Use(middleware.Logger)

	router.Route("/_admin", func(adminRoute chi.Router) {
		adminRoute.Get("/", func(w http.ResponseWriter, r *http.Request) {
			tmpl := template.Must(template.ParseFiles("post-list-template.html"))

			files, err := ioutil.ReadDir("raw")
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
		})

		adminRoute.Post("/", func(w http.ResponseWriter, r *http.Request) {
			if err := r.ParseForm(); err != nil {
				http.Error(w, "error: "+err.Error(), 400)
				return
			}
			postName := r.FormValue("new-post-name")
			postFilename := regexp.MustCompile(`[^\w\d-]`).ReplaceAllString(postName, "-") + ".md"

			author := "charles"
			y, m, d := time.Now().Date()
			p := Post{
				string(postName),
				author,
				fmt.Sprintf("%d-%d-%d", y, m, d),
				[]byte{},
			}

			rawPath := path.Join("raw", postFilename)
			if fileExists(rawPath) {
				http.Error(w, "error: post already exists: "+postFilename, 400)
				return
			}

			source, err := os.Create(rawPath)
			if err != nil {
				http.Error(w, "error: "+err.Error(), 500)
				return
			}
			defer source.Close()

			if err := p.savePost(source); err != nil {
				http.Error(w, "error: "+err.Error(), 500)
				return
			}

			http.Redirect(w, r, "/_admin/"+postFilename, http.StatusMovedPermanently)
		})

		adminRoute.Get("/{postName}", func(w http.ResponseWriter, r *http.Request) {
			tmpl := template.Must(template.ParseFiles("post-edit-template.html"))

			postName := chi.URLParam(r, "postName")

			contents, err := ioutil.ReadFile(path.Join("raw", postName))
			if err != nil {
				http.Error(w, fmt.Sprintf("error reading '%s': ", postName)+err.Error(), 500)
				return
			}

			postMetadata, err := loadPostMetadata(postName)
			if err != nil {
				http.Error(w, fmt.Sprintf("error reading metadata for '%s': ", postName)+err.Error(), 500)
				return
			}

			tmplData := struct {
				Name         string
				Contents     string
				PostMetadata PostMetadata
			}{postName, string(contents), postMetadata}
			if tmpl.Execute(w, tmplData); err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
		})

		adminRoute.Post("/{postName}", func(w http.ResponseWriter, r *http.Request) {
			if err := r.ParseForm(); err != nil {
				http.Error(w, "error: "+err.Error(), 400)
				return
			}

			postName := chi.URLParam(r, "postName")

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

			out, err := os.Create(path.Join("build", postPublishedName(postName)))
			defer out.Close()

			if err != nil {
				http.Error(w, "error: "+err.Error(), 500)
				return
			}

			p.renderPost(out)

			http.Redirect(w, r, "/_admin/"+postName, http.StatusMovedPermanently)
		})

		adminRoute.Delete("/{postName}", func(w http.ResponseWriter, r *http.Request) {
			// TODO: secure this a bit
			postName := chi.URLParam(r, "postName")

			os.Remove(path.Join("build", postPublishedName(postName)))
			os.Remove(path.Join("raw", postName))

			http.Redirect(w, r, "/_admin/", http.StatusMovedPermanently)
		})
	})

	fs := http.StripPrefix("/", http.FileServer(http.Dir("build")))
	router.Get("/*", fs.ServeHTTP)

	log.Println("Listening on :8080")
	if err := http.ListenAndServe(":8080", router); err != nil {
		log.Fatal(err)
	}
}
