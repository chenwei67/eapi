package eapi

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"

	"github.com/chenwei67/eapi/spec"
	"github.com/knadh/koanf"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/urfave/cli/v2"
)

type Config struct {
	Plugin     string
	Dir        string
	Output     string
	Depends    []string
	StrictMode bool
	LogLevel   string `yaml:"logLevel"`
	OpenAPI    OpenAPIConfig

	Generators []*GeneratorConfig
}

type OpenAPIConfig struct {
	OpenAPI         string           `yaml:"openapi"` // OpenAPI version 3.0.0|3.0.3|3.1.0
	Info            *spec.Info       `yaml:"info"`    // Required
	SecuritySchemes *SecuritySchemes `yaml:"securitySchemes"`
}

type SecuritySchemes map[string]*spec.SecurityScheme

func (c OpenAPIConfig) ApplyToDoc(doc *spec.T) {
	if c.OpenAPI != "" {
		doc.OpenAPI = c.OpenAPI
	}
	if c.Info != nil {
		if c.Info.Version != "" {
			doc.Info.Version = c.Info.Version
		}
		if c.Info.Title != "" {
			doc.Info.Title = c.Info.Title
		}
		if c.Info.Description != "" {
			doc.Info.Description = c.Info.Description
		}
		if c.Info.TermsOfService != "" {
			doc.Info.TermsOfService = c.Info.TermsOfService
		}
	}
	if c.SecuritySchemes != nil {
		doc.Components.SecuritySchemes = make(map[string]*spec.SecuritySchemeRef)
		for name, scheme := range *c.SecuritySchemes {
			doc.Components.SecuritySchemes[name] = &spec.SecuritySchemeRef{Value: scheme}
		}
	}
}

type GeneratorConfig struct {
	Name   string
	File   string
	Output string
}

type Entrypoint struct {
	k       *koanf.Koanf
	plugins []Plugin

	cfg Config
}

func NewEntrypoint(plugins ...Plugin) *Entrypoint {
	return &Entrypoint{
		k:       koanf.New("."),
		plugins: plugins,
		cfg: Config{
			Plugin:   "gin",
			Dir:      ".",
			Output:   "docs",
			LogLevel: "info",
		},
	}
}

const usageText = `Generate Doc:
	eapi --config config.yaml
or
	eapi --plugin gin --dir src/ --output docs/

Generate Frontend Code:
	eapi --config config.yaml gencode
or
	eapi --plugin gin --dir src/ --output docs/ gencode`

func (e *Entrypoint) Run(args []string) {
	app := cli.NewApp()
	app.Name = "egen"
	app.Usage = `Tool for generating OpenAPI documentation and Frontend Code by static-analysis`
	app.UsageText = usageText
	app.Description = `Tool for generating OpenAPI documentation and Frontend Code by static-analysis`
	app.Flags = append(app.Flags, &cli.StringFlag{
		Name:        "plugin",
		Aliases:     []string{"p", "plug"},
		Usage:       "specify plugin name",
		Destination: &e.cfg.Plugin,
	})
	app.Flags = append(app.Flags, &cli.StringFlag{
		Name:        "dir",
		Aliases:     []string{"d"},
		Usage:       "directory of your project which contains go.mod file",
		Destination: &e.cfg.Dir,
	})
	app.Flags = append(app.Flags, &cli.StringFlag{
		Name:        "output",
		Aliases:     []string{"o"},
		Usage:       "output directory of openapi.json",
		Destination: &e.cfg.Output,
	})
	app.Flags = append(app.Flags, &cli.StringSliceFlag{
		Name:    "depends",
		Aliases: []string{"dep"},
		Usage:   "depended module name",
		Action: func(context *cli.Context, depends []string) error {
			e.cfg.Depends = depends
			return nil
		},
	})
	app.Flags = append(app.Flags, &cli.StringFlag{
		Name:     "config",
		Aliases:  []string{"c"},
		Usage:    "configuration file",
		Required: false,
	})
	app.Flags = append(app.Flags, &cli.BoolFlag{
		Name:        "strict",
		Aliases:     []string{"s"},
		Usage:       "enable strict mode - show red error logs instead of skipping issues",
		Destination: &e.cfg.StrictMode,
	})
	app.Flags = append(app.Flags, &cli.StringFlag{
		Name:        "log-level",
		Aliases:     []string{"l"},
		Usage:       "set log level (silent, error, warn, info, debug)",
		Value:       "info",
		Destination: &e.cfg.LogLevel,
	})

	app.Commands = append(app.Commands, showVersion())

	app.Action = e.run

	err := app.Run(args)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}

