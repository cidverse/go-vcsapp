package vcsapp

import (
	"os"
	"strings"
)

func getEnvAsMap() map[string]string {
	envMap := make(map[string]string)

	for _, env := range os.Environ() {
		keyValue := strings.SplitN(env, "=", 2)
		if len(keyValue) == 2 {
			envMap[keyValue[0]] = keyValue[1]
		}
	}

	return envMap
}

func mapHasKey(m map[string]string, key string) bool {
	_, exists := m[key]
	return exists
}
