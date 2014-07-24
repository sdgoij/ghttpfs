package main

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

type file struct {
	node
	size uint64
}

func NewFile(parent *directory, fs *filesystem, name string) (node file, err error) {
	node.parent = parent
	node.name = name
	node.fs = fs

	if r, e := fs.client.Do(fs.NewRequest("HEAD", node.fullpath())); e == nil {
		defer r.Body.Close()
		if r.StatusCode != http.StatusOK {
			switch r.StatusCode {
			case http.StatusForbidden, http.StatusUnauthorized:
				err = fuse.EPERM
			default:
				err = fuse.ENOENT
			}
			return
		}
		if s, e := strconv.ParseUint(r.Header.Get("Content-Length"), 10, 64); e == nil {
			node.size = s
		}
	}

	return
}

func (f file) Attr() fuse.Attr {
	return fuse.Attr{
		Size:  f.size,
		Mode:  0400,
		Uid:   MyUID,
		Gid:   MyGID,
		Nlink: 1,
	}
}

func (f file) Read(req *fuse.ReadRequest, resp *fuse.ReadResponse, _ fs.Intr) fuse.Error {
	if !req.Dir {
		start, end := req.Offset, req.Offset+int64(req.Size)-1
		log.Printf("file.Read(file=%s, start=%d, end=%d)", f.fullpath(), start, end)

		req := f.fs.NewRequest("GET", f.fullpath())
		req.Header.Add("Range", fmt.Sprintf("bytes=%d-%d", start, end))

		r, err := f.fs.client.Do(req)
		if err != nil {
			log.Println(err)
			return fuse.EIO
		}
		defer r.Body.Close()

		if r.StatusCode != http.StatusOK && r.StatusCode != http.StatusPartialContent {
			log.Println(f.fullpath(), r.Status)
			return fuse.EIO
		}

		buf := &bytes.Buffer{}
		buf.ReadFrom(r.Body)
		resp.Data = buf.Bytes()

		return nil
	}
	return fuse.EIO
}
