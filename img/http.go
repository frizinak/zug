package img

import (
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
)

type httpHandler struct {
}

func (h *httpHandler) Get(u *url.URL, dir string) (ok bool, temp bool, file string, err error) {
	temp = true
	if u.Scheme != "http" && u.Scheme != "https" {
		return
	}

	ok = true
	hash := sha256.Sum256([]byte(u.String()))
	file = filepath.Join(dir, base64.RawURLEncoding.EncodeToString(hash[:]))
	if stat, _ := os.Stat(file); stat != nil {
		return
	}
	err = get(u, file)
	return
}

func get(u *url.URL, dest string) error {
	res, err := http.Get(u.String())
	if err != nil {
		return err
	}
	defer res.Body.Close()

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, res.Body)
	return err
}

var HttpH = &httpHandler{}
