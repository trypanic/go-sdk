package envs_test

import (
	"fmt"

	"github.com/caarlos0/env/v11"
	"github.com/trypanic/go-sdk/v2/envs"
)

func ExampleLoadWithOptions() {
	var cfg struct {
		Port int `env:"PORT"`
	}
	err := envs.LoadWithOptions(&cfg, env.Options{Environment: map[string]string{"PORT": "8080"}})
	fmt.Println(cfg.Port, err)
	// Output: 8080 <nil>
}
