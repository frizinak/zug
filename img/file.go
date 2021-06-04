package img

import (
	"fmt"
	"io/fs"
	"net/url"
	"os"
)

type fileHandler struct {
}

func (h *fileHandler) Get(u *url.URL, d string) (ok bool, temp bool, path string, err error) {
	if (u.Scheme != "" && u.Scheme != "file") || u.Host != "" {
		return
	}

	ok = true
	var stat fs.FileInfo
	stat, err = os.Stat(u.Path)
	if err != nil {
		return
	}
	if stat.IsDir() {
		err = fmt.Errorf("'%s' is a directory", u.Path)
		return
	}

	path = u.Path
	return
}

var FileH = &fileHandler{}
