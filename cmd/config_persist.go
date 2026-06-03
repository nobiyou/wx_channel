package cmd

import (
	"os"

	"github.com/spf13/viper"
)

func persistViperConfig() error {
	if viper.ConfigFileUsed() == "" {
		viper.SetConfigFile("config.yaml")
	}

	if err := viper.WriteConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok || os.IsNotExist(err) || err.Error() == "missing configuration for 'configPath'" {
			if err := viper.SafeWriteConfig(); err != nil {
				if err := viper.WriteConfigAs("config.yaml"); err != nil {
					return err
				}
			}
			return nil
		}

		if err := viper.WriteConfigAs("config.yaml"); err != nil {
			return err
		}
	}

	return nil
}
