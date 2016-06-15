package router

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const wildcard = "*"

// Request is an extended value with request params
type Request struct {
	*http.Request
	Params map[string]string
}

// Download serves given file to client
func (r *Request) Download(res http.ResponseWriter, path string) (err error) {
	file, err := os.Open(path)
	if err == nil {
		stat, _ := file.Stat()
		res.Header().Set("Content-Disposition", "attachment; filename=\""+file.Name()[max(strings.LastIndex(file.Name(), string(os.PathSeparator))+1, 0):]+"\"")
		http.ServeContent(res, r.Request, file.Name(), stat.ModTime(), file)
		return
	}
	return
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func isURLMatching(path string, req *http.Request) (*Request, bool) {
	ret := &Request{
		req,
		make(map[string]string),
	}
	if path == wildcard {
		return ret, true
	}

	url := req.URL.Path

	i := 0

	if url[len(url)-1] != '/' {
		url += "/"
	}
	length := len(path)

	wildcard, param := false, false

	var b byte
	var key, value string
	for _, b = range []byte(url) {
		if b == '/' {
			if wildcard && i < length-1 {
				i++
			}

			if param {
				ret.Params[key] = value

				key, value = "", ""
				param = false
				i--
			}
			wildcard = false
		}

		cur := i

		switch path[cur] {
		case ':':
			for i++; i < length-1 && path[i] != '/'; i++ {
				key += string(path[i])
			}
			param = true
			fallthrough
		case '*':
			wildcard = true
		}

		if param {
			value += string(b)
		}

		if wildcard {
			continue
		}

		if i < length-1 {
			i++
		}

		if b != path[cur] {
			return ret, false
		}
	}

	return ret, path[i] == b
}

// Router is router
type Router struct {
	paths      []string
	handlers   map[string]func(res http.ResponseWriter, req *Request, next func())
	preprocess []func(res http.ResponseWriter, req *http.Request, next func())
	sort       bool
}

// StaticFolder provides function which serves static files
func StaticFolder(path string) func(res http.ResponseWriter, req *http.Request, next func()) {
	return func(res http.ResponseWriter, req *http.Request, next func()) {
		if files, err := filepath.Glob(filepath.Join(path, req.URL.Path)); len(files) > 0 && err == nil {
			for _, f := range files {
				stat, err := os.Stat(f)
				if !os.IsNotExist(err) && !stat.IsDir() {
					res.Header().Set("Content-Disposition", "attachment; filename=\""+stat.Name()[max(strings.LastIndex(stat.Name(), string(os.PathSeparator))+1, 0):]+"\"")
					http.ServeFile(res, req, f)
					return
				}
			}
		}

		next()
	}
}

// NewRouter returns new router
func NewRouter() *Router {
	return &Router{
		handlers: make(map[string]func(res http.ResponseWriter, req *Request, next func())),
	}
}

// HandlePreprocess registers preprocessing function
func (r *Router) HandlePreprocess(handler func(res http.ResponseWriter, req *http.Request, next func())) {
	r.preprocess = append(r.preprocess, handler)
}

// HandleFunc is used to register a handling function
func (r *Router) HandleFunc(path string, handler func(res http.ResponseWriter, req *Request, next func())) {
	if path == "" {
		path = wildcard
	}

	if path[len(path)-1] != '/' && path != wildcard {
		path += "/"
	}

	if path[0] != '/' && path != wildcard {
		path = "/" + path
	}

	r.handlers[path] = handler

	r.paths = append(r.paths, path)

	sorter := &routerSorter{r.paths}
	sort.Sort(sorter)
}

/*
// HandleRouter is used to register router for handling
func (r* Router) HandleRouter(path string, router *Router){
	if path[len(path) - 1] == '/' {
		path = path[:len(path) - 1]
	}

	r.HandleFunc(path, func(res http.ResponseWriter, req *http.Request, next func()){
		router.Route(path + req.URL.Path, res, req)
	})
}*/

// GetHandler returns handler for the path
func (r *Router) GetHandler(path string) (func(res http.ResponseWriter, req *Request, next func()), bool) {
	handler, ok := r.handlers[path]
	return handler, ok
}

// Preprocess executes preprocessing function
func (r *Router) Preprocess(res http.ResponseWriter, req *http.Request) bool {
	con := true
	for _, f := range r.preprocess {
		con = false
		f(res, req, func() {
			con = true
		})

		if !con {
			break
		}
	}
	return con
}

// Route routes the path
func (r *Router) Route(res http.ResponseWriter, req *http.Request) {
	if r.Preprocess(res, req) {
		for _, p := range r.paths {
			request, ok := isURLMatching(p, req)
			if !ok {
				continue
			}

			f, ok := r.handlers[p]
			if ok {
				con := false
				f(res, request, func() {
					con = true
				})

				if !con {
					return
				}
			}
		}
	}
}

// GetPaths returns all registered path
func (r *Router) GetPaths() []string {
	return r.paths
}

type routerSorter struct {
	paths []string
}

func (r *routerSorter) Len() int {
	return len(r.paths)
}

func (r *routerSorter) Less(i, j int) bool {
	if r.paths[i] == wildcard {
		return false
	}

	rel, err := filepath.Rel(r.paths[i], r.paths[j])
	if err != nil || rel[0] == '.' {
		return true
	}

	return false
}

func (r *routerSorter) Swap(i, j int) {
	r.paths[i], r.paths[j] = r.paths[j], r.paths[i]
}
