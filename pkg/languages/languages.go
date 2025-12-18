package languages

import (
	"strings"

	"github.com/mini-maxit/worker/pkg/constants"
	"github.com/mini-maxit/worker/pkg/errors"
	"github.com/mini-maxit/worker/pkg/messages"
	"github.com/mini-maxit/worker/utils"
)

type LanguageType int

const (
	CPP LanguageType = iota + 1
	PYTHON
)

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
	case PYTHON:
		// Python uses version-specific runtime images
		_, ok := LanguageVersionMap[PYTHON][version]
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
	case PYTHON:
		return true
	case CPP:
		return false
	default:
		return false
	}
}

func (lt LanguageType) GetRunCommand(solutionFileName string) ([]string, error) {
	if err := utils.ValidateFilename(solutionFileName); err != nil {
		return nil, err
	}

	switch lt {
	case PYTHON:
		return []string{"python3", "./" + solutionFileName}, nil
	case CPP:
		return []string{"./" + solutionFileName}, nil
	default:
		return nil, errors.ErrInvalidLanguageType
	}
}

func (lt LanguageType) GetMemoryLimitErrorPatterns() []string {
	switch lt {
	case Python:
		return []string{"MemoryError"}
	case CPP:
		return []string{
			"std::bad_alloc",
			"Memory limit exceeded",
			"Cannot allocate memory",
		}
	default:
		return []string{}
	}
}

var LanguageTypeMap = map[string]LanguageType{
	"CPP":    CPP,
	"PYTHON": PYTHON,
}

var LanguageExtensionMap = map[LanguageType]string{
	CPP:    "cpp",
	PYTHON: "py",
}

var LanguageVersionMap = map[LanguageType]map[string]string{
	CPP: {
		"11": "c++11",
		"14": "c++14",
		"17": "c++17",
		"20": "c++20",
	},
	PYTHON: {
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

func GetSupportedLanguagesWithVersions() messages.ResponseHandshakePayload {
	supportedLanguages := make([]messages.LanguageSpec, 0, len(LanguageTypeMap))
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

		supportedLanguages = append(supportedLanguages, messages.LanguageSpec{
			LanguageName: langName,
			Versions:     versionList,
			Extension:    langExtension,
		})
	}
	return messages.ResponseHandshakePayload{
		Languages: supportedLanguages,
	}
}
