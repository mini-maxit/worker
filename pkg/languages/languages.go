package languages

import (
	"strings"

	"github.com/mini-maxit/worker/pkg/constants"
	"github.com/mini-maxit/worker/pkg/errors"
)

type LanguageType int

const (
	CPP LanguageType = iota + 1
	Python
)

type LanguageSpec struct {
	LanguageName string   `json:"name"`
	Versions     []string `json:"versions"`
	Extension    string   `json:"extension"`
}

func (lt LanguageType) String() string {
	for key, value := range LanguageTypeMap {
		if value == lt {
			return key
		}
	}
	return ""
}

func (lt LanguageType) GetDockerImage(version string) (string, error) {
	switch lt {
	case CPP:
		// C++ compiler does not require versioning, so a single runtime Docker image is used for all versions.
		image := constants.RuntimeImagePrefix + "-cpp:latest"
		return image, nil
	case Python:
		// Python uses version-specific runtime images
		_, ok := LanguageVersionMap[Python][version]
		if !ok {
			return "", errors.ErrInvalidVersion
		}
		image := constants.RuntimeImagePrefix + "-python-" + version + ":latest"
		return image, nil
	default:
		return "", errors.ErrInvalidLanguageType
	}
}

func (lt LanguageType) IsScriptingLanguage() bool {
	switch lt {
	case Python:
		return true
	case CPP:
		return false
	default:
		return false
	}
}

func (lt LanguageType) GetRunCommand(solutionFileName string) (string, error) {
	switch lt {
	case Python:
		return "python3 ./" + solutionFileName, nil
	case CPP:
		return "./" + solutionFileName, nil
	default:
		return "", errors.ErrInvalidLanguageType
	}
}

var LanguageTypeMap = map[string]LanguageType{
	"CPP":    CPP,
	"PYTHON": Python,
}

var LanguageExtensionMap = map[LanguageType]string{
	CPP:    "cpp",
	Python: "py",
}

var LanguageVersionMap = map[LanguageType]map[string]string{
	CPP: {
		"11": "c++11",
		"14": "c++14",
		"17": "c++17",
		"20": "c++20",
	},
	Python: {
		"3.8":  "3.8",
		"3.9":  "3.9",
		"3.10": "3.10",
		"3.11": "3.11",
		"3.12": "3.12",
	},
}

func GetVersionFlag(language LanguageType, version string) (string, error) {
	if versions, ok := LanguageVersionMap[language]; ok {
		if flag, ok := versions[version]; ok {
			return flag, nil
		}
		return "", errors.ErrInvalidVersion
	}
	return "", errors.ErrInvalidLanguageType
}

func GetSupportedLanguages() []string {
	var languages []string
	for lang := range LanguageTypeMap {
		languages = append(languages, lang)
	}
	return languages
}

func ParseLanguageType(s string) (LanguageType, error) {
	if lt, ok := LanguageTypeMap[strings.ToUpper(s)]; ok {
		return lt, nil
	}
	return 0, errors.ErrInvalidLanguageType
}

func GetSupportedLanguagesWithVersions() []LanguageSpec {
	supportedLanguages := make([]LanguageSpec, 0, len(LanguageTypeMap))
	for langType, versions := range LanguageVersionMap {
		langName := langType.String()
		langExtension, ok := LanguageExtensionMap[langType]
		if !ok {
			langExtension = ""
		}

		var versionList []string
		for version := range versions {
			versionList = append(versionList, version)
		}

		supportedLanguages = append(supportedLanguages, LanguageSpec{
			LanguageName: langName,
			Versions:     versionList,
			Extension:    langExtension,
		})
	}
	return supportedLanguages
}
