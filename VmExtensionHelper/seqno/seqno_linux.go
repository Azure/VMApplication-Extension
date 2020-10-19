package seqno

import "path"

var mostRecentSequenceFileName = "mrseq"

// sequence number for the extension from the registry
func getSequenceNumberInternal(name, version, configFolder string) (uint, error) {
	mrseqStr, err := ioutil.ReadFile(path.Join(configFolder, mostRecentSequenceFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return -1, nil
		}
		return -1, fmt.Errorf("failed to read mrseq file : %s", err)
	}

	return strconv.Atoi(string(mrseqStr))

}

func setSequenceNumberInternal(extName, extVersion string, seqNo uint) error {
	b := []byte(fmt.Sprintf("%v", sequenceNumber))
	err := ioutil.WriteFile(mostRecentSequenceFileName, b, chmod)
	if err != nil {
		return errorhelper.AddStackToError(err)
	}
	return nil
}