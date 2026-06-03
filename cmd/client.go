package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var clientCmd = &cobra.Command{
	Use:   "client",
	Short: "Client management commands",
}

var bindCmd = &cobra.Command{
	Use:   "bind [token]",
	Short: "Bind this client to a Hub account using a token",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		token := args[0]

		fmt.Printf("Saving bind token: %s\n", token)

		// Set token in config
		viper.Set("bind_token", token)

		if err := persistViperConfig(); err != nil {
			fmt.Printf("Error updating config file: %v\n", err)
			return
		}

		fmt.Println("Bind token saved successfully.")
		fmt.Println("Please restart the application to complete the binding process.")
	},
}

func init() {
	rootCmd.AddCommand(clientCmd)
	clientCmd.AddCommand(bindCmd)
}
