package layers

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/ooyala/go-docker-registry/src/registry/logger"
	"hash"
	"io"
	"sort"
	"strings"
)

const TAR_FILES_INFO_SIZE = 8

type TarError string

func (e TarError) Error() string {
	return string(e)
}

type TarInfo struct {
	TarSum       *TarSum
	TarFilesInfo *TarFilesInfo
	Error        error
}

func NewTarInfo() *TarInfo {
	return &TarInfo{
		TarSum:       NewTarSum(),
		TarFilesInfo: NewTarFilesInfo(),
		Error:        nil,
	}
}

func (t *TarInfo) Load(file io.ReadSeeker) {
	var reader *tar.Reader
	file.Seek(0, 0)
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		// likely not a gzip compressed file
		file.Seek(0, 0)
		reader = tar.NewReader(file)
	} else {
		reader = tar.NewReader(gzipReader)
	}
	for {
		header, err := reader.Next()
		if err == io.EOF {
			// end of tar file
			break
		} else if err != nil {
			// error occured
			logger.Debug("[TarInfoLoad] Error when reading tar stream tarsum. Disabling TarSum, TarFilesInfo. Error: %s", err.Error())
			t.Error = TarError(err.Error())
			return
		}
		t.TarSum.Append(header, reader)
		t.TarFilesInfo.Append(header)
	}
}

type TarSum struct {
	hashes []string
	sha    hash.Hash
}

func NewTarSum() *TarSum {
	return (&TarSum{}).init()
}

func (t *TarSum) init() *TarSum {
	t.hashes = []string{}
	t.sha = sha256.New()
	t.sha.Reset()
	return t
}

func (t *TarSum) Append(header *tar.Header, reader io.Reader) {
	headerStr := "name" + header.Name
	headerStr += fmt.Sprintf("mode%d", header.Mode)
	headerStr += fmt.Sprintf("uid%d", header.Uid)
	headerStr += fmt.Sprintf("gid%d", header.Gid)
	headerStr += fmt.Sprintf("size%d", header.Size)
	headerStr += fmt.Sprintf("mtime%d", header.ModTime.UTC().Unix())
	headerStr += fmt.Sprintf("typeflag%c", header.Typeflag)
	headerStr += "linkname" + header.Linkname
	headerStr += "uname" + header.Uname
	headerStr += "gname" + header.Gname
	headerStr += fmt.Sprintf("devmajor%d", header.Devmajor)
	headerStr += fmt.Sprintf("devminor%d", header.Devminor)
	t.sha.Reset()
	if header.Size > int64(0) {
		t.sha.Write([]byte(headerStr))
		_, err := io.Copy(t.sha, reader)
		if err != nil {
			logger.Debug("[TarSumAppend] error copying to sha: %s", err.Error())
			t.sha.Reset()
			t.sha.Write([]byte(headerStr))
		}
	} else {
		t.sha.Write([]byte(headerStr))
	}
	t.hashes = append(t.hashes, hex.EncodeToString(t.sha.Sum(nil)))
}

func (t *TarSum) Compute(seed []byte) string {
	logger.Debug("[TarSumCompute] seed:\n<<%s>>", seed)
	sort.Strings(t.hashes)
	t.sha.Reset()
	t.sha.Write(seed)
	for _, hash := range t.hashes {
		t.sha.Write([]byte(hash))
	}
	tarsum := "tarsum+sha256:" + hex.EncodeToString(t.sha.Sum(nil))
	logger.Debug("[TarSumCompute] return %s", tarsum)
	return tarsum
}

type TarFilesInfo struct {
	headers []*tar.Header
}

func NewTarFilesInfo() *TarFilesInfo {
	return &TarFilesInfo{headers: []*tar.Header{}}
}

func (t *TarFilesInfo) Load(file io.Reader) error {
	reader := tar.NewReader(file)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			// end of tar file
			break
		} else if err != nil {
			// error occured
			return TarError(err.Error())
		}
		t.Append(header)
	}
	return nil
}

func (t *TarFilesInfo) Append(h *tar.Header) {
	t.headers = append(t.headers, h)
}

// This function returns json containing a slice of file info objects
// file info is a weird tuple (why it isn't just a map i have no idea)
// file info:
// [
//   filename,
//   file type,
//   is deleted,
//   size,
//   mod time,
//   mode,
//   uid,
//   gid
// ]
func (t *TarFilesInfo) Json() ([]byte, error) {
	// convert to the weird tuple docker-registry 0.6.5 uses (why wasn't this just a map!?)
	tupleSlice := [][]interface{}{}
	for _, header := range t.headers {
		filename := header.Name
		isDeleted := false
		if filename == "." {
			filename = "/"
		}
		if strings.HasPrefix(filename, "./") {
			filename = "/" + strings.TrimPrefix(filename, "./")
		}
		if strings.HasPrefix(filename, "/.wh.") {
			filename = "/" + strings.TrimPrefix(filename, "/.wh.")
			isDeleted = true
		}
		// NOTE(edanaher): Well, if filename started with /.wh..wh., this could trigger.  Presumably, .wh is a
		// tombstone (WHiteout) indicating the file was deleted, and if it was recreated, you "delete" the
		// tombstone, which could conceivable mean a double-tombstone is a no-op.  I feel like it would take more
		// logic to make that work, but it makes some semblance of sense.
		if strings.HasPrefix(filename, "/.wh.") {
			continue
		}

		filetype := "u"
		switch header.Typeflag {
		case tar.TypeReg:
			fallthrough
		case tar.TypeRegA:
			filetype = "f"
		case tar.TypeLink:
			filetype = "l"
		case tar.TypeSymlink:
			filetype = "s"
		case tar.TypeChar:
			filetype = "c"
		case tar.TypeBlock:
			filetype = "b"
		case tar.TypeDir:
			filetype = "d"
		case tar.TypeFifo:
			filetype = "i"
		case tar.TypeCont:
			filetype = "t"
		case tar.TypeGNULongName:
			fallthrough
		case tar.TypeGNULongLink:
			fallthrough
		case 'S': // GNU Sparse (for some reason archive/tar doesn't have a constant for it)
			filetype = string([]byte{header.Typeflag})
		}

		tupleSlice = append(tupleSlice, []interface{}{
			filename,
			filetype,
			isDeleted,
			header.Size,
			header.ModTime.Unix(),
			header.Mode,
			header.Uid,
			header.Gid,
		})
	}
	return json.Marshal(&tupleSlice)
}
