package needle

import (
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/chrislusf/seaweedfs/weed/glog"
	"github.com/chrislusf/seaweedfs/weed/util"
)

func parseMultipart(r *http.Request, sizeLimit int64) (
	fileName string, data []byte, mimeType string, isGzipped bool, originalDataSize int, isChunkedFile bool, e error) {
	defer func() {
		if e != nil && r.Body != nil {
			io.Copy(ioutil.Discard, r.Body)
			r.Body.Close()
		}
	}()
	form, fe := r.MultipartReader()
	if fe != nil {
		glog.V(0).Infoln("MultipartReader [ERROR]", fe)
		e = fe
		return
	}

	//first multi-part item
	part, fe := form.NextPart()
	if fe != nil {
		glog.V(0).Infoln("Reading Multi part [ERROR]", fe)
		e = fe
		return
	}

	fileName = part.FileName()
	if fileName != "" {
		fileName = path.Base(fileName)
	}

	println("reading part", sizeLimit)

	data, e = ioutil.ReadAll(io.LimitReader(part, sizeLimit+1))
	if e != nil {
		glog.V(0).Infoln("Reading Content [ERROR]", e)
		return
	}
	if len(data) == int(sizeLimit)+1 {
		e = fmt.Errorf("file over the limited %d bytes", sizeLimit)
		return
	}

	//if the filename is empty string, do a search on the other multi-part items
	for fileName == "" {
		part2, fe := form.NextPart()
		if fe != nil {
			break // no more or on error, just safely break
		}

		fName := part2.FileName()

		//found the first <file type> multi-part has filename
		if fName != "" {
			data2, fe2 := ioutil.ReadAll(io.LimitReader(part2, sizeLimit+1))
			if fe2 != nil {
				glog.V(0).Infoln("Reading Content [ERROR]", fe2)
				e = fe2
				return
			}
			if len(data) == int(sizeLimit)+1 {
				e = fmt.Errorf("file over the limited %d bytes", sizeLimit)
				return
			}

			//update
			data = data2
			fileName = path.Base(fName)
			break
		}
	}

	originalDataSize = len(data)

	isChunkedFile, _ = strconv.ParseBool(r.FormValue("cm"))

	if !isChunkedFile {

		dotIndex := strings.LastIndex(fileName, ".")
		ext, mtype := "", ""
		if dotIndex > 0 {
			ext = strings.ToLower(fileName[dotIndex:])
			mtype = mime.TypeByExtension(ext)
		}
		contentType := part.Header.Get("Content-Type")
		if contentType != "" && mtype != contentType {
			mimeType = contentType //only return mime type if not deductable
			mtype = contentType
		}

		if part.Header.Get("Content-Encoding") == "gzip" {
			if unzipped, e := util.UnGzipData(data); e == nil {
				originalDataSize = len(unzipped)
			}
			isGzipped = true
		} else if util.IsGzippable(ext, mtype, data) {
			if compressedData, err := util.GzipData(data); err == nil {
				if len(data) > len(compressedData) {
					data = compressedData
					isGzipped = true
				}
			}
		}
	}

	return
}
