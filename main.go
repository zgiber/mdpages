package main

import (
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/jessevdk/go-flags"
	"github.com/spf13/afero"
)

const (
	markdownExt = ".md"
)

var Opts struct {
	RootDir string `long:"root-dir" short:"r" description:"The root directory for starting collecting .md documents from."`
}

var (
	AppFs = afero.NewMemMapFs()
)

func main() {
	_, err := flags.Parse(&Opts)
	if err != nil {
		log.Fatal(err)
	}

	// extensions := parser.CommonExtensions | parser.AutoHeadingIDs
	// parser := parser.NewWithExtensions(extensions)

	htmlFlags := html.CommonFlags | html.HrefTargetBlank
	opts := html.RendererOptions{Flags: htmlFlags}
	renderer := html.NewRenderer(opts)

	fileSystem := os.DirFS(Opts.RootDir)
	fs.WalkDir(fileSystem, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Fatal(err)
		}

		if path.Ext(p) == markdownExt {
			if err := AppFs.MkdirAll(path.Dir(p), fs.ModeDir|fs.ModePerm); err != nil {
				log.Fatal(err)
			}

			f, err := AppFs.Create(p)
			if err != nil {
				log.Fatal(err)
			}

			// src, err := os.Open()

			md, err := os.ReadFile(path.Join(Opts.RootDir, path.Dir(p), path.Base(p)))
			if err != nil {
				log.Fatal(err)
			}

			output := markdown.ToHTML(md, nil, renderer)
			_, err = f.Write(output)
			if err != nil {
				log.Fatal(err)
			}

			f.Close()
		}

		return nil
	})

	httpFS := afero.NewHttpFs(AppFs)
	http.Handle("/", http.FileServer(httpFS.Dir(".")))
	err = http.ListenAndServe(":8000", nil)
	if err != nil {
		log.Fatal(err)
	}
}
