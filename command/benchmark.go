package command

import (
	"fmt"
	"os"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/config"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	RootCmd.AddCommand(benchmarkCmd)
}

var benchmarkCmd = &cobra.Command{
	Use:   "benchmark",
	Short: "Benchmark the connection to the server",
	Long:  `Benchmark the connection to the server.`,
	Args:  cobra.NoArgs,
	PreRun: func(cmd *cobra.Command, args []string) {
		viper.BindPFlag("client.server", cmd.Flags().Lookup("server"))
		viper.BindEnv("client.server", config.CONFIG_ENV_PREFIX+"_SERVER")
	},
	Run: func(cmd *cobra.Command, args []string) {
		cfg := GetServerAddr()
		fmt.Println("Benchmarking server: ", cfg.HttpServer)

		payload := apiclient.BenchmarkPacket{}

		payload.Status = true
		payload.Version = build.Version
		payload.Date = time.Now().UTC()

		client := apiclient.NewClient(cfg.HttpServer, cfg.ApiToken, viper.GetBool("tls_skip_verify"))

		start := time.Now()
		for i := 0; i < 100; i++ {
			err := client.Benchmark(&payload)
			if err != nil {
				fmt.Println("Failed to benchmark server on iteration", i+1)
				os.Exit(1)
			}
		}

		elapsed := time.Since(start)
		fmt.Printf("Benchmark completed in %s\n", elapsed)
	},
}
