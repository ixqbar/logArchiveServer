package logarchive

import (
	"encoding/xml"
	"errors"
	"fmt"
	"os"
)

type ArchiveConfig struct {
	Address   string `xml:"address"`
	Repertory string `xml:"repertory"`
	Timeout   int    `xml:"timeout"`
}

var Config *ArchiveConfig

func ParseXmlConfig(path string) (*ArchiveConfig, error) {
	if len(path) == 0 {
		return nil, errors.New("not found configure xml file")
	}

	n, err := GetFileSize(path)
	if err != nil || n == 0 {
		return nil, errors.New("not found configure xml file")
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	Config = &ArchiveConfig{}

	data := make([]byte, n)

	m, err := f.Read(data)
	if err != nil {
		return nil, err
	}

	if int64(m) != n {
		return nil, errors.New(fmt.Sprintf("expect read configure xml file size %d but result is %d", n, m))
	}

	err = xml.Unmarshal(data, &Config)
	if err != nil {
		return nil, err
	}

	r, err := CheckFileIsDirectory(Config.Repertory)
	if !r {
		return nil, err
	}

	return Config, nil
}
