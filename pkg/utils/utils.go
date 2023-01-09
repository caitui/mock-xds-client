package utils

import "os"

func GetDefaultEnv(key, defaultVal string) string {
	val, ex := os.LookupEnv(key)
	if !ex {
		return defaultVal
	}

	return val
}
