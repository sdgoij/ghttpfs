package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"

	"github.com/mholt/caddy/middleware/browse"

	"golang.org/x/net/html"
)

type directory struct {
	node
	de map[string]dirent
	mu sync.Mutex
}

type dirent struct {
	fuse.Dirent
	mtime time.Time
}

func (d *directory) Attr() fuse.Attr {
	return fuse.Attr{
		Mode:  os.ModeDir | 0500,
		Uid:   MyUID,
		Gid:   MyGID,
		Nlink: 1,
		Mtime: d.mtime(),
	}
}

func NewDirectory(parent *directory, fs *filesystem, name string) *directory {
	return &directory{
		node: node{parent, fs, name},
		de:   map[string]dirent{},
	}
}

func (d *directory) populate() (err fuse.Error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(d.de) > 0 {
		return nil
	}
	req := d.fs.NewRequest("GET", d.fullpath()+"/")
	req.Header.Set("Accept", "application/json")
	r, err := d.fs.client.Do(req)
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
						if a.Val[len(a.Val)-1] == '/' {
							direntType = fuse.DT_Dir
						}
						name, err := url.QueryUnescape(a.Val)
						if err != nil {
							log.Printf("url.QueryUnescape(%s): %s", a.Val, err)
						}
						name = strings.TrimRight(name, "/")
						d.de[name] = dirent{
							fuse.Dirent{
								Name: name,
								Type: direntType,
							},
							ParseTime(n.NextSibling),
						}
						log.Println(name, direntType)
						break
					}
				}
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
		}
		walk(doc)
		time.AfterFunc(30*time.Second, func() {
			d.reset()
		})
	case "application/json":
		if r.Header.Get("Server") == "Caddy" {
			var entries []browse.FileInfo
			data, _ := ioutil.ReadAll(r.Body);

			if err := json.Unmarshal(data, &entries); nil != err {
				log.Printf("Error: %s", err)
			}
			for _, info := range entries {
				direntType := fuse.DT_File
				if info.IsDir {
					direntType = fuse.DT_Dir
				}
				d.de[info.Name] = dirent{
					fuse.Dirent{
						Name: info.Name,
						Type: direntType,
					},
					info.ModTime,
				}
				log.Println(info.Name, direntType)
			}
		}
	default:
		panic("Unsupported directory response.")
	}
	return
}

func (d *directory) ReadDir(_ fs.Intr) (de []fuse.Dirent, err fuse.Error) {
	if err = d.populate(); nil == err {
		de = make([]fuse.Dirent, len(d.de))
		i := 0
		for _, v := range d.de {
			de[i] = v.Dirent
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

func (d *directory) Lookup(name string, _ fs.Intr) (fs.Node, fuse.Error) {
	if _, ignore := ignoreNames[name]; !ignore && !strings.HasPrefix(name, "._") {
		log.Printf("directory.Lookup(name=%s, path=%s)", name, d.fullpath())
		if err := d.populate(); err != nil {
			return nil, fuse.ENOENT
		}
		if de, ok := d.de[name]; ok {
			switch de.Type {
			case fuse.DT_Dir:
				return NewDirectory(d, d.fs, name), nil
			case fuse.DT_File:
				return NewFile(d, d.fs, name)
			default:
				panic("should not happen.")
			}
		}
	}
	return nil, fuse.ENOENT
}

func (d *directory) reset() {
	d.mu.Lock()
	size := len(d.de)
	log.Printf("directory.reset(%s): %d children", d.fullpath(), size)
	d.de = make(map[string]dirent, size)
	d.mu.Unlock()
}

func ParseTime(n *html.Node) (t time.Time) {
	if ts := prepareTimeString(n.Data); ts != "" {
		var err error
		t, err = time.Parse("2-Jan-2006 15:04", ts)
		if err != nil {
			log.Printf("ParseTime('%s'): %s", ts, err)
		}
	}
	return
}

func prepareTimeString(ts string) string {
	return strings.Trim(strings.Join(strings.SplitN(
		strings.Trim(ts, "\t "), " ", 3)[0:2], " "), "\r\n\t ")
}
