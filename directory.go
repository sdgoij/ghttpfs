package main

import (
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"

	"code.google.com/p/go.net/html"
)

type directory struct {
	node
	de map[string]fuse.Dirent
	mu sync.Mutex
}

func (d directory) Attr() fuse.Attr {
	return fuse.Attr{
		Mode:  os.ModeDir | 0500,
		Uid:   MyUID,
		Gid:   MyGID,
		Nlink: 1,
	}
}

func NewDirectory(parent *directory, fs *filesystem, name string) directory {
	return directory{node{parent, fs, name}, map[string]fuse.Dirent{}, sync.Mutex{}}
}

func (d *directory) populate() (err fuse.Error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(d.de) > 0 {
		return nil
	}
	r, err := d.fs.client.Do(d.fs.NewRequest("GET", d.fullpath()+"/"))
	if err != nil {
		log.Println("ERROR:", err)
		return err
	}
	log.Println(d.fullpath(), r.Request.URL, r.Status)
	defer r.Body.Close()

	if r.StatusCode != http.StatusOK {
		switch r.StatusCode {
		case http.StatusForbidden, http.StatusUnauthorized:
			err = fuse.EPERM
		default:
			err = fuse.ENOENT
		}
	}

	switch strings.SplitN(r.Header.Get("Content-Type"), ";", 2)[0] {
	case "text/html":
		doc, err := html.Parse(r.Body)
		if err != nil {
			return err
		}
		var walk func(*html.Node)
		walk = func(n *html.Node) {
			if n.Type == html.ElementNode && n.Data == "a" {
				for _, a := range n.Attr {
					if a.Key == "href" {
						direntType := fuse.DT_File
						if strings.HasSuffix(a.Val, "/") {
							direntType = fuse.DT_Dir
						}
						name, err := url.QueryUnescape(a.Val)
						if err != nil {
							log.Printf("url.QueryUnescape(%s): %s", a.Val, err)
						}
						name = strings.TrimRight(name, "/")
						d.de[name] = fuse.Dirent{
							Name: name,
							Type: direntType,
						}
						log.Println(a.Val, direntType)
						break
					}
				}
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
		}
		walk(doc)
	default:
		panic("Unsupported directory response.")
	}
	return
}

func (d directory) ReadDir(_ fs.Intr) (de []fuse.Dirent, err fuse.Error) {
	if err = d.populate(); nil == err {
		de = make([]fuse.Dirent, len(d.de))
		i := 0
		for _, v := range d.de {
			de[i] = v
			i++
		}
	}
	return
}

var ignoreNames = map[string]struct{}{
	"Backups.backupdb": struct{}{},
	"mach_kernel":      struct{}{},
	".DS_Store":        struct{}{},
	".hidden":          struct{}{},
	"._.":              struct{}{},
}

func (d directory) Lookup(name string, _ fs.Intr) (fs.Node, fuse.Error) {
	if _, ignore := ignoreNames[name]; !ignore && !strings.HasPrefix(name, "._") {
		log.Printf("directory.Lookup(name=%s, path=%s)", name, d.fullpath())
		if err := d.populate(); err != nil {
			return nil, fuse.ENOENT
		}
		if de, ok := d.de[name]; ok {
			switch de.Type {
			case fuse.DT_Dir:
				return NewDirectory(&d, d.fs, name), nil
			case fuse.DT_File:
				return NewFile(&d, d.fs, name)
			default:
				panic("should not happen.")
			}
		}
	}
	return nil, fuse.ENOENT
}
