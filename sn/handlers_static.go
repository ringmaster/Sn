package sn

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/afero"
	"github.com/spf13/viper"
)

func customFileServer(fs afero.Fs, file string) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		fileContents, err := fs.Open(file)
		if err != nil {
			http.Error(rw, "File not found", http.StatusNotFound)
			return
		}
		defer fileContents.Close()
		content, err := io.ReadAll(fileContents)
		if err != nil {
			http.Error(rw, "Error reading file", http.StatusInternalServerError)
			return
		}
		http.ServeContent(rw, r, path.Base(file), time.Now(), bytes.NewReader(content))
	})
}

func replaceBasePath(content []byte, basePath string) []byte {
	result := []byte(strings.ReplaceAll(string(content), "{{BASE_PATH}}", basePath))
	result = []byte(strings.ReplaceAll(string(result), "{{UNSPLASH}}", viper.GetString("unsplash")))
	return result
}

func customDirServer(fs afero.Fs, routeName string, prefix string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Info("serving file", "route", routeName, "path", r.URL.Path)
		upath := r.URL.Path
		if strings.HasSuffix(r.URL.Path, "/") {
			upath = path.Join(upath, "index.html")
		}

		filePath := filepath.Join(prefix, path.Clean(upath))
		file, err := fs.Open(filePath)
		var doNotFound = false
		var stat os.FileInfo
		// If the file doesn't exist, try to serve the index.html file
		if os.IsNotExist(err) {
			doNotFound = true
		} else {
			stat, err = file.Stat()
			if err != nil {
				doNotFound = true
			} else if stat.IsDir() {
				doNotFound = true
				filePath = filePath + "/"
			}
		}

		if doNotFound {
			// Try to find an index.html file in progressive directories from prefix up to filePath
			dirPath := filePath
			for {
				dirPath = filepath.Dir(dirPath)
				fmt.Printf("dirPath: %s\n", dirPath)
				if dirPath == "." || dirPath == "/" {
					break
				}
				indexFilePath := filepath.Join(dirPath, "index.html")
				indexFile, indexErr := fs.Open(indexFilePath)
				if indexErr == nil {
					defer indexFile.Close()
					indexContent, indexErr := io.ReadAll(indexFile)
					if indexErr != nil {
						http.Error(w, "Error reading index file", http.StatusInternalServerError)
						return
					}
					indexContent = replaceBasePath(indexContent, viper.GetString(fmt.Sprintf("routes.%s.path", routeName)))
					http.ServeContent(w, r, "index.html", time.Now(), bytes.NewReader(indexContent))
					return
				}
			}
			w.WriteHeader(http.StatusNotFound)
			http.Error(w, fmt.Sprintf("404.1: %s Cannot find an index.html between %#v and %#v", routeName, filePath, prefix), http.StatusNotFound)
			return
		}
		defer file.Close()

		content, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, "Error reading file", http.StatusInternalServerError)
			return
		}
		content = replaceBasePath(content, viper.GetString(fmt.Sprintf("routes.%s.path", routeName)))
		reader := bytes.NewReader(content)
		http.ServeContent(w, r, stat.Name(), stat.ModTime(), reader)
	})
}
