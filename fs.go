package main

import (
	"crypto/tls"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

var (
	_ fs.FS   = fs.FS(filesystem{})
	_ fs.Node = fs.Node(directory{})
	_ fs.Node = fs.Node(&file{})
)

var (
	MyUID = uint32(os.Getuid())
	MyGID = uint32(os.Getgid())
)

type filesystem struct {
	baseURL string
	client  *http.Client
	root    directory
}

func (fs filesystem) Root() (fs.Node, fuse.Error) {
	return fs.root, nil
}

func (fs filesystem) NewRequest(method, path string) *http.Request {
	if strings.HasSuffix(path, "/") {
		path += "/"
	}
	req, _ := http.NewRequest(method, fs.baseURL+path, nil)
	if *flagHTTPAuth != "" {
		auth := strings.Split(*flagHTTPAuth, ":")
		if len(auth) < 2 {
			auth = append(auth, "")
		}
		req.SetBasicAuth(auth[0], auth[1])
	}
	return req
}

func NewFS(url string, skipVerify bool) filesystem {
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}
	fs := filesystem{
		baseURL: url,
		client: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: skipVerify},
			},
		},
		root: NewDirectory(nil, nil, ""),
	}
	fs.root.fs = &fs
	return fs
}

type node struct {
	parent *directory
	fs     *filesystem
	name   string
}

func (n *node) fullpath() (path string) {
	if n != nil {
		if n.parent != nil {
			path = n.parent.fullpath()
		}
		path = filepath.Join(path, n.name)
	}
	return
}
