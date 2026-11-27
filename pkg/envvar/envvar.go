package envvar

import (
	"errors"
	"strings"

	"github.com/kelseyhightower/envconfig"
)

var envVars EnvironmentalVariables

type EnvironmentalVariables struct {
	// Where Altinnator reaches Altinn: the mock when run locally, or Altinn's platform in TT02 or PROD.
	AltinnPlatformBaseURL string `split_words:"true" required:"true"`
}

func Load() error {
	var loaded EnvironmentalVariables
	if err := envconfig.Process("altinnator", &loaded); err != nil {
		return err
	}
	if strings.TrimSpace(loaded.AltinnPlatformBaseURL) == "" {
		return errors.New("ALTINNATOR_ALTINN_PLATFORM_BASE_URL must not be blank")
	}
	envVars = loaded
	return nil
}

func Get() EnvironmentalVariables {
	return envVars
}
