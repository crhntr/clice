package main

import (
	"embed"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/julienschmidt/httprouter"
)

//go:embed assets
var assets embed.FS

func main() {
	mux := httprouter.New()
	mux.Handler(http.MethodGet, "/assets/*filepath", http.FileServer(http.FS(assets)))
	mux.Handler(http.MethodGet, "/main.wasm", http.HandlerFunc(mainWASM))
	mux.Handler(http.MethodGet, "/", http.HandlerFunc(indexHTTP))
	err := http.ListenAndServe(":8080", mux)
	if err != nil {
		panic(err)
	}
}

func indexHTTP(res http.ResponseWriter, req *http.Request) {
	file, err := assets.Open("assets/index.html")
	if err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	defer closeAndIgnoreError(file)
	stat, _ := file.Stat()
	http.ServeContent(res, req, "index.html", stat.ModTime(), file.(io.ReadSeeker))
}

func mainWASM(res http.ResponseWriter, req *http.Request) {
	pkg := req.URL.Query().Get("package")
	if pkg == "" {
		http.Error(res, "package not set", http.StatusBadRequest)
		return
	}

	dir, err := os.MkdirTemp("", "")
	if pkg == "" {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() {
		err := os.RemoveAll(dir)
		if err != nil {
			log.Printf("failed to delete directory %q: %s", dir, err)
		}
	}()

	fp := filepath.Join(dir, "main.wasm")
	cmd := exec.CommandContext(req.Context(), "go", "build", "-v", "-o", fp, pkg)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout

	cmd.Env = cmd.Environ()
	cmd.Env = setEnv(cmd.Env, "GOOS", "js")
	cmd.Env = setEnv(cmd.Env, "GOARCH", "wasm")

	err = cmd.Run()
	if err != nil {
		http.Error(res, err.Error(), http.StatusBadRequest)
		return
	}

	mainWASMFile, err := os.Open(fp)
	if err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	defer closeAndIgnoreError(mainWASMFile)

	res.Header().Set("content-type", "application/wasm")
	res.WriteHeader(http.StatusOK)

	_, err = io.Copy(res, mainWASMFile)
	if err != nil {
		log.Println(err)
		return
	}
}

func setEnv(env []string, name, value string) []string {
	filtered := env[:0]
	found := false
	variable := strings.Join([]string{name, value}, "=")
	for _, e := range env {
		if strings.HasPrefix(e, name+"=") {
			found = true
			filtered = append(filtered, variable)
		} else {
			filtered = append(filtered, e)
		}
	}
	if !found {
		filtered = append(filtered, variable)
	}
	return filtered
}

func closeAndIgnoreError(c io.Closer) {
	_ = c.Close()
}
