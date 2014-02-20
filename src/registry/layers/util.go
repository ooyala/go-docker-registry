package layers

import (
	"encoding/json"
	"errors"
	"registry/logger"
	"registry/storage"
	"strings"
)

func GetImageFilesCache(s storage.Storage, imageID string) ([]byte, error) {
	return s.Get(storage.ImageFilesPath(imageID))
}

func SetImageFilesCache(s storage.Storage, imageID string, filesJson []byte) error {
	return s.Put(storage.ImageFilesPath(imageID), filesJson)
}

// return json file listing for givem image id
// Dowload the specified layer and determine the file contents. If the cache already exists, just return it.
func GetImageFilesJson(s storage.Storage, imageID string) ([]byte, error) {
	// check if cache exists
	filesJson, err := GetImageFilesCache(s, imageID)
	if err != nil {
		return filesJson, nil
	}

	// cache doesn't exist. download remote layer
	// docker-registry 0.6.5 has an lzma decompress here. it actually doesn't seem to be used so i've omitted it
	// will add it later if need be.
	tarFilesInfo := NewTarFilesInfo()
	reader, err := s.GetReader(storage.ImageLayerPath(imageID))
	if err != nil {
		return nil, err
	}
	if err := tarFilesInfo.Load(reader); err != nil {
		return nil, err
	}
	return tarFilesInfo.Json()
}

func StoreChecksum(s storage.Storage, imageID, checksum string) error {
	parts := strings.Split(checksum, ":")
	if len(parts) != 2 {
		return errors.New("Invalid checksum format")
	}
	return s.Put(storage.ImageChecksumPath(imageID), []byte(checksum))
}

func GenerateAncestry(s storage.Storage, imageID, parentID string) error {
	path := storage.ImageAncestryPath(imageID)
	if parentID == "" {
		s.Put(path, []byte("[\""+imageID+"\"]"))
	}
	content, err := s.Get(storage.ImageAncestryPath(parentID))
	if err != nil {
		return err
	}
	var ancestry []string
	if err := json.Unmarshal(content, &ancestry); err != nil {
		return err
	}
	ancestry = append([]string{imageID}, ancestry...)
	if content, err = json.Marshal(&ancestry); err != nil {
		return err
	}
	return s.Put(path, content)
}

func GetImageDiffCache(s storage.Storage, imageID string) ([]byte, error) {
	path := storage.ImageDiffPath(imageID)
	if exists, _ := s.Exists(path); exists {
		return s.Get(storage.ImageDiffPath(imageID))
	}
	return nil, nil // nil error, because cache missed
}

func SetImageDiffCache(s storage.Storage, imageID string, diffJson []byte) error {
	return s.Put(storage.ImageDiffPath(imageID), diffJson)
}

func GenDiff(s storage.Storage, imageID string) {
	// Comment from docker-registry 0.6.5
	// get json describing file differences in layer
	// Calculate the diff information for the files contained within
	// the layer. Return a dictionary of lists grouped by whether they
	// were deleted, changed or created in this layer.
	// To determine what happened to a file in a layer we walk backwards
	// through the ancestry until we see the file in an older layer. Based
	// on whether the file was previously deleted or not we know whether
	// the file was created or modified. If we do not find the file in an
	// ancestor we know the file was just created.
	// - File marked as deleted by union fs tar: DELETED
	// - Ancestor contains non-deleted file:     CHANGED
	// - Ancestor contains deleted marked file:  CREATED
	// - No ancestor contains file:              CREATED

	diffJson, err := GetImageDiffCache(s, imageID)
	if err == nil && diffJson != nil {
		// cache hit, just return
		logger.Debug("[GenDiff][" + imageID + "] already exists")
		return
	}

	anPath := storage.ImageAncestryPath(imageID)
	anContent, err := s.Get(anPath)
	if err != nil {
		// error fetching ancestry, just return
		logger.Error("[GenDiff][" + imageID + "] error fetching ancestry: " + err.Error())
		return
	}
	var ancestry []string
	if err := json.Unmarshal(anContent, &ancestry); err != nil {
		// json unmarshal fail, just return
		logger.Error("[GenDiff][" + imageID + "] error unmarshalling ancestry json: " + err.Error())
		return
	}
	// get map of file infos
	infoMap, err := fileInfoMap(s, imageID)
	if err != nil {
		// error getting file info, just return
		logger.Error("[GenDiff][" + imageID + "] error getting files info: " + err.Error())
		return
	}

	deleted := map[string][]interface{}{}
	changed := map[string][]interface{}{}
	created := map[string][]interface{}{}

	for i := 1; i < len(ancestry); i++ {
		anID := ancestry[i]
		anInfoMap, err := fileInfoMap(s, anID)
		if err != nil {
			// error getting file info, just return
			logger.Error("[GenDiff][" + imageID + "] error getting ancestor " + anID + " files info: " + err.Error())
			return
		}
		for fname, info := range infoMap {
			isDeleted, ok := (info[1]).(bool)
			if !ok || isDeleted { // !ok because it should technically never happen, but if it does just mark it as deleted
				if !ok {
					logger.Error("[GenDiff][" + imageID + "] file info is in a bad format")
				}
				deleted[fname] = info
				delete(infoMap, fname)
				continue
			}
			anInfo := anInfoMap[fname]
			if err != nil || anInfo == nil {
				// doesn't exist, must be created. do nothing.
				continue
			}
			isDeleted, ok = anInfo[1].(bool)
			if !ok || isDeleted {
				if !ok {
					logger.Error("[GenDiff][" + imageID + "] file info is in a bad format")
				}
				// deleted in ancestor, must be created now.
				created[fname] = info
			} else {
				// not deleted in ancestor, must have just changed now.
				changed[fname] = info
			}
			delete(infoMap, fname)
		}
	}
	// dump all created stuff from infoMap
	for fname, info := range infoMap {
		created[fname] = info
	}

	diff := map[string]map[string][]interface{}{
		"deleted": deleted,
		"changed": changed,
		"created": created,
	}
	if diffJson, err = json.Marshal(&diff); err != nil {
		// json marshal fail. just return
		logger.Error("[GenDiff][" + imageID + "] error marshalling new diff json: " + err.Error())
		return
	}
	if err := SetImageDiffCache(s, imageID, diffJson); err != nil {
		// json marshal fail. just return
		logger.Error("[GenDiff][" + imageID + "] error setting new diff cache: " + err.Error())
		return
	}
}

func fileInfoMap(s storage.Storage, imageID string) (map[string][]interface{}, error) {
	fContent, err := GetImageFilesJson(s, imageID)
	if err != nil {
		return nil, err
	}
	var infoArr [][]interface{}
	if err := json.Unmarshal(fContent, &infoArr); err != nil {
		return nil, err
	}
	m := make(map[string][]interface{}, len(infoArr))
	for _, info := range infoArr {
		if len(info) != TAR_FILES_INFO_SIZE {
			continue
		}
		if nameStr, ok := info[0].(string); ok {
			m[nameStr] = info[1:]
		}
	}
	return m, nil
}
