package main

import (
	"flag"
	"log"
	"os"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

var (
	flagHTTPAddr = flag.String("http.addr", "http://localhost/", "HTTP(S) server address to connect to.")
	flagHTTPAuth = flag.String("http.auth", "", "Authenticate using username:password.")
	flagHTTPSkip = flag.Bool("http.insecure-skip-verify", true, "Controls whether a client verifies the server's certificate chain and host name.")
	flagAcceptJSON = flag.Bool("json", false, "Add Accept application/json header for directory listing requests.")
)

func main() {
	flag.Usage = func() {
		flag.PrintDefaults()
	}
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lshortfile)

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	conn, err := fuse.Mount(flag.Arg(0))
	if err != nil {
		log.Fatalln(err)
	}
	defer func() {
		fuse.Unmount(flag.Arg(0))
		conn.Close()
	}()

	if err := fs.Serve(conn, NewFS(*flagHTTPAddr, *flagHTTPSkip, *flagAcceptJSON)); err != nil {
		log.Fatalln(err)
	}

	<-conn.Ready
	if err := conn.MountError; err != nil {
		log.Fatalln(err)
	}
}
