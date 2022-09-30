package contentfulclient

import (
	"encoding/json"
	"os"
	"os/user"
	"strings"

	"github.com/foomo/contentful"
)

type contentfulRc struct {
	ManagementToken string `json:"managementToken"`
}

func GetCmaKeyFromRcFile() string {
	currentUser, errGetUser := user.Current()
	if errGetUser != nil {
		return ""
	}
	contentfulRcBytes, errReadFile := os.ReadFile(currentUser.HomeDir + "/.contentfulrc.json")
	if errReadFile != nil {
		return ""
	}
	var contentfulConfig contentfulRc
	errUnmarshal := json.Unmarshal(contentfulRcBytes, &contentfulConfig)
	if errUnmarshal != nil {
		return ""
	}
	return contentfulConfig.ManagementToken
}

func GetCMA(cmaKey string) *contentful.Contentful {
	return contentful.NewCMA(cmaKey)
}

func GetSpaceAndEnvironment(param string) (spaceID string, environment string) {
	splits := strings.Split(param, "/")
	if len(splits) > 1 {
		return splits[0], splits[1]
	}
	return splits[0], "master"
}
