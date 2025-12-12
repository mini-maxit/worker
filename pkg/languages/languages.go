package languages

import (
	"fmt"
	"strings"

	"github.com/mini-maxit/worker/pkg/constants"
	"github.com/mini-maxit/worker/pkg/errors"
	"github.com/mini-maxit/worker/pkg/messages"
)

type LanguageType int

const (
	CPP LanguageType = iota + 1
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
	default:
		return "", errors.ErrInvalidLanguageType
	}
}

var LanguageTypeMap = map[string]LanguageType{
	"CPP": CPP,
}

var LanguageExtensionMap = map[LanguageType]string{
	CPP: "cpp",
}

var LanguageVersionMap = map[LanguageType]map[string]string{
	CPP: {
		"11": "c++11",
		"14": "c++14",
		"17": "c++17",
		"20": "c++20",
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

func GetSolutionFileNameWithExtension(solutionName string, language LanguageType) (string, error) {
	if extension, ok := LanguageExtensionMap[language]; ok {
		return fmt.Sprintf("%s%s", solutionName, extension), nil
	}
	return "", errors.ErrInvalidLanguageType
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
