package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

type file struct {
	node
	size uint64 // total filesize
	buf  []byte // buffer of pre-feched data
	pos  uint64 // start position of buf
}

func NewFile(parent *directory, fs *filesystem, name string) (f *file, err error) {
	f = &file{node{parent, fs, name}, 0, nil, 0}

	if r, e := fs.client.Do(fs.NewRequest("HEAD", f.fullpath())); e == nil {
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
			f.size = s
		}
	}

	return
}

func (f *file) Attr() fuse.Attr {
	return fuse.Attr{
		Size:  f.size,
		Mode:  0400,
		Uid:   MyUID,
		Gid:   MyGID,
		Nlink: 1,
	}
}

func (f *file) Read(req *fuse.ReadRequest, resp *fuse.ReadResponse, _ fs.Intr) fuse.Error {
	if !req.Dir {
		offset, size, bpos := int64(req.Offset), int64(req.Size), int64(0)
		if bpos = f.offsetToBufferPos(offset, size); bpos < 0 {
			end := offset + size*10 - 1 // pre-fetch 10x the requested data, 4kB -> 40kB
			log.Printf("file.Read(file=%s path=%s): Fetching data start=%d, end=%d, size=%d",
				f.name, f.parent.fullpath(), offset, end, size*10)

			req := f.fs.NewRequest("GET", f.fullpath())
			req.Header.Add("Range", fmt.Sprintf("bytes=%d-%d", offset, end))

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

			if f.buf, err = ioutil.ReadAll(r.Body); err != nil {
				return fuse.EIO
			}

			f.pos = uint64(offset)
			bpos = 0
		}

		log.Printf("file.Read(file=%s path=%s): Reading start=%d/%d end=%d",
			f.name, f.parent.fullpath(), offset, bpos, offset+size)

		resp.Data = f.buf[bpos : bpos+size]
		return nil
	}
	return fuse.EIO
}

func (f *file) offsetToBufferPos(offset, size int64) int64 {
	if pos := offset - int64(f.pos); pos >= 0 && pos+size <= int64(len(f.buf)) {
		return pos
	}
	return -1
}
