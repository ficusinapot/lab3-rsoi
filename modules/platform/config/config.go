package config

import (
	"path/filepath"

	"github.com/samber/oops"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Action NewCommand keeps configuration local to each command and loads only YAML.
type Action[T any] struct {
	Name     string
	Run      func(*cobra.Command, T) error
	Children []Action[T]
}

func NewCommand[T any](name string, run func(*cobra.Command, T) error, actions ...Action[T]) *cobra.Command {
	var path string
	callback := func(run func(*cobra.Command, T) error) func(*cobra.Command, []string) error {
		return func(cmd *cobra.Command, _ []string) error {
			cfg, err := Load[T](path)
			if err != nil {
				return err
			}
			return run(cmd, cfg)
		}
	}
	command := &cobra.Command{
		Use:           name,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          callback(run),
	}
	command.PersistentFlags().StringVar(&path, "config", "config.yaml", "YAML configuration file")
	var add func(*cobra.Command, []Action[T])
	add = func(parent *cobra.Command, children []Action[T]) {
		for _, action := range children {
			child := &cobra.Command{Use: action.Name, Args: cobra.NoArgs, RunE: callback(action.Run)}
			parent.AddCommand(child)
			add(child, action.Children)
		}
	}
	add(command, actions)
	return command
}

func Load[T any](path string) (T, error) {
	var cfg T
	if ext := filepath.Ext(path); ext != ".yaml" && ext != ".yml" {
		return cfg, oops.Errorf("configuration must be a YAML file")
	}
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil {
		return cfg, oops.Wrapf(err, "read configuration")
	}
	if err := v.UnmarshalExact(&cfg); err != nil {
		return cfg, oops.Wrapf(err, "decode configuration")
	}
	if validatable, ok := any(&cfg).(interface{ Validate() error }); ok {
		if err := validatable.Validate(); err != nil {
			return cfg, oops.Wrapf(err, "validate configuration")
		}
	}
	return cfg, nil
}
