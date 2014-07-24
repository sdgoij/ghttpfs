ghttpfs
--

Simple (experimental) FUSE based HTTP (read-only) filesystem, written in [Go](http://golang.org/).

```bash
go get github.com/sdgoij/ghttpfs
mkdir example.com
ghttpfs -http.addr="http://example.com/" example.com; umount example.com
```
