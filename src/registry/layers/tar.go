package layers

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"registry/logger"
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

func NewTarInfo(sumSeed []byte) *TarInfo {
	return &TarInfo{
		TarSum:       NewTarSum(sumSeed),
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
	seed   []byte
	hashes []string
}

func NewTarSum(seed []byte) *TarSum {
	return (&TarSum{}).init(seed)
}

func (t *TarSum) init(seed []byte) *TarSum {
	t.seed = seed
	t.hashes = []string{}
	return t
}

func (t *TarSum) Append(header *tar.Header, reader io.Reader) {
	headerStr := ""
	headerStr += "name" + header.Name
	if header.Typeflag == tar.TypeDir && !strings.HasSuffix(headerStr, "/") {
		headerStr += "/"
	}
	headerStr += fmt.Sprintf("mode%d", header.Mode)
	headerStr += fmt.Sprintf("uid%d", header.Uid)
	headerStr += fmt.Sprintf("gid%d", header.Gid)
	headerStr += fmt.Sprintf("size%d", header.Size)
	headerStr += fmt.Sprintf("mtime%d", header.ModTime.Unix())
	headerStr += fmt.Sprintf("typeflag%c", header.Typeflag)
	headerStr += "linkname" + header.Linkname
	headerStr += "uname" + header.Uname
	headerStr += "gname" + header.Gname
	headerStr += fmt.Sprintf("devmajor%d", header.Devmajor)
	headerStr += fmt.Sprintf("devminor%d", header.Devminor)
	sha := sha256.New()
	if header.Size > int64(0) {
		_, err := io.Copy(sha, reader)
		if err != nil {
			sha.Reset()
			sha.Write([]byte(headerStr))
		}
	} else {
		sha.Write([]byte(headerStr))
	}
	t.hashes = append(t.hashes, fmt.Sprintf("%x", sha.Sum(nil)))
}

func (t *TarSum) Compute() string {
	sort.Strings(t.hashes)
	sha := sha256.New()
	sha.Write(t.seed)
	for _, hash := range t.hashes {
		sha.Write([]byte(hash))
	}
	tarsum := fmt.Sprintf("tarsum+sha256:%x", sha.Sum(nil))
	logger.Debug("checsums.compute_tarsum: return %s", tarsum)
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
		if strings.HasPrefix(filename, "/.wh.") {
			// wtf is this!? isn't this redundant!? just copying from docker-registry 0.6.5
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