func (e *Entrypoint) before(c *cli.Context) error {
	cfg := c.String("config")
	if cfg == "" {
		fileInfo, err := os.Stat("eapi.yaml")
		if err == nil && !fileInfo.IsDir() {
			cfg = "eapi.yaml"
		}
	}
	if cfg != "" {
		err := e.loadConfig(cfg)
		if err != nil {
			return err
		}
		err = e.k.Unmarshal("", &e.cfg)
		if err != nil {
			return err
		}
	}

	if e.cfg.Plugin == "" {
		return fmt.Errorf("'plugin' is not set")
	}
	if e.cfg.Dir == "" {
		e.cfg.Dir = "."
	}
	if e.cfg.Output == "" {
		e.cfg.Output = "docs"
	}

	// Initialize global logger
	logLevel := ParseLogLevel(e.cfg.LogLevel)
	SetGlobalLogLevel(logLevel)
	SetGlobalLogStrictMode(e.cfg.StrictMode)

	return nil
}

func (e *Entrypoint) loadConfig(cfg string) error {
	return e.k.Load(file.Provider(cfg), yaml.Parser())
}

func (e *Entrypoint) run(c *cli.Context) error {
	var plugin Plugin

	err := e.before(c)
	if err != nil {
		return err
	}

	for _, p := range e.plugins {
		if p.Name() == e.cfg.Plugin {
			plugin = p
			break
		}
	}
	if plugin == nil {
		return fmt.Errorf("plugin %s not exists", e.cfg.Plugin)
	}

	stat, err := os.Stat(e.cfg.Dir)
	if err != nil {
		return err
	}
	if !stat.IsDir() {
		return fmt.Errorf("%s is not a directory", e.cfg.Dir)
	}

	err = os.MkdirAll(e.cfg.Output, os.ModePerm)
	if err != nil {
		return err
	}
	LogInfo("output directory: %s", e.cfg.Output)
	if e.cfg.StrictMode {
		LogWarn("[STRICT MODE] Enabled - errors will be reported instead of skipped")
	}
	a := NewAnalyzer(e.k).Plugin(plugin).Depends(e.cfg.Depends...).WithStrictMode(e.cfg.StrictMode)
	LogDebug("doc0: 开始处理文档")

	// 获取原始文档
	LogDebug("doc0.1: 开始Process处理")
	processedAnalyzer := a.Process(e.cfg.Dir)
	LogDebug("doc0.2: Process处理完成")

	rawDoc := processedAnalyzer.Doc()
	LogDebug("doc0.3: 获取原始文档完成，开始Specialize处理")

	// 执行Specialize，这里可能出现unknown type error
	doc := rawDoc.Specialize()
	LogDebug("doc1: Specialize处理完成")
	e.cfg.OpenAPI.ApplyToDoc(doc)
	// write documentation
	{
		docContent, err := json.MarshalIndent(doc, "", "    ")
		if err != nil {
			return fmt.Errorf("json MarshalIndent err: %s", err.Error())
		}
		err = os.WriteFile(filepath.Join(e.cfg.Output, "openapi.json"), docContent, fs.ModePerm)
		if err != nil {
			return err
		}
	}

	// execute generators
	for idx, item := range e.cfg.Generators {
		err = newGeneratorExecutor(
			item,
			doc,
			func(key string) interface{} {
				confMap := e.k.Get("generators").([]interface{})[idx].(map[string]interface{})
				val, ok := confMap[key]
				if !ok {
					return nil
				}
				return val
			},
			e.cfg.StrictMode,
		).execute()
		if err != nil {
			return err
		}
	}

	return nil
}

func showVersion() *cli.Command {
	return &cli.Command{
		Name: "version",
		Action: func(c *cli.Context) error {
			info, ok := debug.ReadBuildInfo()
			if !ok {
				LogInfo("unknown version")
				os.Exit(1)
				return nil
			}
			LogInfo("%s", info.Main.Version)
			return nil
		},
	}
}
