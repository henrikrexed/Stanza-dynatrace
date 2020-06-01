package pipeline

import (
	"github.com/bluemedora/bplogagent/errors"
	"github.com/bluemedora/bplogagent/plugin"
	"github.com/bluemedora/bplogagent/plugin/helper"
	yaml "gopkg.in/yaml.v2"
)

// Config is the configuration of a pipeline.
type Config []Params

// BuildPipeline will build a pipeline from the config.
func (c Config) BuildPipeline(context plugin.BuildContext) (*Pipeline, error) {
	pluginConfigs, err := c.buildPluginConfigs(context)
	if err != nil {
		return nil, errors.Wrap(err, "build plugin configs")
	}

	plugins, err := c.buildPlugins(pluginConfigs, context)
	if err != nil {
		return nil, errors.Wrap(err, "build plugins")
	}

	pipeline, err := NewPipeline(plugins)
	if err != nil {
		return nil, errors.Wrap(err, "new pipeline")
	}

	return pipeline, nil
}

func (c Config) buildPlugins(pluginConfigs []plugin.Config, context plugin.BuildContext) ([]plugin.Plugin, error) {
	plugins := make([]plugin.Plugin, 0, len(pluginConfigs))
	for _, pluginConfig := range pluginConfigs {
		plugin, err := pluginConfig.Build(context)

		if err != nil {
			return nil, errors.WithDetails(err, "plugin_id", pluginConfig.ID(), "plugin_type", pluginConfig.Type())
		}

		plugins = append(plugins, plugin)
	}

	return plugins, nil
}

func (c Config) buildPluginConfigs(context plugin.BuildContext) ([]plugin.Config, error) {
	pluginConfigs := make([]plugin.Config, 0, len(c))

	for _, params := range c {
		if err := params.Validate(); err != nil {
			return nil, errors.Wrap(err, "validate config params")
		}

		configs, err := params.BuildConfigs(context, "$")
		if err != nil {
			return nil, errors.Wrap(err, "build plugin configs")
		}
		pluginConfigs = append(pluginConfigs, configs...)
	}

	return pluginConfigs, nil
}

// Params is a raw params map that can be converted into a plugin config.
type Params map[string]interface{}

// ID returns the id field in the params map.
func (p Params) ID() string {
	return p.getString("id")
}

// Type returns the type field in the params map.
func (p Params) Type() string {
	return p.getString("type")
}

// Output returns the output field in the params map.
func (p Params) Output() string {
	return p.getString("output")
}

// String will return the string representation of the params.
func (p Params) String() string {
	bytes, err := yaml.Marshal(p)
	if err != nil {
		return ""
	}

	return string(bytes)
}

// Validate will validate the basic fields required to make a plugin config.
func (p Params) Validate() error {
	if p.ID() == "" {
		return errors.NewError(
			"missing required `id` field for plugin config",
			"ensure that all plugin configs have a defined id field",
			"config", p.String(),
		)
	}

	if p.Type() == "" {
		return errors.NewError(
			"missing required `type` field for plugin config",
			"ensure that all plugin configs have a defined type field",
			"config", p.String(),
		)
	}

	return nil
}

// getString returns a string value from the params block
func (p Params) getString(key string) string {
	rawValue, ok := p[key]
	if !ok {
		return ""
	}

	stringValue, ok := rawValue.(string)
	if !ok {
		return ""
	}

	return stringValue
}

// BuildConfigs will build plugin configs from a params map.
func (p Params) BuildConfigs(context plugin.BuildContext, namespace string) ([]plugin.Config, error) {
	if plugin.IsDefined(p.Type()) {
		return p.buildAsBuiltin(namespace)
	}

	if context.CustomRegistry.IsDefined(p.Type()) {
		return p.buildAsCustom(context, namespace)
	}

	return nil, errors.NewError(
		"unsupported `type` field for plugin config",
		"ensure that all plugins have a supported builtin or custom type",
		"config", p.String(),
	)
}

// buildAsBuiltin will build a builtin config from a params map.
func (p Params) buildAsBuiltin(namespace string) ([]plugin.Config, error) {
	bytes, err := yaml.Marshal(p)
	if err != nil {
		return nil, errors.NewError(
			"failed to parse config map as yaml",
			"ensure that all config values are supported yaml values",
			"error", err.Error(),
		)
	}

	var config plugin.Config
	if err := yaml.Unmarshal(bytes, &config); err != nil {
		return nil, err
	}

	config.SetNamespace(namespace)
	return []plugin.Config{config}, nil
}

// buildAsCustom will build a custom config from a params map.
func (p Params) buildAsCustom(context plugin.BuildContext, namespace string) ([]plugin.Config, error) {
	input := helper.AddNamespace(p.ID(), namespace)
	output := helper.AddNamespace(p.Output(), namespace)
	templateParams := map[string]interface{}{}

	for key, value := range p {
		templateParams[key] = value
	}

	templateParams["input"] = input
	templateParams["output"] = output

	config, err := context.CustomRegistry.Render(p.Type(), templateParams)
	if err != nil {
		return nil, errors.Wrap(err, "failed to render custom config")
	}

	for _, pluginConfig := range config.Pipeline {
		pluginConfig.SetNamespace(input, input, output)
	}

	return config.Pipeline, nil
}