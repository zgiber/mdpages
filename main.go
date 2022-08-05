package main

import (
	"bytes"
	_ "embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/gomarkdown/markdown"
	mdhtml "github.com/gomarkdown/markdown/html"
	"github.com/jessevdk/go-flags"
	"github.com/spf13/afero"
	"golang.org/x/net/html"
)

const (
	markdownExt = ".md"
)

var Opts struct {
	RootDir string `long:"root-dir" short:"r" description:"The root directory for starting collecting .md documents from."`
}

var (
	AppFs = afero.NewMemMapFs()

	//go:embed github-markdown.css
	externalCSS []byte

	ignored = map[string]struct{}{
		"node_modules": {},
	}
)

func main() {
	_, err := flags.Parse(&Opts)
	if err != nil {
		log.Fatal(err)
	}

	htmlFlags := mdhtml.CommonFlags | mdhtml.HrefTargetBlank
	opts := mdhtml.RendererOptions{Flags: htmlFlags}
	renderer := mdhtml.NewRenderer(opts)

	f, err := AppFs.Create("github-markdown.css")
	if err != nil {
		log.Fatal(err)
	}
	_, err = f.Write(externalCSS)
	if err != nil {
		log.Fatal(err)
	}
	f.Close()

	fileSystem := os.DirFS(Opts.RootDir)
	fs.WalkDir(fileSystem, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Fatal(err)
		}

		for ignore := range ignored {
			if strings.HasPrefix(path.Dir(p), ignore) {
				return nil
			}
		}

		if path.Ext(p) == markdownExt {
			if err := AppFs.MkdirAll(path.Dir(p), fs.ModeDir|fs.ModePerm); err != nil {
				log.Fatal(err)
			}

			htmlDocPath := strings.ReplaceAll(p, ".md", ".html")
			f, err := AppFs.Create(htmlDocPath)
			if err != nil {
				log.Fatal(err)
			}

			// fmt.Println("copying ", path.Join(Opts.RootDir, path.Dir(p), path.Base(p)), "to", p)
			md, err := os.ReadFile(path.Join(Opts.RootDir, path.Dir(p), path.Base(p)))
			if err != nil {
				log.Fatal(err)
			}

			output := markdown.ToHTML(md, nil, renderer)
			doc, err := html.Parse(bytes.NewReader(output))
			if err != nil {
				log.Println(err)
				return nil
			}

			addStyle(doc)
			processDocument(d, p)(doc)

			err = html.Render(f, doc)
			if err != nil {
				log.Fatal(err)
			}

			f.Close()
		}

		return nil
	})

	log.Println("running http server at port 8000")
	httpFS := afero.NewHttpFs(AppFs)
	http.Handle("/", http.FileServer(httpFS.Dir(".")))
	err = http.ListenAndServe(":8000", nil)
	if err != nil {
		log.Fatal(err)
	}
}

func addStyle(n *html.Node) {
	if n.Type == html.ElementNode && n.Data == "head" {
		n.AppendChild(style())
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		addStyle(c)
	}
}

func updateMDLink(n *html.Node) {
	for i := 0; i < len(n.Attr); i++ {
		a := n.Attr[i]
		if a.Key == "href" {
			// rewrite .md links to .html
			if strings.HasSuffix(a.Val, ".md") {
				n.Attr[i].Val = strings.ReplaceAll(a.Val, ".md", ".html") // watch out for false matches
			}
		}

		// open links in same window
		if a.Key == "target" {
			n.Attr[i].Val = "_self"
		}
	}
}

func packLocalImage(n *html.Node, currentPath string) {
	for i := 0; i < len(n.Attr); i++ {
		a := n.Attr[i]
		if a.Key == "src" {
			a.Val = strings.Split(a.Val, "?")[0]

			// ignore remote images
			if strings.HasPrefix(a.Val, "http") {
				continue
			}

			p := (path.Join(path.Dir(currentPath), a.Val))
			src, err := os.ReadFile(path.Join(Opts.RootDir, p))
			if err != nil {
				log.Println(err)
				continue
			}

			dst, err := AppFs.Create(p)
			if err != nil {
				log.Fatal(err)
			}

			_, err = dst.Write(src)
			if err != nil {
				log.Fatal(err)
			}
			break
		}
	}
}

func createID(n *html.Node) {
	id := strings.ToLower(clean([]byte(n.FirstChild.Data)))
	n.Attr = append(n.Attr, html.Attribute{
		Key: "id",
		Val: id,
	})
}

func addStyleClass(n *html.Node) {
	n.Attr = append(n.Attr, html.Attribute{
		Key: "class",
		Val: "markdown-body",
	})
}

func processDocument(d fs.DirEntry, currentPath string) func(*html.Node) {
	return func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "img":
				packLocalImage(n, currentPath)
			case "a":
				updateMDLink(n)
			case "h1", "h2", "h3", "h4", "h5", "h6":
				createID(n)
			case "body":
				addStyleClass(n)
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			processDocument(d, currentPath)(c)
		}
	}
}

var style = func() *html.Node {
	n, err := html.Parse(strings.NewReader(css))
	if err != nil {
		log.Fatal(err)
	}
	return n
}

const css = `
<link rel="stylesheet" href="http://localhost:8000/github-markdown.css">
<style>
	.markdown-body {
		box-sizing: border-box;
		min-width: 200px;
		max-width: 980px;
		margin: 0 auto;
		padding: 45px;
	}

	@media (max-width: 767px) {
		.markdown-body {
			padding: 15px;
		}
	}
</style>
`

// https://stackoverflow.com/questions/54461423/efficient-way-to-remove-all-non-alphanumeric-characters-from-large-text
// :)
func clean(s []byte) string {
	j := 0
	for _, b := range s {
		if ('0' <= b && b <= '9') ||
			('a' <= b && b <= 'z') ||
			('A' <= b && b <= 'Z') {
			s[j] = b
			j++
		}
		if b == ' ' {
			s[j] = '-'
			j++
		}
	}
	return string(s[:j])
}
