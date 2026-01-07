package config

import (
	"os"
)

type Config struct {
	OpenWebUIHostURL   string
	OpenWebUIToken     string
	OpenWebUIModelName string
	DVSAAPIURL         string
	OpenALPRAPIURL     string
	TempDirPath        string
}

func LoadConfigFromEnv() (*Config, error) {

	return &Config{
		OpenWebUIHostURL:   os.Getenv("OPENWEBUIHOSTURL"),
		OpenWebUIToken:     os.Getenv("OPENWEBUIAPITOKEN"),
		OpenWebUIModelName: os.Getenv("OPENWEBUIMODELNAME"),
		DVSAAPIURL:         os.Getenv("DVSAAPIURL"),
		OpenALPRAPIURL:     os.Getenv("OPENALPRAPIURL"),
		TempDirPath:        os.Getenv("TEMPDIRPATH"),
	}, nil
}
